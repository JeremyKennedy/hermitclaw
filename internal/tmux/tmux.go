package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnsureTmpdir sets TMUX_TMPDIR if not already set, so that shell and systemd
// sessions share the same tmux socket.
func EnsureTmpdir() {
	if os.Getenv("TMUX_TMPDIR") != "" {
		return
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		os.Setenv("TMUX_TMPDIR", dir)
	}
}

// SessionExists checks if a tmux session with the given name exists.
func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// NewSession creates a new detached tmux session running the given command.
func NewSession(name string, command string) error {
	cmd := exec.Command("tmux", "new-session",
		"-d", "-s", name,
		"-x", "200", "-y", "50",
		command,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Set history limit
	setOpt := exec.Command("tmux", "set-option", "-t", name, "history-limit", "50000")
	_ = setOpt.Run() // best-effort

	return nil
}

// KillSession kills the named tmux session.
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Attach attaches to the named tmux session.
func Attach(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CapturePaneOutput returns the scrollback buffer of the named session.
func CapturePaneOutput(name string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", name, "-p", "-S", "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// SessionCreatedTime returns the session creation timestamp.
func SessionCreatedTime(name string) (string, error) {
	cmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{session_created}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
