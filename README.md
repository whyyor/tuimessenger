# tuimessenger

Your chats from WhatsApp, Telegram, Slack and Instagram in one terminal app, with the bridges running on your own
machine.

## Building

Always build with the `goolm` and `sqlite_fts5` tags (the Makefile does this):

```sh
make build   # or: go build -tags goolm,sqlite_fts5 ./cmd/tuimessenger
make test
make vet
```

Without `goolm`, the build looks for the libolm C library (`olm/olm.h not found`).

## Credits

tuimessenger is built on the work of others:

- **The terminal client** (`internal/tui`, `internal/rpc`, `internal/hicli`, `internal/rainbow`) comes from
  [gomuks](https://github.com/gomuks/gomuks) by Tulir Asokan and contributors, via the fork
  [whyyor/gomuks](https://github.com/whyyor/gomuks), and was imported with its git history.
- **The network bridges** are the [mautrix](https://github.com/mautrix) bridges (WhatsApp, Telegram, Meta, Slack)
  and the [mautrix-go](https://github.com/mautrix/go) bridge library.

## License

tuimessenger is licensed under the GNU Affero General Public License v3 (see `LICENSE`). Imported files keep their
original license headers: parts of the client are under the Mozilla Public License 2.0 (`internal/hicli/LICENSE`).
