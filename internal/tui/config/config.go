// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2020 Tulir Asokan
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

package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"codeberg.org/tslocum/cbind"
	"github.com/gdamore/tcell/v2"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/ptr"
	"go.mau.fi/zeroconfig"
	"gopkg.in/yaml.v3"

	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// UserPreferences are the preferences: in terminal.yaml (Ctrl+p changes them) and settable in tuimessenger.yaml.
// doc and enum describe them for the schema, the docs and the checks; user:"-" keeps one out of tuimessenger.yaml
// (unused, or overwritten at startup). enum lists the YAML spellings; "relative" and "finder" mean "".
type UserPreferences struct {
	HideUserList bool `yaml:"hide_user_list" user:"-"`
	// ShowUserList turns on the member list (Ctrl+b); off by default.
	ShowUserList         bool   `yaml:"-"`
	HideRoomList         bool   `yaml:"hide_room_list" user:"-"`
	FilePicker           string `yaml:"file_picker" enum:"finder,yazi" default:"finder" doc:"What /upload and the attach key open to pick files: the system picker (Finder) or yazi in the terminal."`
	SidebarOnStart       bool   `yaml:"sidebar_on_start" doc:"Show the chat list sidebar when tuimessenger starts (Ctrl+s toggles it for the session)."`
	HideAliases          bool   `yaml:"hide_aliases" doc:"Keep your chat aliases out of lists and headers; they still match when searching."`
	HideTimestamp        bool   `yaml:"hide_timestamp" doc:"Hide message times in the classic layout."`
	BareMessageView      bool   `yaml:"bare_message_view" doc:"Plain text messages without images or formatting (also Ctrl+l / Alt+l, the pager)."`
	DisableImages        bool   `yaml:"disable_images" doc:"Don't show photos, stickers, GIFs and video previews inline; media shows as a link."`
	DisableTypingNotifs  bool   `yaml:"disable_typing_notifs" user:"-"`
	DisableEmojis        bool   `yaml:"disable_emojis" doc:"Don't turn :shortcodes: into emoji when sending, and no Tab completion for them."`
	DisableMarkdown      bool   `yaml:"disable_markdown" user:"-"`
	DisableHTML          bool   `yaml:"disable_html" user:"-"`
	DisableDownloads     bool   `yaml:"disable_downloads" doc:"Don't download media in the background for previews."`
	DisableNotifications bool   `yaml:"disable_notifications" doc:"No desktop notifications."`
	DisableShowURLs      bool   `yaml:"disable_show_urls" doc:"Don't show a link's address after its text when they differ."`

	InlineURLMode string `yaml:"inline_url_mode" enum:"auto,enable,disable" default:"auto" doc:"Clickable links (OSC 8): auto turns them on in terminals known to support them (Ghostty, kitty, iTerm2, foot, VTE)."`
	// ImageProtocol: "" auto-detects kitty graphics, "kitty" forces it, "ansi" forces half-blocks.
	ImageProtocol string `yaml:"image_protocol" enum:"auto,kitty,ansi" default:"auto" doc:"How images are drawn: auto uses the kitty graphics protocol where it works (kitty, Ghostty…), ansi uses coloured half-blocks everywhere."`
	ClassicLayout bool   `yaml:"classic_layout" doc:"Time and name columns instead of chat bubbles (yours right, theirs left)."`
	// TimestampStyle for bubbles: "" (relative: now, 5m, 3h, 2d), "absolute", "off".
	TimestampStyle string `yaml:"timestamp_style" enum:"relative,absolute,off" default:"relative" doc:"Times on chat bubbles: relative (now, 5m, 3h), absolute (22:16), or off (only on the selected message)."`
}

var InlineURLsProbablySupported bool

func init() {
	vteVersion, _ := strconv.Atoi(os.Getenv("VTE_VERSION"))
	term := os.Getenv("TERM")
	// Enable inline URLs by default on VTE 0.50.0+
	InlineURLsProbablySupported = vteVersion > 5000 ||
		os.Getenv("TERM_PROGRAM") == "iTerm.app" ||
		os.Getenv("TERM_PROGRAM") == "ghostty" ||
		term == "foot" ||
		term == "xterm-ghostty" ||
		term == "xterm-kitty"
}

