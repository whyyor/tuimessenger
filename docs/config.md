# Configuring tuimessenger

Everything you can set lives in one file, **`tuimessenger.yaml`**: the theme, colours, preferences (sidebar, layout,
timestamps, notifications…) and keys.

## Where it is

| | Path |
|---|---|
| macOS and Linux | `~/.config/tuimessenger/tuimessenger.yaml` |
| with `XDG_CONFIG_HOME` | `$XDG_CONFIG_HOME/tuimessenger/tuimessenger.yaml` |
| anywhere else | set `TUIMESSENGER_CONFIG=/path/to/tuimessenger.yaml` |

The first time tuimessenger starts without one, it writes a **commented template** there: every setting, commented
out at its default. Uncomment a line to change it, then restart tuimessenger.

tuimessenger only reads this file (it never rewrites it), so it's safe to keep in your dotfiles and link it into
place.

## Check it

```sh
tuimessenger config        # where the file is, and any mistakes in it (exit code 1 if there are some)
tuimessenger config init   # write the commented template if there's no file yet
```

Mistakes never stop tuimessenger: it shows them in a yellow toast when it starts ("line 12: unknown preference
`sidebar_on_strat` (did you mean `sidebar_on_start`?)") and uses everything else.

## Editor checking and completion

The template's first line points editors at the [JSON Schema](config.schema.json):

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/whyyor/tuimessenger/main/docs/config.schema.json
```

With the YAML language server, your editor then completes setting names and values, shows what each does, and
underlines typos:

- **Neovim**: `yamlls` (e.g. via `nvim-lspconfig` / mason)
- **VS Code**: the *YAML* extension by Red Hat
- **Zed**, **Helix**: YAML support is built in (`yaml-language-server` on your `PATH`)

## What wins

1. tuimessenger's defaults
2. what you change in the app (Ctrl+p), saved in `terminal.yaml` in tuimessenger's data folder
3. **`tuimessenger.yaml`**, which always wins

A setting from `tuimessenger.yaml` can still be changed in Ctrl+p, but only until you restart; tuimessenger says so in
a toast. An older `tuimessenger.yaml` next to `terminal.yaml` (in the data folder's `tui/config/`) is still read;
`~/.config`'s wins over it.

## A short example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/whyyor/tuimessenger/main/docs/config.schema.json
theme: tokyonight
theme_mode: dark

preferences:
  sidebar_on_start: true
  file_picker: yazi
  timestamp_style: absolute

colors:
  accent: "#ff79c6"

keybindings:
  main:
    'Ctrl+n': next_room
    'Ctrl+s': none # remove a default
```

## Themes

`theme:` takes `tuimessenger` (the default), any of [opencode](https://opencode.ai)'s themes (listed below, bundled
under their MIT licence), or one of yours. **Ctrl+p → Theme** previews them live as you move through the list; Enter
keeps one, Escape puts the old one back. **Ctrl+p → Theme mode** switches between a theme's dark and light colours.

### Your own theme

Put it in `~/.config/tuimessenger/themes/<name>.json` (next to `tuimessenger.yaml`) and set `theme: <name>`. It uses
[opencode's theme format](https://opencode.ai/docs/themes/), so opencode themes work as they are:

```json
{
  "defs": { "pink": "#ff79c6", "night": "#1e1f29" },
  "theme": {
    "primary": "pink",
    "accent": { "dark": "#bd93f9", "light": "#6c4bb3" },
    "text": "#f8f8f2",
    "textMuted": "#6272a4",
    "background": "night",
    "backgroundPanel": "#191a21",
    "backgroundElement": "#282a36",
    "border": "#44475a",
    "success": "#50fa7b",
    "warning": "#f1fa8c",
    "error": "#ff5555",
    "info": "#8be9fd",
    "secondary": "#bd93f9",
    "markdownCode": "#50fa7b"
  }
}
```

A colour is `"#rrggbb"`, `"none"` (your terminal's own colour), a name from `defs`, another slot's name, or
`{ "dark": …, "light": … }`. `primary`, `accent`, `text` and `background` are required. How the slots map onto
tuimessenger:

| opencode slot | tuimessenger colours |
|---|---|
| `primary` | `accent` (composer bar), `selection` |
| `accent` | `sidebar_selection`, Ctrl+k's pill, `pill_pink` |
| `backgroundPanel` | `panel` (Ctrl+k, Ctrl+p) |
| `backgroundElement` | `selection_bar`, `code_background` |
| `border` | `border` |
| `text` / `textMuted` | `bar_text` / `dim_text` |
| `success` / `info` / `warning` / `error` | toasts, Ctrl+p's pill, unread counts, pills |
| `secondary` | `pill_violet` |
| `markdownCode` | `inline_code` |
| `background` (or `selectedListItemText`) | text on selections and pills |

tuimessenger draws on your terminal's own background, so a theme looks best with a terminal background close to the
theme's `background`. `colors:` in `tuimessenger.yaml` still wins over any theme.

## Keys

`keybindings:` has four tables: `main` (everywhere), `room` (in a chat), `visual` (selecting messages) and `modal`
(popups). Each maps a key to an action; your keys are added to the defaults below, and `none` removes a default:

```yaml
keybindings:
  room:
    'Alt+v': none
    'Ctrl+s': select
```

Press **?** (with an empty message box) or **F1** in tuimessenger to see every key, as they are with your changes.

## Keep the backend running

tuimessenger's backend (the bridges to WhatsApp, Telegram, Slack and Instagram) runs inside the app by default, so it
stops when you quit. To keep your networks connected all the time, and keep the app running even if a bridge ever
crashes, run it as a background service:

```sh
tuimessenger service install    # a launchd agent on macOS, a systemd user unit on Linux
tuimessenger service status
tuimessenger service uninstall
```

After upgrading tuimessenger, run `tuimessenger service install` again to restart it with the new version.

## Reference

Generated from tuimessenger's code, so it's always complete.

<!-- generated: options start (UPDATE_DOCS=1 go test ./internal/tui/config/) -->

### Top level

| Setting | Default | Description |
|---|---|---|
| `theme` | `tuimessenger` | Colour theme: tuimessenger, any of opencode's themes, or one of yours (themes/<name>.json next to this file, in opencode's format). Ctrl+p → Theme previews them live. |
| `theme_mode` | `dark` | Use a theme's dark or light colours (most opencode themes have both). One of: `dark`, `light`. |

### `preferences`

| Preference | Default | Values | Description |
|---|---|---|---|
| `file_picker` | `finder` | `finder`, `yazi` | What /upload and the attach key open to pick files: the system picker (Finder) or yazi in the terminal. |
| `sidebar_on_start` | `false` | `true`, `false` | Show the chat list sidebar when tuimessenger starts (Ctrl+s toggles it for the session). |
| `hide_aliases` | `false` | `true`, `false` | Keep your chat aliases out of lists and headers; they still match when searching. |
| `hide_timestamp` | `false` | `true`, `false` | Hide message times in the classic layout. |
| `bare_message_view` | `false` | `true`, `false` | Plain text messages without images or formatting (also Ctrl+l / Alt+l, the pager). |
| `disable_images` | `false` | `true`, `false` | Don't show photos, stickers, GIFs and video previews inline; media shows as a link. |
| `disable_emojis` | `false` | `true`, `false` | Don't turn :shortcodes: into emoji when sending, and no Tab completion for them. |
| `disable_downloads` | `false` | `true`, `false` | Don't download media in the background for previews. |
| `disable_notifications` | `false` | `true`, `false` | No desktop notifications. |
| `disable_show_urls` | `false` | `true`, `false` | Don't show a link's address after its text when they differ. |
| `inline_url_mode` | `auto` | `auto`, `enable`, `disable` | Clickable links (OSC 8): auto turns them on in terminals known to support them (Ghostty, kitty, iTerm2, foot, VTE). |
| `image_protocol` | `auto` | `auto`, `kitty`, `ansi` | How images are drawn: auto uses the kitty graphics protocol where it works (kitty, Ghostty…), ansi uses coloured half-blocks everywhere. |
| `classic_layout` | `false` | `true`, `false` | Time and name columns instead of chat bubbles (yours right, theirs left). |
| `timestamp_style` | `relative` | `relative`, `absolute`, `off` | Times on chat bubbles: relative (now, 5m, 3h), absolute (22:16), or off (only on the selected message). |
| `backspace1_removes_word` | `true` | `true`, `false` | Backspace deletes a whole word (in terminals that send it as ^H, e.g. Ctrl+Backspace in some). |
| `backspace2_removes_word` | `false` | `true`, `false` | Backspace deletes a whole word (in terminals that send it as DEL, i.e. most). |

### `colors`

The defaults are the `tuimessenger` theme's. A colour marked *follows* takes the other colour's value when neither the theme nor you set it.

| Colour | Default | Description |
|---|---|---|
| `accent` | `#a8a9eb` | Composer bar and prompts. |
| `selection` | `#8d70c2` | Background of the selected message and emoji. |
| `selection_text` | `#ffffff` | Text on the selection. |
| `sidebar_selection` | `#f92672` | The selected chat in the sidebar. |
| `sidebar_selection_text` | `#fff1f3` | Text of the selected chat. |
| `panel` | `#0e0e13` | Background of Ctrl+k, Ctrl+p and other popups. |
| `selection_bar` | `#1f1f28` | The selected row in Ctrl+k and Ctrl+p. |
| `border` | `#54546d` | Popup borders. |
| `dim_text` | `#8a8a94` | Status line, details and aliases. |
| `bar_text` | `#c8c9c5` | Text in bars and popups. |
| `switcher_pill` | `#f92672` | Ctrl+k's title pill and its ▌. *Follows `sidebar_selection`.* |
| `switcher_pill_text` | `#fff1f3` | Text on Ctrl+k's pill. *Follows `sidebar_selection_text`.* |
| `palette_pill` | `#adda78` | Ctrl+p's title pill. *Follows `toast_success`.* |
| `palette_accent` | `#adda78` | Ctrl+p's ▌ and magnifier. *Follows `palette_pill`.* |
| `palette_pill_text` | `#000000` | Text on Ctrl+p's pill. |
| `toast_success` | `#adda78` | Success toasts. |
| `toast_info` | `#a8a9eb` | Info toasts. |
| `toast_warning` | `#f9cc6c` | Warning toasts. |
| `toast_error` | `#fd6883` | Error toasts. |
| `unread_badge` | `#7fb4ca` | Unread counts. |
| `unread_mention` | `#e6c384` | Unread counts with a mention of you. |
| `pill_yellow` | `#e6c384` | Pills such as the Ctrl+m hint. |
| `pill_pink` | `#d27e99` | The search pill. |
| `pill_green` | `#98bb6c` | Green pills. |
| `pill_blue` | `#7e9cd8` | Blue pills. |
| `pill_violet` | `#a8a9eb` | The peek pill. |
| `pill_text` | `#1f1f28` | Text on pills. |
| `code_background` | `#1a1a22` | Code blocks and inline code. |
| `inline_code` | `#f38d70` | Inline code text. |

### Themes

`tuimessenger`, `aura`, `ayu`, `carbonfox`, `catppuccin`, `catppuccin-frappe`, `catppuccin-macchiato`, `cobalt2`, `cursor`, `dracula`, `everforest`, `flexoki`, `github`, `gruvbox`, `kanagawa`, `lucent-orng`, `material`, `matrix`, `mercury`, `monokai`, `nightowl`, `nord`, `one-dark`, `opencode`, `orng`, `osaka-jade`, `palenight`, `rosepine`, `solarized`, `synthwave84`, `tokyonight`, `vercel`, `vesper`, `zenburn`

### `keybindings`

The default keys, by table. Keys are written like `Ctrl+k`, `Alt+Enter`, `Shift+Tab`, `F1` or a single character.

#### `main`: Everywhere (unless a popup is open)

| Action | Default keys |
|---|---|
| `add_newline` | `Alt+Enter`, `Ctrl+Enter`, `Ctrl+j` |
| `command_palette` | `Ctrl+p` |
| `force_quit` | `Ctrl+c` |
| `mark_unread` | `Alt+u` |
| `next_active_room` | `Alt+a` |
| `next_room` | `Alt+Down`, `Ctrl+Down` |
| `prev_room` | `Alt+Up`, `Ctrl+Up` |
| `resync` | `Alt+r`, `Ctrl+r` |
| `scroll_down` | `Alt+End`, `Ctrl+End` |
| `scroll_up` | `Alt+Home`, `Ctrl+Home` |
| `search_rooms` | `Alt+k`, `Ctrl+k` |
| `show_bare` | `Alt+l`, `Ctrl+l` |
| `toggle_members` | `Ctrl+b` |
| `toggle_sidebar` | `Ctrl+s` |

#### `room`: In a chat

| Action | Default keys |
|---|---|
| `clear` | `Escape` |
| `scroll_down` | `Ctrl+d`, `Ctrl+n`, `PageDown` |
| `scroll_up` | `Ctrl+u`, `PageUp` |
| `search` | `Ctrl+f` |
| `select` | `Alt+v` |
| `send` | `Enter` |

#### `visual`: Selecting messages (visual mode)

| Action | Default keys |
|---|---|
| `clear` | `Escape` |
| `confirm` | `Enter`, `l` |
| `select_next` | `Down`, `j` |
| `select_prev` | `Up`, `k` |

#### `modal`: Popups and lists

| Action | Default keys |
|---|---|
| `cancel` | `Escape` |
| `confirm` | `Enter` |
| `select_next` | `Down`, `Tab` |
| `select_prev` | `Backtab`, `Up` |

<!-- generated: options end -->
