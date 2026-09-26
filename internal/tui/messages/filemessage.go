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

	"github.com/gdamore/tcell/v2"
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

	URL         id.ContentURI
	IsEncrypted bool

	eventID id.EventID

	imageData   []byte
	downloading bool
	buffer      []tstring.TString

	kittyPNG  []byte
	kittyCols int
	kittyRows int

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
	return newUIMessage(room, evt, content, "", &FileMessage{
		Type:        content.MsgType,
		Body:        content.Body,
		URL:         url,
		IsEncrypted: isEncrypted,
		eventID:     evt.ID,
		matrix:      matrix,
	})
}

func (msg *FileMessage) Clone() MessageRenderer {
	data := make([]byte, len(msg.imageData))
	copy(data, msg.imageData)
	return &FileMessage{
		Body:        msg.Body,
		URL:         msg.URL,
		IsEncrypted: msg.IsEncrypted,
		imageData:   data,
		matrix:      msg.matrix,
	}
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
		if pngData, err := encodeToPNG(data); err == nil {
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
}

func (msg *FileMessage) Height() int {
	if msg.kittyRows > 0 {
		return msg.kittyRows
	}
	return len(msg.buffer)
}

func (msg *FileMessage) Draw(screen mauview.Screen, _ *UIMessage) {
	if msg.kittyRows > 0 {
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
			return
		}
		// Terminal refused; fall back to text until the next recalculation.
	}
	for y, line := range msg.buffer {
		line.Draw(screen, 0, y)
	}
}
