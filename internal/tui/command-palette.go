package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"codeberg.org/tslocum/cbind"
	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/widget"
)

type paletteSection int

const (
	sectionCommands paletteSection = iota
	sectionChats
	sectionKeys
)

var sectionNames = []string{"Commands", "Chats", "Keys"}

// paletteChatsUnfiltered caps chats shown without the @ prefix.
const paletteChatsUnfiltered = 6

type paletteItem struct {
	section   paletteSection
	title     string
	alias     string // searchable; shown unless aliases are hidden
	showAlias bool
	detail    string
	icon      string
	iconColor tcell.Color
	run       func()
}

// paletteRow is an item or a section header (item == nil).
type paletteRow struct {
	item   *paletteItem
	header string
}

// CommandPalette is Ctrl+p: every command, chat and key in one search.
type CommandPalette struct {
	parent *MainView
	items  []*paletteItem
	query  []rune

	rows     []paletteRow
	selected int
	scroll   int

	// Last drawn geometry, for mouse hits.
	x, y, w, h, listY, listH int
}

func NewCommandPalette(view *MainView) *CommandPalette {
	p := &CommandPalette{parent: view}
	p.items = append(p.commandItems(), p.chatItems()...)
	p.items = append(p.items, keyItems(view.config)...)
	p.refilter()
	return p
}

// keyHint formats the bindings for a main action, Ctrl first: "Ctrl+r".
func keyHint(bindings map[config.Keybind]string, action string) string {
	var hints []string
	for kb, act := range bindings {
		if act != action {
			continue
		}
		enc, err := cbind.Encode(kb.Mod, kb.Key, kb.Ch)
		if err != nil {
			continue
		}
		if i := strings.LastIndexByte(enc, '+'); i != -1 && len(enc)-i == 2 {
			enc = enc[:i+1] + strings.ToLower(enc[i+1:])
		}
		hints = append(hints, enc)
	}
	sort.Slice(hints, func(i, j int) bool {
		ci, cj := strings.HasPrefix(hints[i], "Ctrl"), strings.HasPrefix(hints[j], "Ctrl")
		if ci != cj {
			return ci
		}
		return hints[i] < hints[j]
	})
	if len(hints) == 0 {
		return ""
	}
	return hints[0]
}

