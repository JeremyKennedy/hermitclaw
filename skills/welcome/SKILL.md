---
name: welcome
description: Use on session start to announce via Telegram that Claude Code is up and running. Triggered automatically by SessionStart hook.
---

# Welcome

Send a brief Telegram message announcing that Claude Code is online.

Read `~/.claude/channels/telegram/access.json` to get the first entry in `allowFrom` — use that as the `chat_id`.

Use `mcp__plugin_telegram_telegram__reply` to send a message like:

> Claude Code is up and running in `<cwd>`.

where `<cwd>` is the current working directory. Keep it short — one line.

If the Telegram channel is not connected, the access file is missing, or `allowFrom` is empty, skip silently.
