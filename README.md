# hermitclaw

Persistent Claude Code agent runner. Runs Claude Code in a tmux session with auto-restart, configurable channels, and conversation resume.

Like a hermit crab — lives in a shell, never lets go.

## Features

- **Persistent tmux sessions** — Claude Code runs in a named tmux session you can attach/detach from
- **Auto-restart** — if Claude exits, it restarts automatically after a configurable delay
- **Resume by default** — conversations persist across restarts via `--continue`
- **Channel support** — connect Telegram or other channels
- **Systemd integration** — optional systemd user service for boot-time start
- **Repo-local startup prompt** — lets the working tree define startup behavior without writing agent-specific Claude assets into `~/.claude`

> **Security Warning**
>
> hermitclaw passes `--dangerously-skip-permissions` to Claude Code. This grants the agent **unrestricted access** to your user account — arbitrary file reads/writes, shell commands, and network access with no human approval.
>
> **Recommended mitigations:**
> - Run as a dedicated unprivileged user
> - Use filesystem permissions to limit scope
> - Use systemd sandboxing (`ProtectHome=`, `ReadOnlyPaths=`)
> - Review the tmux session regularly (`hermitclaw attach`)

## Install

### Option A: Nix Flake (NixOS / home-manager)

```nix
# flake.nix inputs:
hermitclaw = {
  url = "git+https://git.jeremyk.net/jeremy/hermitclaw.git";
  inputs.nixpkgs.follows = "nixpkgs";
};

# home-manager config:
imports = [ inputs.hermitclaw.homeManagerModules.default ];
programs.hermitclaw = {
  enable = true;
  claudePackage = pkgs.claude-code; # or your claude-code package
  workingDirectory = "~/dev";
  channels = [ "plugin:telegram@claude-plugins-official" ];
};
```

If you want startup behavior, define it in the working tree and pass a plain-text `initial_prompt` via hermitclaw config instead of relying on `/welcome` from `~/.claude`.

The Nix module automatically:
- Installs the binary with tmux on PATH
- Generates `~/.config/hermitclaw/config.toml`
- Creates a systemd user service
- Starts Claude in the configured working directory without installing agent-specific Claude skills or commands

### Option B: Shell Installer

```bash
git clone https://git.jeremyk.net/jeremy/hermitclaw.git
cd hermitclaw
./install.sh            # Copies binary + config only
./install.sh --systemd  # Also installs systemd user service
```

Requires: `tmux`, `claude` (Claude Code), and optionally `go` (to build from source).

## Usage

```bash
hermitclaw              # Attach if running, start + attach if not
hermitclaw start        # Start session in background
hermitclaw stop         # Stop session
hermitclaw restart      # Stop + start
hermitclaw attach       # Attach to running session
hermitclaw status       # Show whether session is running
hermitclaw logs         # Dump tmux scrollback
hermitclaw fresh        # Stop + start WITHOUT --continue (new conversation)
```

## Configuration

Config file: `~/.config/hermitclaw/config.toml`

```toml
# Working directory for Claude Code sessions
working_directory = "~/dev"

# Optional plain-text initial prompt passed to Claude at startup.
# Keep agent-specific instructions in the working tree, e.g. ~/dev/jk-agent/CLAUDE.md
# initial_prompt = "Read CLAUDE.md in the current working directory and follow it."

# tmux session name
session_name = "hermitclaw"

# Path to claude binary
# claude_binary = "claude"

# Claude Code channels (comma-separated)
# channels = "plugin:telegram@claude-plugins-official"

# Extra arguments passed to claude
# extra_args = ""

# Resume previous conversation on restart
auto_resume = true

# Seconds between restarts
restart_delay = 5
```

CLI flags override config file values. See `hermitclaw --help`.

## License

GPL v3. See [LICENSE](LICENSE).
