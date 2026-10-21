// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package messages

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/tidwall/gjson"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/lib/ansimage"
	"go.mau.fi/gomuks/tui/messages/tstring"
)

type FileMessage struct {
	Type event.MessageType
	Body string
	// FileName is the event's filename field. When it's set, Body is a
	// caption rather than the file name.
	FileName string
	MimeType string
	// LinkURL is the post this media came from (e.g. an Instagram reel):
	// the bridge's external_url, else the first URL in the caption. Bridged
	// reels often attach only a thumbnail, so this is the playable source.
	LinkURL  string
	linkLine tstring.TString

	URL         id.ContentURI
	IsEncrypted bool

	eventID id.EventID

	imageData   []byte
	downloading bool
	buffer      []tstring.TString

	kittyPNG  []byte
	kittyCols int
	kittyRows int

	// Audio: duration, 16 bar levels (made up when the bridge sent none).
	Duration  time.Duration
	levels    []int
	synthetic bool
	voiceTail string
	voiceRow  bool

	matrix *client.GomuksClient
}

// NewFileMessage creates a new FileMessage object with the provided values and the default state.
func NewFileMessage(room *store.RoomStore, matrix *client.GomuksClient, evt *database.Event, content *event.MessageEventContent) *UIMessage {
	var url id.ContentURI
	var isEncrypted bool
	if content.File != nil {
		url = content.File.URL.ParseOrIgnore()
		isEncrypted = true
	} else {
		url = content.URL.ParseOrIgnore()
	}
	mimeType := ""
	if content.Info != nil {
		mimeType = content.Info.MimeType
	}
	link := gjson.GetBytes(evt.GetContent(), "external_url").Str
	if link == "" {
		link = FirstURL(content.Body)
	}
	fm := &FileMessage{
		Type:        content.MsgType,
		Body:        content.Body,
		FileName:    content.FileName,
		MimeType:    mimeType,
		LinkURL:     link,
		URL:         url,
		IsEncrypted: isEncrypted,
		eventID:     evt.ID,
		matrix:      matrix,
	}
	if content.MsgType == event.MsgAudio {
		fm.setAudio(content, evt.ID)
	}
	return newUIMessage(room, evt, content, "", fm)
}

func (msg *FileMessage) setAudio(content *event.MessageEventContent, eventID id.EventID) {
	ms := 0
	if content.Info != nil {
		ms = content.Info.Duration
	}
	var wave []int
	if content.MSC1767Audio != nil {
		ms = max(ms, content.MSC1767Audio.Duration)
		wave = content.MSC1767Audio.Waveform
	}
	msg.Duration = time.Duration(ms) * time.Millisecond
	msg.levels = waveformLevels(wave)
	if len(wave) == 0 {
		msg.levels, msg.synthetic = syntheticLevels(eventID.String()), true
	}
	// Voice notes need no label; other audio shows its name.
	if content.MSC3245Voice == nil {
		name := content.FileName
		if name == "" {
			name = content.Body
		}
		msg.voiceTail = " · " + name
	}
}

func (msg *FileMessage) Clone() MessageRenderer {
	data := make([]byte, len(msg.imageData))
	copy(data, msg.imageData)
	return &FileMessage{
		Type:        msg.Type,
		Body:        msg.Body,
		FileName:    msg.FileName,
		MimeType:    msg.MimeType,
		LinkURL:     msg.LinkURL,
		URL:         msg.URL,
		IsEncrypted: msg.IsEncrypted,
		imageData:   data,
		eventID:     msg.eventID,
		Duration:    msg.Duration,
		levels:      msg.levels,
		synthetic:   msg.synthetic,
		voiceTail:   msg.voiceTail,
		matrix:      msg.matrix,
	}
}

// EventID is the message's event, used to match the playing voice note.
func (msg *FileMessage) EventID() id.EventID {
	return msg.eventID
}

func (msg *FileMessage) NotificationContent() string {
	switch msg.Type {
	case event.MsgImage:
		return "Sent an image"
	case event.MsgAudio:
		return "Sent an audio file"
	case event.MsgVideo:
		return "Sent a video"
	case event.MsgFile:
		fallthrough
	default:
		return "Sent a file"
	}
}

func (msg *FileMessage) PlainText() string {
	return fmt.Sprintf("%s: %s", msg.Body, msg.matrix.GetDownloadURL(msg.URL, msg.IsEncrypted, true))
}

func (msg *FileMessage) String() string {
	return fmt.Sprintf(`&messages.FileMessage{Body="%s", URL="%s", Encrypted=%t}`, msg.Body, msg.URL, msg.IsEncrypted)
}

