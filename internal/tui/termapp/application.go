// mauview - A Go TUI library based on tcell.
// Copyright © 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package termapp is mauview's Application (go.mau.fi/mauview v0.3.0, application.go and the event types from
// eventhandler.go), changed for tuimessenger: the screen reads keys through kittyTty, which turns on the Kitty
// keyboard protocol and translates it for tcell 2 (see kitty.go). The rest is unchanged.
package termapp

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

type Application struct {
	screenLock   sync.RWMutex
	screen       tcell.Screen
	keys         *kittyTty
	prevMouseEvt *tcell.EventMouse
	root         mauview.Component
	updates      chan interface{}
	redrawTicker *time.Ticker
	stop         chan struct{}
	waitForStop  chan struct{}
	alwaysClear  bool
}

const queueSize = 255

func NewApplication() *Application {
	return &Application{
		prevMouseEvt: &tcell.EventMouse{},
		updates:      make(chan interface{}, queueSize),
		redrawTicker: time.NewTicker(1 * time.Minute),
		stop:         make(chan struct{}, 1),
		alwaysClear:  true,
	}
}

func newScreen(events chan tcell.Event) (tcell.Screen, *kittyTty, error) {
	keys := newKittyTty()
	var screen tcell.Screen
	var err error
	if keys != nil {
		screen, err = tcell.NewTerminfoScreenFromTty(keys)
	} else {
		screen, err = tcell.NewScreen()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create screen: %w", err)
	} else if err = screen.Init(); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize screen: %w", err)
	}
	screen.EnableMouse()
	screen.EnablePaste()
	// After Init: tcell is on the alternate screen now, which has its own Kitty flags.
	keys.enable()
	go screen.ChannelEvents(events, nil)
	return screen, keys, nil
}

func (app *Application) SetRedrawTicker(tick time.Duration) {
	app.redrawTicker.Stop()
	app.redrawTicker = time.NewTicker(tick)
}

func (app *Application) Start() error {
	if app.root == nil {
		return errors.New("root component not set")
	}

	events := make(chan tcell.Event, queueSize)
	screen, keys, err := newScreen(events)
	if err != nil {
		return err
	}

	app.screenLock.Lock()
	app.screen = screen
	app.keys = keys
	app.screenLock.Unlock()
	app.waitForStop = make(chan struct{})

	defer func() {
		app.screenLock.Lock()
		app.screen = nil
		app.screenLock.Unlock()
		if screen != nil {
			keys.disable()
			screen.Fini()
		}
		close(app.waitForStop)
	}()

	var pasteBuffer strings.Builder
	var isPasting bool

	for {
		var redraw bool
		var clear bool
		select {
		case eventInterface := <-events:
			switch event := eventInterface.(type) {
			case *tcell.EventKey:
				if isPasting {
					switch event.Key() {
					case tcell.KeyRune:
						pasteBuffer.WriteRune(event.Rune())
					case tcell.KeyEnter:
						pasteBuffer.WriteByte('\n')
					}
				} else {
					redraw = app.root.OnKeyEvent(event)
				}
			case *tcell.EventPaste:
				if event.Start() {
					isPasting = true
					pasteBuffer.Reset()
				} else {
					customEvt := customPasteEvent{event, pasteBuffer.String()}
					isPasting = false
					pasteBuffer.Reset()
					redraw = app.root.OnPasteEvent(customEvt)
				}
			case *tcell.EventMouse:
				onlyButtons := event.Buttons() < tcell.WheelUp
				hasMotion := onlyButtons && app.prevMouseEvt.Buttons() == event.Buttons()
				customEvt := customMouseEvent{event, hasMotion}
				app.prevMouseEvt = event
				redraw = app.root.OnMouseEvent(customEvt)
			case *tcell.EventResize:
				clear = true
				redraw = true
			}
		case <-app.redrawTicker.C:
			redraw = true
		case updaterInterface := <-app.updates:
			switch updater := updaterInterface.(type) {
			case redrawUpdate:
				redraw = true
			case setRootUpdate:
				app.root = updater.newRoot
				focusable, ok := app.root.(mauview.Focusable)
				if ok {
					focusable.Focus()
				}
				redraw = true
				clear = true
			case suspendUpdate:
				// Programs run meanwhile (mpv, yazi, the shell) get ordinary keys.
				keys.disable()
				err = screen.Suspend()
				if err != nil {
					// This shouldn't fail
					panic(err)
				}
				updater.wait()
				err = screen.Resume()
				if err != nil {
					screen.Fini()
					fmt.Println("Failed to resume screen:", err)
					os.Exit(40)
				}
				keys.enable()
				redraw = true
				clear = true
			}
		case <-app.stop:
			return nil
		}
		select {
		case <-app.stop:
			return nil
		default:
		}
		if redraw {
			if clear || app.alwaysClear {
				screen.Clear()
			}
			screen.HideCursor()
			app.root.Draw(screen)
			screen.Show()
		}
	}
}

func (app *Application) Stop() {
	select {
	case app.stop <- struct{}{}:
	default:
	}
	<-app.waitForStop
}

func (app *Application) ForceStop() {
	app.screenLock.RLock()
	screen, keys := app.screen, app.keys
	app.screenLock.RUnlock()
	if screen != nil {
		keys.disable()
		screen.Fini()
	}
	select {
	case app.stop <- struct{}{}:
	default:
	}
}

type suspendUpdate struct {
	wait func()
}

type redrawUpdate struct{}

type setRootUpdate struct {
	newRoot mauview.Component
}

func (app *Application) Suspend(wait func()) {
	app.updates <- suspendUpdate{wait}
}

func (app *Application) Redraw() {
	app.updates <- redrawUpdate{}
}

func (app *Application) SetRoot(view mauview.Component) {
	app.screenLock.RLock()
	defer app.screenLock.RUnlock()
	if app.screen != nil {
		app.updates <- setRootUpdate{view}
	} else {
		app.root = view
		focusable, ok := app.root.(mauview.Focusable)
		if ok {
			focusable.Focus()
		}
	}
}

// Screen returns the main tcell screen currently used in the app.
func (app *Application) Screen() tcell.Screen {
	app.screenLock.RLock()
	screen := app.screen
	app.screenLock.RUnlock()
	return screen
}

func (app *Application) SetAlwaysClear(always bool) {
	app.alwaysClear = always
}

type customPasteEvent struct {
	*tcell.EventPaste
	text string
}

func (cpe customPasteEvent) Text() string {
	return cpe.text
}

type customMouseEvent struct {
	*tcell.EventMouse
	motion bool
}

func (cme customMouseEvent) HasMotion() bool {
	return cme.motion
}
