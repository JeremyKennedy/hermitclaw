# hermitclaw

Persistent Claude Code agent runner. Runs Claude Code in a tmux session with auto-restart, configurable channels, and resume-by-default.

**Visibility**: Public.

## Architecture

Single bash script (`hermitclaw`) is the entire runtime. The Nix module and shell installer are delivery mechanisms.

```
hermitclaw              # The script — all runtime logic
flake.nix               # Nix flake: package + home-manager module + checks
install.sh              # Shell installer for non-Nix users
config.example.toml     # Documented example config
skills/welcome/         # Telegram announcement skill
```

## Key Design Decisions

- **Script is source of truth**: Nix module generates config.toml and calls the script. No duplicated logic.
- **`_run` self-invocation**: The script calls itself inside tmux (`hermitclaw _run`) — no separate wrapper script needed.
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
