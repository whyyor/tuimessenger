---
name: tuimessenger
description: "Read, search and send the user's chats (WhatsApp, Telegram, Slack, Instagram) through tuimessenger. Use when the user asks about their messages, chats, DMs, unread messages, who said what, or asks you to reply to, text or send a file to someone. Requires the tuimessenger CLI."
---

# tuimessenger

tuimessenger puts the user's WhatsApp, Telegram, Slack and Instagram chats in one terminal app. Its `tuimessenger agent`
commands give you the same chats as JSON. These are the user's private conversations: read only what the task needs,
and never send, forward or paste their contents anywhere else unless the user asks.

The installed binary is the authority for syntax. Start with:

```bash
tuimessenger agent help
tuimessenger agent status
```

`status` tells you whether the agent is logged in (needed only to send), whether the backend is running (needed only
to send), and `synced_at`: data is as fresh as the user's last tuimessenger session. Mention it when freshness
matters ("as of 10:42").

## Reading

```bash
tuimessenger agent chats -unread              # chats with unread messages, newest first
tuimessenger agent chats -limit 20            # the 20 most recent chats
tuimessenger agent messages "Maya" -limit 50  # a chat's newest messages, oldest first
tuimessenger agent search "dinner friday"     # every word as a prefix, newest first
tuimessenger agent search -chat "whatsapp:Maya" invoice
```

- `CHAT` is a chat `id` (`!…`), a name or alias, or `network:name` (`whatsapp`, `telegram`, `slack`, `instagram`).
  Exact names win over partial ones.
- If a name matches several chats you get `{"error": …, "matches": [...]}`. Pick by `network`/`direct` when the user's
  words make it clear, otherwise ask, then use the chat's `id`.
- Messages have `sender` ("you" for the user), `from_me`, `time`, `kind` (text, image, video, audio, file, sticker…),
  `text` (for media: the caption or file name), `reply_to` and `reactions`. You can't see media itself.
- Reading never marks anything as read or tells anyone the user looked.

## Sending

Sending writes to a real person on the user's behalf. Always:

1. Resolve the chat and draft the text **without** `-yes`. Nothing is sent; you get back the chat it resolved to:

   ```bash
   tuimessenger agent send "Maya" "running 10 min late, sorry!"
   ```

2. Show the user the exact recipient (`name` and `network`) and the exact text, and wait for an explicit yes. A
   general instruction earlier ("reply to Maya") is not approval of this draft.
3. Only then send the same command with `-yes`, using the chat `id` from step 1 so it can't resolve differently:

   ```bash
   tuimessenger agent send '!roomid:tuimessenger.localhost' "running 10 min late, sorry!" -yes
   ```

4. Report `result.status`: `delivered`, `sent` (the network hasn't confirmed yet) or `failed` (with `result.error`).
   Don't retry a failed send without asking.

Text after `--` is taken literally, for messages starting with `-`.

### Files

Send a file (photo, video, audio or any other file, up to 100 MB) with `-file`; any text becomes its caption. The
same rules apply: draft without `-yes` first. The draft's `file` shows the `name`, `size` and `kind` (`image`,
`video`, `audio` or `file`: how the network will show it); show those to the user with the recipient.

```bash
tuimessenger agent send "Maya" "the build from today" -file ./app-release.apk
tuimessenger agent send '!roomid:tuimessenger.localhost' "the build from today" -file ./app-release.apk -yes
```

Only send files the user pointed you to; never pick files on your own.

## Errors

- **"the agent isn't logged in"**: the user runs `tuimessenger agent login` themselves (it asks for their backend
  password). Never ask for the password or try to log in for them.
- **"the backend isn't running"**: reading still works; to send, the user opens tuimessenger (or starts its service).
- **"no tuimessenger client data here"**: tuimessenger hasn't been set up on this machine (`tuimessenger tui`).

Don't run `tuimessenger agent logout` unless the user asks: it revokes the agent's access.
