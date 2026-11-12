package config

import "strings"

// Action is something a key can do, in one keybindings table.
type Action struct {
	Name string
	Doc  string
}

// Actions are every action, per keybindings table, in the order help and the docs list them.
var Actions = map[string][]Action{
	"main": {
		{"search_rooms", "Jump to a chat"},
		{"command_palette", "Command palette: every command, chat and key"},
		{"help", "Show every key and command"},
		{"next_room", "Next chat"},
		{"prev_room", "Previous chat"},
		{"next_active_room", "Next chat with new messages"},
		{"mark_unread", "Mark the chat unread and close it"},
		{"toggle_sidebar", "Show or hide the sidebar"},
		{"toggle_members", "Show or hide the members list"},
		{"show_bare", "Open the chat in a pager (plain text, for selecting with the mouse)"},
		{"scroll_up", "Scroll to the oldest loaded message"},
		{"scroll_down", "Scroll to the newest message"},
		{"add_newline", "New line in the message"},
		{"resync", "Reconnect to the backend"},
		{"force_quit", "Clear what you typed, or quit when there's nothing"},
		{"quit", "Quit"},
	},
	"room": {
		{"send", "Send the message"},
		{"select", "Select messages (visual mode)"},
		{"search", "Search this chat"},
		{"emoji", "Emoji picker"},
		{"mark_read", "Mark the chat seen (sends blue ticks)"},
		{"clear", "Cancel reply, edit or attachments, then close the chat"},
		{"scroll_up", "Scroll up half a page (loads older messages at the top)"},
		{"scroll_down", "Scroll down half a page"},
	},
	"visual": {
		{"select_prev", "Select the previous message"},
		{"select_next", "Select the next message"},
		{"confirm", "Do what visual mode was opened for (reply by default)"},
		{"open", "View, play or open the message (image, GIF, voice, video, file, link)"},
		{"reply", "Reply to it"},
		{"react", "React with any emoji"},
		{"quick_react_1", "React with 👍"},
		{"quick_react_2", "React with ❤️"},
		{"quick_react_3", "React with 😂"},
		{"quick_react_4", "React with 😮"},
		{"quick_react_5", "React with 😢"},
		{"quick_react_6", "React with 🙏"},
		{"copy", "Copy it (a whole range after range)"},
		{"copy_link", "Copy the first link in it"},
		{"edit", "Edit it (yours)"},
		{"delete", "Delete it (yours)"},
		{"retry", "Send it again (a failed message of yours)"},
		{"play", "Play a voice note or video"},
		{"open_external", "Open the media with the system app"},
		{"download", "Save the media to Downloads"},
		{"info", "When it was sent, delivered and seen"},
		{"hide", "Hide it in tuimessenger (/unclear brings it back)"},
		{"range", "Start or end a range, like vim's visual mode"},
		{"clear", "Drop the range, or leave visual mode"},
		{"exit", "Leave visual mode"},
	},
	"modal": {
		{"select_next", "Next item"},
		{"select_prev", "Previous item"},
		{"confirm", "Choose it"},
		{"cancel", "Close"},
	},
}

// FixedKey is a key that can't be changed (terminal plumbing, or only where nothing else uses keys).
type FixedKey struct {
	Group, Keys, Doc string
}

// FixedKeys are listed by help and the docs next to the configurable ones.
var FixedKeys = []FixedKey{
	{"Typing", "Shift+Enter", "New line"},
	{"Typing", "Alt+Backspace", "Delete a word"},
	{"Typing", "Alt+← / Alt+→", "Move by word"},
	{"Typing", "Tab", "Complete an :emoji: shortcode"},
	{"Typing", "Ctrl+z", "Undo (brings back cleared text)"},
	{"Typing", "Ctrl+v", "Paste; copied files or an image are attached"},
	{"Typing", "Backspace", "With nothing typed: remove the last attached file"},
	{"Typing", "/", "Commands (listed below)"},
	{"Home screen", "↑ / ↓", "Browse chats"},
	{"Home screen", "1–8", "Open an unread chat"},
	{"Popups", "↑ / ↓, Tab", "Move"},
	{"Popups", "Enter", "Choose"},
	{"Popups", "Esc", "Close"},
	{"Popups", "@ > ?", "In Ctrl+p: only chats, commands or keys"},
}

// KeybindingActions are the action names each keybindings table can use.
func KeybindingActions() map[string][]string {
	out := map[string][]string{}
	for section, actions := range Actions {
		for _, a := range actions {
			out[section] = append(out[section], a.Name)
		}
	}
	return out
}

// ActionDoc describes an action of a table.
func ActionDoc(section, name string) string {
	for _, a := range Actions[section] {
		if a.Name == name {
			return a.Doc
		}
	}
	return ""
}

// KeyLabel is how a key is shown: Ctrl+m and Ctrl+; arrive as Ctrl+^ and Ctrl+_ (the keys tuimessenger binds), so
// they're shown as what you press.
func KeyLabel(key string) string {
	switch key {
	case "Ctrl+^":
		return "Ctrl+m"
	case "Ctrl+_":
		return "Ctrl+;"
	}
	if i := strings.LastIndexByte(key, '+'); i > 0 && len(key)-i == 2 {
		return key[:i+1] + strings.ToLower(key[i+1:])
	}
	return key
}
