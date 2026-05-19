---
name: self-management
description: Use when needing to restart, clear session, check status, or manage the hermitclaw agent service — after config changes, when asked to restart, or to check if the agent is running.
---

# Self Management

Manage the hermitclaw service that runs this Claude instance.

## Commands

| Command | Effect |
|---------|--------|
| `hermitclaw status` | Check if session is running |
| `hermitclaw logs` | Dump tmux scrollback |
| `systemctl --user status hermitclaw` | Systemd service status |

## Restarting

You are running inside the hermitclaw tmux session. Restarting kills your process, so the command must outlive you:

```bash
nohup bash -c 'sleep 1 && hermitclaw restart' &>/dev/null &
```

The `sleep 1` gives time for your final response to render before the session dies. The restart loop will start a new claude session with `--continue`, resuming the previous conversation.

## Fresh Session (Clear Context)

To start a completely new conversation (no `--continue`):

```bash
nohup bash -c 'sleep 1 && hermitclaw fresh' &>/dev/null &
```

This kills the current session and starts fresh — no conversation history carried over.

## Before Restarting or Clearing

1. **Send a Telegram message** telling the user you're restarting/clearing
2. **State why** (config change, explicit request, etc.)
3. Then run the nohup command as your last action

After restart, hermitclaw may pass a plain-text `initial_prompt` from config. Keep that prompt repo-local and avoid depending on globally installed Claude skills or commands.

## When to Restart

- After `dotman deploy --local` that changed Claude Code settings/hooks/skills
- When explicitly asked
- After plugin config changes that require a session restart

## When to Clear (Fresh)

- When explicitly asked to start a new conversation
- When context is heavily polluted and a clean slate is needed
- When switching to a fundamentally different task domain

## Do Not Restart

- For routine tasks or mid-conversation unless asked
- To "fix" a problem — debug first