// DownloadPreview fetches the image in the background; when it lands, the
// message buffer is invalidated and a repaint requested, so the "Download
// media" placeholder upgrades to an inline rendering.
func (msg *FileMessage) DownloadPreview(uiMsg *UIMessage) {
	if msg.Type != event.MsgImage || len(msg.imageData) > 0 || msg.downloading || msg.URL.IsEmpty() {
		return
	}
	msg.downloading = true
	go func() {
		defer debug.Recover()
		data, err := msg.matrix.Download(msg.URL, msg.IsEncrypted)
		if err != nil {
			debug.Printf("Failed to download image %s: %v", msg.URL, err)
			return
		}
		msg.imageData = data
		// Pre-encode a display-sized preview off the render thread; both the
		// kitty and the half-block path render from it so HD originals are
		// decoded at full resolution exactly once.
		pngData, err := encodeToPNG(data)
		if err != nil {
			// Animated webp and friends: let ffmpeg take the first frame.
			pngData, err = ffmpegFirstFramePNG(data)
		}
		if err == nil {
			msg.kittyPNG = pngData
		} else {
			debug.Print("Failed to prepare image preview:", err)
		}
		uiMsg.bufferedWidth = -1
		RequestRender()
	}()
}

func (msg *FileMessage) ThumbnailPath() string {
	return "" // FIXME
	//return msg.matrix.GetCachePath(msg.Thumbnail)
}

func (msg *FileMessage) CalculateBuffer(prefs config.UserPreferences, width int, uiMsg *UIMessage) {
	if width < 2 {
		return
	}
	// Only image renderings get the link row; the text fallback already
	// shows the caption, which contains the link.
	msg.linkLine = nil
	msg.voiceRow = false

	// Reply previews keep the one-line text form.
	if msg.levels != nil && !prefs.BareMessageView && !uiMsg.IsReplyBubble {
		msg.voiceRow = true
		msg.buffer = nil
		msg.kittyRows = 0
		return
	}

	if prefs.BareMessageView || prefs.DisableImages || len(msg.imageData) == 0 {
		url := msg.matrix.GetDownloadURL(msg.URL, msg.IsEncrypted, true)
		var urlTString tstring.TString
		if prefs.EnableInlineURLs() {
			urlTString = tstring.NewStyleTString("Download media", tcell.StyleDefault.Url(url).UrlId(msg.eventID.String()))
		} else {
			urlTString = tstring.NewTString(url)
		}
		text := tstring.NewTString(msg.Body).
			Append(": ").
			AppendTString(urlTString)
		msg.buffer = calculateBufferWithText(prefs, text, width, uiMsg)
		return
	}

	// Render from the downscaled preview when available; decoding the
	// original again would repeat the full-resolution work per width change.
	renderData := msg.imageData
	if len(msg.kittyPNG) > 0 {
		renderData = msg.kittyPNG
	}
	img, _, err := image.DecodeConfig(bytes.NewReader(renderData))
	if err != nil || img.Width <= 0 || img.Height <= 0 {
		debug.Print("File could not be decoded:", err)
		msg.buffer = []tstring.TString{tstring.NewColorTString("Failed to display image", tcell.ColorRed)}
		return
	}

	// Kitty placeholder path: real pixels instead of half-blocks. Reply
	// bubbles keep the text fallback; a virtual placement has one geometry
	// per image, and the bubble preview would fight the full-size one.
	if len(msg.kittyPNG) > 0 && !uiMsg.IsReplyBubble && kittyUsable(prefs.ImageProtocol) {
		maxCols := width - 2
		if maxCols > 48 {
			maxCols = 48
		}
		cols := maxCols
		// A cell is roughly twice as tall as wide.
		rows := (img.Height*cols + img.Width) / (img.Width * 2)
		const maxRows = 16
		if rows > maxRows {
			rows = maxRows
			cols = rows * 2 * img.Width / img.Height
			if cols > maxCols {
				cols = maxCols
			}
		}
		if rows < 1 {
			rows = 1
		}
		if cols < 1 {
			cols = 1
		}
		msg.kittyCols = cols
		msg.kittyRows = rows
		msg.buffer = nil
		msg.linkLine = msg.makeLinkLine(prefs, width)
		return
	}
	msg.kittyRows = 0
	// Fit into a bounding box: each terminal row is two pixels tall, so a
	// 16-row cap means 32 pixels of height. Never upscale.
	maxCols := width - 2
	if maxCols > 48 {
		maxCols = 48
	}
	const maxRowsPx = 32
	pxWidth := img.Width
	if pxWidth > maxCols {
		pxWidth = maxCols
	}
	if h := img.Height * pxWidth / img.Width; h > maxRowsPx {
		pxWidth = maxRowsPx * img.Width / img.Height
	}
	if pxWidth < 1 {
		pxWidth = 1
	}

	ansFile, err := ansimage.NewScaledFromReader(bytes.NewReader(renderData), 0, pxWidth, color.Black)
	if err != nil {
		msg.buffer = []tstring.TString{tstring.NewColorTString("Failed to display image", tcell.ColorRed)}
		debug.Print("Failed to display image:", err)
		return
	}

	msg.buffer = ansFile.Render()
	msg.linkLine = msg.makeLinkLine(prefs, width)
}

