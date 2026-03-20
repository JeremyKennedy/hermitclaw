# HermitClaw

Persistent Claude Code agent runner. Runs Claude Code in a tmux session with auto-restart, configurable channels, and resume-by-default.

**Visibility**: Public.

## Architecture

Go binary (`hermitclaw`) is the runtime. The Nix module and shell installer are delivery mechanisms.

```
main.go                 # Entry point — CLI, tmux session management, restart loop
internal/config/        # Config loading (TOML) with CLI flag overrides
internal/tmux/          # tmux session operations
flake.nix               # Nix flake: package + home-manager module + checks
config.example.toml     # Documented example config
skills/welcome/         # Telegram announcement skill (runs on every start)
skills/self-management/ # Agent self-restart and status commands
```

## Key Design Decisions

- **Binary is source of truth**: Nix module generates config.toml and calls the binary. No duplicated logic.
- **`_run` self-invocation**: The binary calls itself inside tmux (`hermitclaw _run`) — no separate wrapper script needed.
- **Config cascade**: CLI flags > config.toml > built-in defaults. No env var layer for simplicity.
- **TOML parsing**: Flat key=value only, grep/sed, never eval'd.
- **`--dangerously-skip-permissions` always**: Not configurable. This tool is for autonomous agent operation.
- **`--continue` by default**: Resume previous conversation on restart. `hermitclaw fresh` for new session.

## Commands

- `just check` — shellcheck + structure validation
- `just build` — alias for check
- `just dev` — development info
- `nix flake check` — Nix-wrapped validation

## Commits

Use conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`.