func (p *CommandPalette) commandItems() []*paletteItem {
	view := p.parent
	main := view.config.Keybindings.Main
	cmd := func(title, detail string, run func()) *paletteItem {
		return &paletteItem{section: sectionCommands, title: title, detail: detail, run: run}
	}
	items := []*paletteItem{
		cmd("Search chats", keyHint(main, "search_rooms"), func() { view.ShowModal(NewFuzzySearchModal(view, 42, 12)) }),
		cmd("Resync with server", keyHint(main, "resync"), func() { go view.parent.Resync() }),
		cmd("Next chat with activity", keyHint(main, "next_active_room"), func() { view.SwitchRoom(view.roomList.NextWithActivity()) }),
	}
	sidebar := "Hide sidebar"
	if view.config.Preferences.HideRoomList {
		sidebar = "Show sidebar"
	}
	items = append(items, cmd(sidebar, keyHint(main, "toggle_sidebar"), view.toggleSidebar))
	aliases := "Hide aliases"
	if view.config.Preferences.HideAliases {
		aliases = "Show aliases"
	}
	items = append(items, cmd(aliases, "", func() {
		view.config.Preferences.HideAliases = !view.config.Preferences.HideAliases
		go view.config.Save()
		RequestRepaint()
	}))
	members := "Show members list"
	if view.config.Preferences.ShowUserList {
		members = "Hide members list"
	}
	items = append(items, cmd(members, keyHint(main, "toggle_members"), func() {
		view.config.Preferences.ShowUserList = !view.config.Preferences.ShowUserList
		RequestRepaint()
	}))
	layout := "Classic layout (time and name columns)"
	if view.config.Preferences.ClassicLayout {
		layout = "Bubble layout (mine right, theirs left)"
	}
	styles := []string{"", "absolute", "off"}
	labels := map[string]string{"": "relative (now, 5m, 3h)", "absolute": "absolute (22:16)", "off": "off (selected only)"}
	cur := view.config.Preferences.TimestampStyle
	next := styles[(slices.Index(styles, cur)+1)%len(styles)]
	items = append(items, cmd("Timestamps: "+labels[cur]+" → "+labels[next], "", func() {
		view.config.Preferences.TimestampStyle = next
		go view.config.Save()
		RequestRepaint()
	}))
	items = append(items, cmd(layout, "", func() {
		view.config.Preferences.ClassicLayout = !view.config.Preferences.ClassicLayout
		go view.config.Save()
		RequestRepaint()
	}))
	room := view.currentRoom
	if room != nil {
		muted := view.roomList.IsMuted(room.Room.ID)
		privacy := chatPrivacy(room.Room)
		aliasTitle := "Set an alias for this chat (searchable)"
		if current := chatAlias(room.Room); current != "" {
			aliasTitle = "Change alias (" + current + ")"
		}
		cleared := !clearedAt(room.Room).IsZero()
		muteTitle, muteCmd := "Mute this chat", "/mute"
		if muted {
			muteTitle, muteCmd = "Unmute this chat", "/unmute"
		}
		clearTitle, clearCmd := "Clear history in gomuks", "/clear"
		if cleared {
			clearTitle, clearCmd = "Show cleared history", "/unclear"
		}
		items = append(items,
			cmd("Search this chat", "Ctrl+f", room.OpenSearch),
			cmd("Insert emoji", "Ctrl+e", room.OpenEmojiPicker),
			cmd("Open chat in pager", keyHint(main, "show_bare"), func() { view.ShowBare(room) }),
			cmd("Mark chat seen (sends blue ticks)", "Ctrl+m", room.MarkReadExplicit),
			cmd("Mark chat unread", keyHint(main, "mark_unread"), view.MarkUnread),
			cmd(aliasTitle, "/alias", func() {
				room.input.SetText("/alias " + chatAlias(room.Room))
				room.input.SetCursorOffset(-1)
				room.input.Focus()
			}),
			cmd("Share typing in this chat: "+onOff(privacy.Typing)+" → "+onOff(!privacy.Typing), "/typing",
				func() { go room.SetPrivacy(func(p *ChatPrivacy) { p.Typing = !p.Typing }) }),
			cmd("Share seen in this chat: "+onOff(privacy.ReadReceipts)+" → "+onOff(!privacy.ReadReceipts), "/seen",
				func() { go room.SetPrivacy(func(p *ChatPrivacy) { p.ReadReceipts = !p.ReadReceipts }) }),
			cmd(muteTitle, muteCmd, func() { go room.SetMuted(!muted) }),
			cmd(clearTitle, clearCmd, func() { go room.SetCleared(!cleared) }),
			cmd("Attach files…", "/upload", func() { go room.AttachPicked() }),
			cmd("Attach copied files or image", "Ctrl+v", func() { go room.AttachClipboard() }),
			cmd("Reply to a message", "r", func() { room.StartSelecting(SelectReply, "") }),
			cmd("React to a message", "+", func() { room.StartSelecting(SelectReact, "") }),
			cmd("Edit a message", "e", func() { room.StartSelecting(SelectEdit, "") }),
			cmd("Delete a message", "x", func() { room.StartSelecting(SelectRedact, "") }),
			cmd("Copy a message", "y", func() { room.StartSelecting(SelectCopy, "clipboard") }),
			cmd("Play a voice note or video", "p", func() { room.StartSelecting(SelectPlay, "") }),
			cmd("Download media", "d", func() { room.StartSelecting(SelectDownload, "") }),
			cmd("Close chat", "Escape", view.CloseCurrentRoom),
		)
		items = append(items,
			cmd("Mute sync: mute on WhatsApp, Instagram, Telegram", "/mute-sync", func() { go room.SyncMutes() }),
			cmd("Mute prune: delete rules of left chats", "/mute-prune", func() { go room.PruneMutes() }),
			cmd("Delete played media cache", "/cache", func() { go room.ClearMediaCache() }),
		)
	}
	return append(items, cmd("Quit", keyHint(main, "force_quit"), view.parent.Finish))
}