// makeLinkLine renders the source post as a dim, clickable row under the
// thumbnail, e.g. "▶ instagram.com/reel/AbCdEfGhIjK".
func (msg *FileMessage) makeLinkLine(prefs config.UserPreferences, width int) tstring.TString {
	if msg.LinkURL == "" {
		return nil
	}
	label := "▶ " + shortLink(msg.LinkURL)
	style := tcell.StyleDefault.Foreground(tcell.ColorGray)
	if prefs.EnableInlineURLs() {
		style = style.Url(msg.LinkURL).UrlId(msg.eventID.String() + "-link")
	} else {
		// Without hyperlinks the full URL has to be visible to be usable.
		label = "▶ " + msg.LinkURL
	}
	if runewidth.StringWidth(label) > width-1 {
		label = runewidth.Truncate(label, width-1, "…")
	}
	return tstring.NewStyleTString(label, style)
}

// shortLink drops the scheme, www., query string and trailing slash.
func shortLink(link string) string {
	link = strings.TrimPrefix(strings.TrimPrefix(link, "https://"), "http://")
	link = strings.TrimPrefix(link, "www.")
	if i := strings.IndexAny(link, "?#"); i != -1 {
		link = link[:i]
	}
	return strings.TrimSuffix(link, "/")
}

// FirstURL returns the first http(s) URL in a message body. Bridged link
// posts, like Instagram reels, carry it bracketed: "[https://…](https://…)".
func FirstURL(body string) string {
	idx := strings.Index(body, "http")
	if idx == -1 {
		return ""
	}
	url := body[idx:]
	if end := strings.IndexAny(url, " ]\n)"); end != -1 {
		url = url[:end]
	}
	return url
}

func (msg *FileMessage) Height() int {
	if msg.voiceRow {
		return 1
	}
	base := len(msg.buffer)
	if msg.kittyRows > 0 {
		base = msg.kittyRows
	}
	if msg.linkLine != nil {
		base++
	}
	return base
}

func (msg *FileMessage) Draw(screen mauview.Screen, _ *UIMessage) {
	if msg.voiceRow {
		msg.drawVoice(screen)
		return
	}
	rows := len(msg.buffer)
	drewKitty := false
	if msg.kittyRows > 0 && measuring {
		for y := 0; y < msg.kittyRows; y++ {
			for x := 0; x < msg.kittyCols; x++ {
				screen.SetContent(x, y, kittyPlaceholder, nil, tcell.StyleDefault)
			}
		}
		rows, drewKitty = msg.kittyRows, true
	} else if msg.kittyRows > 0 {
		img := kittyImageFor(msg.URL.String())
		if kittyEnsurePlaced(img, msg.kittyPNG, msg.kittyCols, msg.kittyRows) {
			// Foreground color carries the image ID; diacritics carry the
			// cell's position in the virtual placement grid.
			style := tcell.StyleDefault.Foreground(tcell.NewRGBColor(
				int32(img.id>>16&0xff), int32(img.id>>8&0xff), int32(img.id&0xff)))
			for y := 0; y < msg.kittyRows; y++ {
				for x := 0; x < msg.kittyCols; x++ {
					screen.SetContent(x, y, kittyPlaceholder,
						[]rune{kittyRowColDiacritics[y], kittyRowColDiacritics[x]}, style)
				}
			}
			rows = msg.kittyRows
			drewKitty = true
		}
		// Terminal refused; fall back to text until the next recalculation.
	}
	if !drewKitty {
		for y, line := range msg.buffer {
			line.Draw(screen, 0, y)
		}
	}
	if msg.linkLine != nil {
		msg.linkLine.Draw(screen, 0, rows)
	}
}