func (up *UserPreferences) EnableInlineURLs() bool {
	return up.InlineURLMode == "enable" || (InlineURLsProbablySupported && up.InlineURLMode != "disable")
}

type Keybind struct {
	Mod tcell.ModMask
	Key tcell.Key
	Ch  rune
}

type ParsedKeybindings struct {
	Main   map[Keybind]string
	Room   map[Keybind]string
	Modal  map[Keybind]string
	Visual map[Keybind]string
}

type RawKeybindings struct {
	Main   map[string]string `yaml:"main,omitempty"`
	Room   map[string]string `yaml:"room,omitempty"`
	Modal  map[string]string `yaml:"modal,omitempty"`
	Visual map[string]string `yaml:"visual,omitempty"`
}

// Config contains the main config of tuimessenger.
type Config struct {
	NotifySound bool `yaml:"notify_sound"`

	Backspace1RemovesWord bool `yaml:"backspace1_removes_word" default:"true" doc:"Backspace deletes a whole word (in terminals that send it as ^H, e.g. Ctrl+Backspace in some)."`
	Backspace2RemovesWord bool `yaml:"backspace2_removes_word" doc:"Backspace deletes a whole word (in terminals that send it as DEL, i.e. most)."`

	// Theme and ThemeMode are what the Ctrl+p theme picker chose; tuimessenger.yaml's theme wins.
	Theme     string `yaml:"theme,omitempty"`
	ThemeMode string `yaml:"theme_mode,omitempty"`

	AlwaysClearScreen bool `yaml:"always_clear_screen"`

	LogConfig zeroconfig.Config `yaml:"log_config"`

	// Mode is how this machine was set up: "complete" runs the backend inside the TUI when none is running,
	// "self-hosted" only connects to one elsewhere. Empty (set up before modes existed): only connect.
	Mode string `yaml:"mode,omitempty"`

	Dir string `yaml:"-"`

	Preferences UserPreferences   `yaml:"preferences"`
	Keybindings ParsedKeybindings `yaml:"-"`

	rawKeybindings RawKeybindings
	nosave         bool
}

func GetConfigDirectory() string {
	if configHome := os.Getenv("TUIMESSENGER_CONFIG_HOME"); configHome != "" {
		return configHome
	} else if root := os.Getenv("TUIMESSENGER_ROOT"); root != "" {
		return filepath.Join(root, "config")
	}
	return filepath.Join(exerrors.Must(os.UserConfigDir()), "tuimessenger")
}

func GetLogDirectory() string {
	if logsHome := os.Getenv("TUIMESSENGER_LOGS_HOME"); logsHome != "" {
		return logsHome
	} else if root := os.Getenv("TUIMESSENGER_ROOT"); root != "" {
		return filepath.Join(root, "logs")
	} else if xdgStateHome := os.Getenv("XDG_STATE_HOME"); xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "tuimessenger")
	} else if runtime.GOOS == "darwin" {
		return filepath.Join(exerrors.Must(os.UserHomeDir()), "Library", "Logs", "tuimessenger")
	} else if runtime.GOOS == "windows" {
		return filepath.Join(exerrors.Must(os.UserCacheDir()), "logs")
	} else {
		return filepath.Join(exerrors.Must(os.UserHomeDir()), ".local", "state", "tuimessenger")
	}
}

// NewConfig creates a config in the default directories (or the ones from the TUIMESSENGER_* environment variables).
func NewConfig() *Config {
	return NewConfigAt(GetConfigDirectory(), GetLogDirectory())
}

// NewConfigAt creates a config that loads data from dir and writes terminal.log to logDir.
func NewConfigAt(dir, logDir string) *Config {
	return &Config{
		Dir: dir,

		NotifySound:           true,
		Backspace1RemovesWord: true,
		AlwaysClearScreen:     true,

		LogConfig: zeroconfig.Config{
			Writers: []zeroconfig.WriterConfig{{
				Type:   zeroconfig.WriterTypeFile,
				Format: zeroconfig.LogFormatJSON,
				FileConfig: zeroconfig.FileConfig{
					Filename:   filepath.Join(logDir, "terminal.log"),
					MaxSize:    100,
					MaxBackups: 10,
				},
			}},
			MinLevel: ptr.Ptr(zerolog.TraceLevel),
		},
	}
}