func (p *CommandPalette) chatItems() []*paletteItem {
	view := p.parent
	muted := view.roomList.mutedSet()
	var items []*paletteItem
	for _, room := range applyLocalSeen(view.matrix, view.matrix.ReversedRoomList.Current()) {
		icon, color := BridgeIconColor(room.Bridge)
		var notes []string
		if room.UnreadMessages > 0 {
			notes = append(notes, fmt.Sprintf("%d unread", room.UnreadMessages))
		} else if room.MarkedUnread {
			notes = append(notes, "unread")
		}
		if muted[room.RoomID] {
			notes = append(notes, "muted")
		}
		if view.currentRoom != nil && view.currentRoom.Room.ID == room.RoomID {
			notes = append(notes, "open")
		}
		roomID := room.RoomID
		items = append(items, &paletteItem{
			section: sectionChats, title: room.Name, detail: strings.Join(notes, " · "),
			alias: aliasOf(view.matrix, roomID), showAlias: !view.config.Preferences.HideAliases,
			icon: icon, iconColor: color,
			run: func() { view.SwitchRoom(roomID) },
		})
	}
	return items
}

// keyItems is the reference list: keys that aren't commands of their own.
func keyItems(cfg *config.Config) []*paletteItem {
	refs := [][2]string{
		{"Command palette", "Ctrl+p"},
		{"Browse chats (home screen)", "↑ ↓"},
		{"Open unread chat (home screen)", "1–8"},
		{"Mark chat unread (also while browsing)", "Alt+u"},
		{"Previous / next chat", keyHint(cfg.Keybindings.Main, "prev_room") + " / " + keyHint(cfg.Keybindings.Main, "next_room")},
		{"Select messages (visual mode)", "Ctrl+; or Alt+v"},
		{"Visual: move", "j / k"},
		{"Visual: select a range", "v"},
		{"Visual: quick react", "1–6"},
		{"Visual: reaction popup", "+"},
		{"Visual: view, play or open (image, voice, video, file, link)", "Space"},
		{"Visual: hide message", "h"},
		{"Visual: copy link", "u"},
		{"Visual: message info (sent, delivered, seen)", "i"},
		{"Emoji picker (recent, / to search)", "Ctrl+e"},
		{"New line", "Shift+Enter"},
		{"Clear input (undo: Ctrl+z)", "Ctrl+c"},
		{"Scroll up / down", "PageUp / PageDown"},
	}
	items := make([]*paletteItem, len(refs))
	for i, ref := range refs {
		items[i] = &paletteItem{section: sectionKeys, title: ref[0], detail: ref[1]}
	}
	return items
}

// splitQuery reads a leading @ (chats), > (commands) or ? (keys) filter.
func splitQuery(q string) (section paletteSection, text string, filtered bool) {
	if q == "" {
		return 0, "", false
	}
	switch q[0] {
	case '@':
		return sectionChats, q[1:], true
	case '>':
		return sectionCommands, q[1:], true
	case '?':
		return sectionKeys, q[1:], true
	}
	return 0, q, false
}

// matchScore ranks a title: 0 prefix, 1 word start, 2 substring, 3+ fuzzy; -1 no match.
func matchScore(title, query string) int {
	if query == "" {
		return 0
	}
	lt, lq := strings.ToLower(title), strings.ToLower(strings.TrimSpace(query))
	switch idx := strings.Index(lt, lq); {
	case idx == 0:
		return 0
	case idx > 0 && !unicode.IsLetter(rune(lt[idx-1])):
		return 1
	case idx > 0:
		return 2
	}
	if fuzzy.MatchFold(lq, lt) {
		return 3 + fuzzy.LevenshteinDistance(lq, lt)
	}
	return -1
}

