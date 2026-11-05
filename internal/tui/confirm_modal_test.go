package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestConfirmModalKeys(t *testing.T) {
	cases := []struct {
		name string
		key  *tcell.EventKey
		want bool
	}{
		{"y confirms", tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone), true},
		{"Y confirms", tcell.NewEventKey(tcell.KeyRune, 'Y', tcell.ModNone), true},
		{"n cancels", tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone), false},
		{"Esc cancels", tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), false},
		{"Enter picks the focused Cancel", tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			view := &MainView{}
			modal := NewConfirmModal(view, "Leave this chat?", []string{"Leave Test group?"}, "Leave")
			view.ShowModal(modal)
			modal.OnKeyEvent(c.key)
			select {
			case got := <-modal.answer:
				if got != c.want {
					t.Errorf("got %v, want %v", got, c.want)
				}
			case <-time.After(time.Second):
				t.Fatal("no answer")
			}
			if view.modal != nil {
				t.Error("the dialog should close after answering")
			}
			modal.OnKeyEvent(c.key) // a second press must not block on the full channel
		})
	}
}