func (config *Config) LoadAll() {
	config.Load()
	config.LoadKeybindings()
}

// Load loads the config from config.yaml in the directory given to the config struct.
func (config *Config) Load() {
	err := config.load("config", config.Dir, "terminal.yaml", config)
	if err != nil {
		panic(fmt.Errorf("failed to load config.yaml: %w", err))
	}
}

func (config *Config) SaveAll() {
	config.Save()
}

// Save saves this config to config.yaml in the directory given to the config struct.
func (config *Config) Save() {
	config.save("config", config.Dir, "terminal.yaml", config)
}

//go:embed keybindings.yaml
var DefaultKeybindings string

func parseKeybindings(input map[string]string) (output map[Keybind]string) {
	output = make(map[Keybind]string, len(input))
	for shortcut, action := range input {
		mod, key, ch, err := cbind.Decode(shortcut)
		if err != nil {
			panic(fmt.Errorf("failed to parse keybinding %s -> %s: %w", shortcut, action, err))
		}
		// TODO find out if other keys are parsed incorrectly like this
		if key == tcell.KeyEscape {
			ch = 0
		}
		parsedShortcut := Keybind{
			Mod: mod,
			Key: key,
			Ch:  ch,
		}
		output[parsedShortcut] = action
	}
	return
}

func (config *Config) LoadKeybindings() {
	var inputConfig RawKeybindings

	err := yaml.Unmarshal([]byte(DefaultKeybindings), &inputConfig)
	if err != nil {
		panic(fmt.Errorf("failed to unmarshal default keybindings: %w", err))
	}
	_ = config.load("keybindings", config.Dir, "terminal-keybindings.yaml", &inputConfig)
	config.rawKeybindings = inputConfig
	config.parseAllKeybindings()
}

// ApplyKeybindings puts tuimessenger.yaml's keybindings (already checked) over the defaults.
func (config *Config) ApplyKeybindings(yours RawKeybindings) {
	MergeKeybindings(&config.rawKeybindings, yours)
	config.parseAllKeybindings()
}

func (config *Config) parseAllKeybindings() {
	config.Keybindings.Main = parseKeybindings(config.rawKeybindings.Main)
	config.Keybindings.Room = parseKeybindings(config.rawKeybindings.Room)
	config.Keybindings.Modal = parseKeybindings(config.rawKeybindings.Modal)
	config.Keybindings.Visual = parseKeybindings(config.rawKeybindings.Visual)
}

func (config *Config) SaveKeybindings() {
	config.save("keybindings", config.Dir, "terminal-keybindings.yaml", &config.Keybindings)
}

func (config *Config) load(name, dir, file string, target interface{}) error {
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		debug.Print("Failed to create", dir)
		return err
	}

	path := filepath.Join(dir, file)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		debug.Print("Failed to read", name, "from", path)
		return err
	}

	if strings.HasSuffix(file, ".yaml") {
		err = yaml.Unmarshal(data, target)
	} else {
		err = json.Unmarshal(data, target)
	}
	if err != nil {
		debug.Print("Failed to parse", name, "at", path)
		return err
	}
	return nil
}

func (config *Config) save(name, dir, file string, source interface{}) {
	if config.nosave {
		return
	}

	err := os.MkdirAll(dir, 0700)
	if err != nil {
		debug.Print("Failed to create", dir)
		panic(err)
	}
	var data []byte
	if strings.HasSuffix(file, ".yaml") {
		data, err = yaml.Marshal(source)
	} else {
		data, err = json.Marshal(source)
	}
	if err != nil {
		debug.Print("Failed to marshal", name)
		panic(err)
	}

	path := filepath.Join(dir, file)
	err = os.WriteFile(path, data, 0600)
	if err != nil {
		debug.Print("Failed to write", name, "to", path)
		panic(err)
	}
}
