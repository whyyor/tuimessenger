package tui

import (
	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// keyOf is a key press as the keybindings tables store it. For a typed character Shift is part of the character
// ("R" arrives as R with or without Shift), so it's left out.
func keyOf(event mauview.KeyEvent) config.Keybind {
	key, ch, mod := event.Key(), event.Rune(), event.Modifiers()
	switch {
	case key == tcell.KeyRune:
		mod &^= tcell.ModShift
	case key > tcell.KeyNUL && key < ' ' && key != tcell.KeyBackspace && key != tcell.KeyTab &&
		key != tcell.KeyEnter && key != tcell.KeyEscape:
		// A control key, however it was encoded: Ctrl with its character, as the tables store it.
		ch, mod = rune(key), mod|tcell.ModCtrl
	}
	return config.Keybind{Key: key, Ch: ch, Mod: mod}
}

// isTypedChar is a plain character (no Ctrl or Alt): bound to an action, it only acts when you aren't typing.
func isTypedChar(event mauview.KeyEvent) bool {
	return event.Key() == tcell.KeyRune && event.Modifiers()&^tcell.ModShift == 0
}

// typing reports whether a character would go into a text field now: the message box has text, or chat search, the
// emoji picker or message info is open. Visual mode and the home screen aren't typing.
func (view *MainView) typing() bool {
	room := view.currentRoom
	if room == nil || room.selecting {
		return false
	}
	return room.input.GetText() != "" || room.search != nil || room.picker != nil || room.info != nil
}