func (p *CommandPalette) refilter() {
	section, text, filtered := splitQuery(string(p.query))
	type scored struct {
		item  *paletteItem
		score int
		order int
	}
	bySection := make([][]scored, len(sectionNames))
	for i, item := range p.items {
		if filtered && item.section != section {
			continue
		}
		s := matchScore(item.title, text)
		if item.alias != "" {
			if as := matchScore(item.alias, text); as >= 0 && (s < 0 || as < s) {
				s = as
			}
		}
		if s >= 0 {
			bySection[item.section] = append(bySection[item.section], scored{item, s, i})
		}
	}
	p.rows = p.rows[:0]
	for sec, list := range bySection {
		if text != "" {
			slices.SortStableFunc(list, func(a, b scored) int { return a.score - b.score })
		}
		if !filtered && paletteSection(sec) == sectionChats && len(list) > paletteChatsUnfiltered {
			list = list[:paletteChatsUnfiltered]
		}
		if len(list) == 0 {
			continue
		}
		if !filtered {
			p.rows = append(p.rows, paletteRow{header: sectionNames[sec]})
		}
		for _, s := range list {
			p.rows = append(p.rows, paletteRow{item: s.item})
		}
	}
	p.selected, p.scroll = -1, 0
	p.move(1)
}

// move steps the selection over items only, wrapping around.
func (p *CommandPalette) move(dir int) {
	n := len(p.rows)
	if n == 0 {
		p.selected = -1
		return
	}
	i := p.selected
	for range n {
		i = (i + dir + n) % n
		if p.rows[i].item != nil {
			p.selected = i
			return
		}
	}
}

func (p *CommandPalette) run() {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return
	}
	item := p.rows[p.selected].item
	p.parent.HideModal()
	RequestRepaint()
	if item.run != nil {
		item.run()
	}
	p.parent.parent.Render()
}

func (p *CommandPalette) OnKeyEvent(event mauview.KeyEvent) bool {
	switch event.Key() {
	case tcell.KeyEscape:
		p.parent.HideModal()
		RequestRepaint()
	case tcell.KeyEnter:
		p.run()
	case tcell.KeyDown, tcell.KeyTab, tcell.KeyCtrlN:
		p.move(1)
	case tcell.KeyUp, tcell.KeyBacktab, tcell.KeyCtrlP:
		p.move(-1)
	case tcell.KeyPgDn:
		for range max(1, p.listH-1) {
			p.move(1)
		}
	case tcell.KeyPgUp:
		for range max(1, p.listH-1) {
			p.move(-1)
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.refilter()
		}
	case tcell.KeyCtrlU:
		p.query = p.query[:0]
		p.refilter()
	case tcell.KeyRune:
		p.query = append(p.query, event.Rune())
		p.refilter()
	case tcell.KeyCtrlC:
		p.parent.HideModal()
		RequestRepaint()
	}
	return true
}

func (p *CommandPalette) OnMouseEvent(event mauview.MouseEvent) bool {
	x, y := event.Position()
	switch event.Buttons() {
	case tcell.WheelDown:
		p.move(1)
	case tcell.WheelUp:
		p.move(-1)
	case tcell.Button1:
		if event.HasMotion() {
			return true
		}
		if x < p.x || x >= p.x+p.w || y < p.y || y >= p.y+p.h {
			p.parent.HideModal()
			RequestRepaint()
			return true
		}
		if i := p.scroll + y - p.listY; y >= p.listY && i < len(p.rows) && p.rows[i].item != nil {
			p.selected = i
			p.run()
		}
	}
	return true
}

