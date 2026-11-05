package tui

import (
	"sync"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

// ConfirmModal asks a yes/no question. y or the right button confirms; n, Esc or the left button cancels.
type ConfirmModal struct {
	mauview.Component

	parent *MainView
	answer chan bool
	once   sync.Once
}

// Confirm shows a question and waits for the answer. It blocks, so call it from a goroutine, not the UI thread.
func (view *MainView) Confirm(title string, lines []string, yesLabel string) bool {
	modal := NewConfirmModal(view, title, lines, yesLabel)
	view.ShowModal(modal)
	view.parent.Render()
	return <-modal.answer
}

func NewConfirmModal(parent *MainView, title string, lines []string, yesLabel string) *ConfirmModal {
	modal := &ConfirmModal{parent: parent, answer: make(chan bool, 1)}
	rows := []int{1}
	for range lines {
		rows = append(rows, 1)
	}
	rows = append(rows, 1, 1, 1) // gap, buttons, bottom padding
	form := mauview.NewForm()
	form.SetColumns([]int{1, 21, 2, 21, 1}).SetRows(rows)
	for i, line := range lines {
		form.AddComponent(mauview.NewTextField().SetText(line), 1, 1+i, 3, 1)
	}
	buttons := 1 + len(lines) + 1
	// Cancel is added first so it has focus: Enter on its own never does the risky thing.
	form.AddFormItem(mauview.NewButton("Cancel (n)").SetOnClick(func() { modal.done(false) }), 1, buttons, 1, 1)
	form.AddFormItem(mauview.NewButton(yesLabel+" (y)").SetOnClick(func() { modal.done(true) }), 3, buttons, 1, 1)

	center := mauview.Center(mauview.NewBox(form).SetTitle(title), 50, len(rows)+2).SetAlwaysFocusChild(true)
	center.Focus()
	form.FocusNextItem()
	modal.Component = center
	return modal
}

func (modal *ConfirmModal) done(yes bool) {
	modal.once.Do(func() {
		modal.parent.HideModal()
		modal.answer <- yes
	})
}

func (modal *ConfirmModal) OnKeyEvent(event mauview.KeyEvent) bool {
	switch event.Key() {
	case tcell.KeyEscape:
		modal.done(false)
		return true
	case tcell.KeyRune:
		switch unicode.ToLower(event.Rune()) {
		case 'y':
			modal.done(true)
			return true
		case 'n':
			modal.done(false)
			return true
		}
	}
	return modal.Component.OnKeyEvent(event)
}
