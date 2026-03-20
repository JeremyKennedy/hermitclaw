---
name: welcome
description: Announce that Claude Code is online via Telegram. Runs automatically on session start — do not wait for user input.
---

# Welcome

Send a Telegram message announcing you're online.

## Steps

1. Read `~/.claude/channels/telegram/access.json` and extract the first entry from `allowFrom` — this is the chat_id.
2. If the file doesn't exist or has no entries, skip silently (Telegram not configured).
3. Send a message via the `mcp__plugin_telegram_telegram__reply` tool:
   - `chat_id`: the value from step 1
   - `text`: "Claude Code is up and running in {cwd}." where {cwd} is your current working directory (use `~` prefix instead of the full home path)

Do not ask questions. Do not wait for confirmation. Just send the message.
