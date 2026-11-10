<p align="center">
  <img src="assets/icon.svg" alt="tuimessenger icon" width="128">
</p>

<h1 align="center">tuimessenger</h1>

<p align="center">
Your chats from WhatsApp, Telegram, Slack and Instagram in one terminal app, with the bridges running on your own
machine.
</p>

https://github.com/user-attachments/assets/bb0a1f29-8515-4bcb-99df-c29424b06132

## See it

| Jump to any chat with Ctrl+k, send photos with yazi | Reply and react |
|---|---|
| ![Ctrl+k to a chat, then /upload picks a photo in yazi and sends it](docs/demo/upload-yazi.gif) | ![Replying to a message and reacting with an emoji](docs/demo/reply-react.gif) |

| Photos open full screen, right in the terminal |
|---|
| ![A photo opened full screen inside the terminal](docs/demo/fullscreen-image.gif) |

Everything in these recordings is made-up demo data (`tuimessenger demo serve`, a developer build); the photos are from
[Unsplash](https://unsplash.com/license) via [Lorem Picsum](https://picsum.photos).

## Building

Always build with the `goolm` and `sqlite_fts5` tags (the Makefile does this):

```sh
make build   # or: go build -tags goolm,sqlite_fts5 ./cmd/tuimessenger
make test
make vet
```

Without `goolm`, the build looks for the libolm C library (`olm/olm.h not found`).

## AI agents

`tuimessenger agent` gives scripts and AI agents your chats as JSON: `chats`, `messages`, `search` and `send`. Reading
works with the app closed, straight from the terminal client's data, and never marks anything as read. Sending needs a
login of the agent's own, which you create with `tuimessenger agent login` and can revoke with `agent logout`.
Nothing is sent without `-yes`, and the skill tells agents to show you the draft and wait for your OK first.

```sh
tuimessenger integration install claude opencode codex   # teach your agents how to use it
tuimessenger --skill                                     # or read the skill yourself
```

## The terminal client's files

`tuimessenger tui` keeps its files in the data folder's `tui/` subfolder
(`~/Library/Application Support/tuimessenger/tui` on macOS):

- `config/terminal.yaml`: preferences (changed from the TUI too).
- `config/tuimessenger.yaml`: your colours and pinned preferences; it wins over `terminal.yaml`.
  Keep it in your dotfiles and link it here.
- `client.db`: the client's copy of your chats.

Logs go to `~/Library/Logs/tuimessenger` (`terminal.log`, `client.log`). Notification icons per network come from
`~/.local/opt/tuimessenger/notifiers/<Network>.app` (copies of terminal-notifier), with `tuimessenger.app` for the rest.

## Contributing: keep personal data out

CI runs gitleaks (`.gitleaks.toml`) on every commit for secrets and personal data: phone numbers, chat-network user
IDs, home-folder paths. Use made-up values in tests (`+15550100001`, `@whatsapp_15550100001`, "Mariana"). Locally,
enable the hooks once with `git config core.hooksPath .githooks`; they also check against a private word list in
`.git/info/sensitive-words` (one per line; never committed or pushed) for names and numbers only you know.

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

tuimessenger is licensed under the GNU Affero General Public License v3 (see `LICENSE`). Imported files keep their
original license headers: parts of the client are under the Mozilla Public License 2.0 (`internal/hicli/LICENSE`).