func (p *CommandPalette) OnPasteEvent(event mauview.PasteEvent) bool {
	p.query = append(p.query, []rune(strings.ReplaceAll(event.Text(), "\n", " "))...)
	p.refilter()
	return true
}

func (p *CommandPalette) Focus() {}
func (p *CommandPalette) Blur()  {}

func (p *CommandPalette) Draw(screen mauview.Screen) {
	sw, sh := screen.Size()
	if sw < 24 || sh < 8 {
		return
	}
	p.w = min(sw-4, 72)
	p.x = (sw - p.w) / 2
	p.y = sh / 6
	maxList := max(3, sh*3/5-4)
	p.listH = max(1, min(len(p.rows), maxList))
	p.h = p.listH + 4
	p.listY = p.y + 3
	drawThinBox(screen, p.x, p.y, p.w, p.h, " \uf120 Command palette ")

	inner := p.w - 2
	accent := tcell.StyleDefault.Foreground(ColorComposerBar).Bold(true)
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	qx := p.x + 1
	widget.WriteLine(screen, mauview.AlignLeft, " › ", qx, p.y+1, 3, accent)
	query := string(p.query)
	widget.WriteLine(screen, mauview.AlignLeft, query, qx+3, p.y+1, inner-4, tcell.StyleDefault)
	screen.ShowCursor(qx+3+runewidth.StringWidth(query), p.y+1)
	if len(p.query) == 0 {
		hint := "@ chats  > commands  ? keys "
		widget.WriteLine(screen, mauview.AlignRight, hint, qx, p.y+1, inner, dim)
	}
	for cx := p.x + 1; cx < p.x+p.w-1; cx++ {
		screen.SetContent(cx, p.y+2, '─', nil, tcell.StyleDefault.Foreground(ColorBorder))
	}

	if len(p.rows) == 0 {
		widget.WriteLine(screen, mauview.AlignLeft, "  No matches", qx, p.listY, inner, dim)
		return
	}
	if p.selected < p.scroll {
		p.scroll = p.selected
		// Keep the header above the first item of a section visible.
		if p.scroll > 0 && p.rows[p.scroll-1].item == nil {
			p.scroll--
		}
	} else if p.selected >= p.scroll+p.listH {
		p.scroll = p.selected - p.listH + 1
	}
	for i := 0; i < p.listH && p.scroll+i < len(p.rows); i++ {
		p.drawRow(screen, p.rows[p.scroll+i], p.scroll+i == p.selected, qx, p.listY+i, inner)
	}
}

func (p *CommandPalette) drawRow(screen mauview.Screen, row paletteRow, selected bool, x, y, width int) {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	if row.item == nil {
		widget.WriteLine(screen, mauview.AlignLeft, " "+row.header, x, y, width, dim.Bold(true))
		return
	}
	base := tcell.StyleDefault
	if selected {
		base = tcell.StyleDefault.Background(ColorHighlight).Foreground(ColorOnHighlight)
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x, y, width, base)
	}
	item := row.item
	detailW := runewidth.StringWidth(item.detail)
	if detailW > 0 {
		detailStyle := dim
		if selected {
			detailStyle = base
		}
		widget.WriteLine(screen, mauview.AlignLeft, item.detail, x+width-detailW-1, y, detailW, detailStyle)
	}
	titleX := x + 2
	if item.icon != "" {
		iconStyle := base.Foreground(item.iconColor)
		if selected {
			iconStyle = base
		}
		widget.WriteLine(screen, mauview.AlignLeft, item.icon, titleX, y, 2, iconStyle)
		titleX += 2
	}
	titleMax := x + width - titleX - detailW - 2
	if titleMax > 0 {
		alias, aliasStyle := "", aliasDim(base)
		if item.showAlias {
			alias = item.alias
		}
		if selected {
			aliasStyle = base
		}
		writeNameAlias(screen, titleX, y, titleMax, item.title, alias, base.Bold(selected), aliasStyle)
	}
}
