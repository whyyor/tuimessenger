<p align="center">
  <img src="assets/icon.svg" alt="tuimessenger icon" width="128">
</p>

<h1 align="center">tuimessenger</h1>

<p align="center">
All your networks in one terminal app: WhatsApp, Telegram, Slack and Instagram.
</p>

https://github.com/user-attachments/assets/bb0a1f29-8515-4bcb-99df-c29424b06132

- **Every network in one app**, with your chats, sessions and media kept on your machine.
- **Keyboard first:** Ctrl+k jumps to any chat, Ctrl+p has every command; reply, react and open photos without the
  mouse.
- **AI help when you want it:** let your coding agent read your chats and draft replies
  (`tuimessenger integration install claude`, `codex` or `opencode`).
- **Light:** under 100 MB of RAM with all your networks connected, where desktop messengers often take 1.5–2 GB.

> ⚠️ Bridges carry a low risk of the network flagging or banning your account. Proceed with caution.

## Install

**Script** (macOS and Linux, no sudo; installs to `~/.local/bin`, run it again to update):

```sh
curl -fsSL https://raw.githubusercontent.com/whyyor/tuimessenger/main/install.sh | sh
```

**Homebrew** (macOS and Linux):

```sh
brew install whyyor/tap/tuimessenger
```

**Download:** grab the archive for your system from [Releases](https://github.com/whyyor/tuimessenger/releases),
unpack it and put `tuimessenger` somewhere on your `PATH` (e.g. `~/.local/bin`). The binaries aren't signed by Apple,
so on macOS a browser download is blocked the first time; clear that once with
`xattr -d com.apple.quarantine ./tuimessenger` (Homebrew installs don't need this).

**Nix:**

```sh
nix run github:whyyor/tuimessenger -- tui          # try it
nix profile install github:whyyor/tuimessenger    # install it
```

**From source:** see [Building](#building).

Linux binaries need glibc 2.35 or newer (Ubuntu 22.04, Debian 12, Raspberry Pi OS and later). A
[Nerd Font](https://www.nerdfonts.com) in your terminal makes the icons show up; [yazi](https://yazi-rs.github.io)
is used as the file picker when it's installed.

## Quick start

1. **Start it:** `tuimessenger tui`, choose **Complete setup** (backend and client on this machine) and pick a
   password. The password protects the local server; other clients on this machine log in with it.
2. **Connect your networks:** Ctrl+p → **Networks** → pick one and follow the steps: a QR code or pairing code for
   WhatsApp, phone number and code for Telegram, a browser login for Slack and Instagram. The browser login opens
   Chrome, Chromium, Brave, Edge or Helium (`TUIMESSENGER_BROWSER` picks another) and closes it when you're done;
   without one, you paste the cookies instead. For Telegram, first create your keys: log in at
   [my.telegram.org](https://my.telegram.org) → API development tools → create an app (any name) → paste `api_id` and
   `api_hash` into the Networks panel.
3. **Chat:** Ctrl+k jumps to any chat, Enter sends, Ctrl+p shows every command and key.

Optional: `tuimessenger service install` runs the backend in the background (a launchd agent on macOS, a systemd user
unit on Linux), so your networks stay connected while the terminal client is closed. `tuimessenger service status`
shows how it's doing; `uninstall` removes it. After upgrading, run `service install` again to restart it.

## Keys

| Key | What it does |
|---|---|
| Ctrl+p | Command palette: every command, chat and key (`>` commands, `@` chats, `?` keys) |
| Ctrl+k | Jump to a chat (then Ctrl+p to peek without sending "seen") |
| ↑ ↓, 1–8 | Browse chats / open an unread chat (home screen) |
| Ctrl+; or Alt+v | Select messages (visual mode): j/k move, 1–6 quick react, + reactions, Space opens media, i message info |
| Ctrl+e | Emoji picker |
| Alt+u | Mark a chat unread |
| Shift+Enter | New line |
| Ctrl+c | Clear the input (Ctrl+z brings it back) |

## Where things are

| | macOS | Linux |
|---|---|---|
| Data (chats, sessions, media, `config.yaml`) | `~/Library/Application Support/tuimessenger` | `~/.local/share/tuimessenger` |
| Logs | `~/Library/Logs/tuimessenger` | `~/.local/state/tuimessenger` |

- `tui/config/terminal.yaml` in the data folder holds the client's preferences (changed from the TUI too);
  `tui/config/tuimessenger.yaml` holds your colours and pinned preferences and wins over it. Keep it in your
  dotfiles and link it here.
- `tui/client.db` is the client's copy of your chats.
- Logs: `tuimessenger.log` (backend), `terminal.log` and `client.log` (client), and `crash.log`, which keeps the
  stack trace if the app ever crashes. Please attach it to bug reports.
- macOS notifications come from small apps tuimessenger sets up in `~/.local/opt/tuimessenger/notifiers`: one per
  network (WhatsApp, Telegram, Slack, Instagram), so each shows its own icon and can be muted on its own in System
  Settings → Notifications, and `tuimessenger.app` for the rest.

## Building

Go 1.26+ and a C compiler (cgo, for SQLite). Always build with the `goolm` and `sqlite_fts5` tags (the Makefile does
this):

```sh
make build   # or: go build -tags goolm,sqlite_fts5 ./cmd/tuimessenger
make test
make vet
```

Without `goolm`, the build looks for the libolm C library (`olm/olm.h not found`).

Releases: a `vX.Y.Z` tag on `main` builds the binaries for macOS and Linux, publishes the GitHub release and updates
the Homebrew formula (`.github/workflows/release.yml`).

## What's next

- Signal.
- Running the backend on another machine, like a Raspberry Pi, with the terminal app connecting to it.
- More Linux testing: it builds and runs on x86_64 and arm64, but most use so far is on macOS. Reports welcome.

## Credits

tuimessenger is built on the work of others:

- **The terminal client** (`internal/tui`, `internal/rpc`, `internal/hicli`, `internal/rainbow`) comes from
  [gomuks](https://github.com/gomuks/gomuks) by Tulir Asokan and contributors, via the fork
  [whyyor/gomuks](https://github.com/whyyor/gomuks), and was imported with its git history.
- **The network bridges** are the [mautrix](https://github.com/mautrix) bridges (WhatsApp, Telegram, Meta, Slack)
  and the [mautrix-go](https://github.com/mautrix/go) bridge library.
- **Notifications on macOS** are shown by [terminal-notifier](https://github.com/julienXX/terminal-notifier) by
  Eloy Durán and Julien Blanchard (MIT), bundled with tuimessenger's name and icon; see `THIRD_PARTY_NOTICES.md`.

## License

tuimessenger is licensed under the GNU Affero General Public License, version 3 or (at your option) any later version
(see `LICENSE`). Imported files keep their
original license headers: parts of the client are under the Mozilla Public License 2.0 (`internal/hicli/LICENSE`).
