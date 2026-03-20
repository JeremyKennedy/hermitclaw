package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"git.jeremyk.net/jeremy/hermitclaw/internal/config"
	"git.jeremyk.net/jeremy/hermitclaw/internal/tmux"
)

var version = "dev"

var (
	flagWorkDir      string
	flagSession      string
	flagChannels     string
	flagExtraArgs    string
	flagNoResume     bool
	flagClaudeBin    string
	flagRestartDelay int
	flagConfigFile   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:           "hermitclaw",
		Short:         "Persistent Claude Code agent runner",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if tmux.SessionExists(cfg.SessionName) {
				return tmux.Attach(cfg.SessionName)
			}
			if err := startSession(cfg); err != nil {
				return err
			}
			return tmux.Attach(cfg.SessionName)
		},
	}

	addFlags(rootCmd)

	rootCmd.AddCommand(
		startCmd(),
		stopCmd(),
		restartCmd(),
		attachCmd(),
		statusCmd(),
		logsCmd(),
		freshCmd(),
		runCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addFlags(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&flagWorkDir, "work-dir", "", "Working directory")
	pf.StringVar(&flagSession, "session", "", "tmux session name")
	pf.StringVar(&flagChannels, "channels", "", "Comma-separated Claude channels")
	pf.StringVar(&flagExtraArgs, "extra-args", "", "Extra arguments for claude")
	pf.BoolVar(&flagNoResume, "no-resume", false, "Don't pass --continue on restart")
	pf.StringVar(&flagClaudeBin, "claude-bin", "", "Path to claude binary")
	pf.IntVar(&flagRestartDelay, "restart-delay", 0, "Seconds between restarts")
	pf.StringVar(&flagConfigFile, "config", "", "Config file path")
}

func loadConfig() *config.Config {
	cfgPath := flagConfigFile
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config from %s: %v\n", cfgPath, err)
		cfg = config.DefaultConfig()
	}

	// CLI flags override config
	if flagWorkDir != "" {
		cfg.WorkingDirectory = flagWorkDir
	}
	if flagSession != "" {
		cfg.SessionName = flagSession
	}
	if flagChannels != "" {
		cfg.Channels = flagChannels
	}
	if flagExtraArgs != "" {
		cfg.ExtraArgs = flagExtraArgs
	}
	if flagNoResume {
		cfg.AutoResume = false
	}
	if flagClaudeBin != "" {
		cfg.ClaudeBinary = flagClaudeBin
	}
	if flagRestartDelay > 0 {
		cfg.RestartDelay = flagRestartDelay
	}

	return cfg
}

func startSession(cfg *config.Config) error {
	workDir := config.ExpandHome(cfg.WorkingDirectory)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", workDir)
	}

	// Build the command that runs inside tmux
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find self: %w", err)
	}

	// Pass config file path to _run so it reads the same config
	cfgPath := flagConfigFile
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}

	runCmd := fmt.Sprintf("%s --config %s _run", selfPath, cfgPath)

	tmux.EnsureTmpdir()
	if err := tmux.NewSession(cfg.SessionName, runCmd); err != nil {
		return err
	}

	fmt.Printf("Session '%s' started.\n", cfg.SessionName)
	return nil
}

// --- Subcommands ---

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start session in background",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if tmux.SessionExists(cfg.SessionName) {
				return fmt.Errorf("session '%s' is already running", cfg.SessionName)
			}
			return startSession(cfg)
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if !tmux.SessionExists(cfg.SessionName) {
				fmt.Printf("Session '%s' is not running.\n", cfg.SessionName)
				return nil
			}
			if err := tmux.KillSession(cfg.SessionName); err != nil {
				return err
			}
			fmt.Printf("Session '%s' stopped.\n", cfg.SessionName)
			return nil
		},
	}
}

func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Stop + start session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if tmux.SessionExists(cfg.SessionName) {
				if err := tmux.KillSession(cfg.SessionName); err != nil {
					return err
				}
				fmt.Printf("Session '%s' stopped.\n", cfg.SessionName)
			}
			return startSession(cfg)
		},
	}
}

func attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach",
		Short: "Attach to running session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if !tmux.SessionExists(cfg.SessionName) {
				return fmt.Errorf("session '%s' is not running — use 'hermitclaw start' or run with no subcommand", cfg.SessionName)
			}
			return tmux.Attach(cfg.SessionName)
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether session is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if !tmux.SessionExists(cfg.SessionName) {
				fmt.Printf("Session '%s' is not running.\n", cfg.SessionName)
				return nil
			}
			fmt.Printf("Session '%s' is running.\n", cfg.SessionName)
			if ts, err := tmux.SessionCreatedTime(cfg.SessionName); err == nil {
				if epoch, err := strconv.ParseInt(ts, 10, 64); err == nil {
					started := time.Unix(epoch, 0)
					fmt.Printf("  Started: %s\n", started.Format("2006-01-02 15:04:05"))
					fmt.Printf("  Uptime:  %s\n", time.Since(started).Truncate(time.Second))
				}
			}
			return nil
		},
	}
}

func logsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Dump tmux scrollback",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()
			if !tmux.SessionExists(cfg.SessionName) {
				return fmt.Errorf("session '%s' is not running", cfg.SessionName)
			}
			output, err := tmux.CapturePaneOutput(cfg.SessionName)
			if err != nil {
				return err
			}
			fmt.Println(output)
			return nil
		},
	}
}

func freshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fresh",
		Short: "Stop + start WITHOUT --continue",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			tmux.EnsureTmpdir()

			// Stop if running
			if tmux.SessionExists(cfg.SessionName) {
				if err := tmux.KillSession(cfg.SessionName); err != nil {
					return err
				}
				fmt.Printf("Session '%s' stopped.\n", cfg.SessionName)
			}

			// Write fresh marker
			markerDir := freshMarkerDir()
			if err := os.MkdirAll(markerDir, 0o700); err != nil {
				return fmt.Errorf("failed to create marker dir: %w", err)
			}
			if err := os.WriteFile(filepath.Join(markerDir, "fresh"), nil, 0o644); err != nil {
				return fmt.Errorf("failed to write fresh marker: %w", err)
			}

			return startSession(cfg)
		},
	}
}

// runCmd is the internal _run command executed inside tmux.
func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_run",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			workDir := config.ExpandHome(cfg.WorkingDirectory)
			if err := os.Chdir(workDir); err != nil {
				return fmt.Errorf("failed to cd to %s: %w", workDir, err)
			}

			// Check fresh marker
			isFresh := false
			markerPath := filepath.Join(freshMarkerDir(), "fresh")
			if _, err := os.Stat(markerPath); err == nil {
				isFresh = true
				os.Remove(markerPath)
			}

			for {
				claudeArgs := []string{"--dangerously-skip-permissions"}

				if cfg.AutoResume && !isFresh {
					claudeArgs = append(claudeArgs, "--continue")
				}

				// Channels
				if cfg.Channels != "" {
					for _, ch := range strings.Split(cfg.Channels, ",") {
						ch = strings.TrimSpace(ch)
						if ch != "" {
							claudeArgs = append(claudeArgs, "--channels", ch)
						}
					}
				}

				// Extra args
				if cfg.ExtraArgs != "" {
					claudeArgs = append(claudeArgs, strings.Fields(cfg.ExtraArgs)...)
				}

				fmt.Println("Starting claude...")
				c := exec.Command(cfg.ClaudeBinary, claudeArgs...)
				c.Stdin = os.Stdin
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				err := c.Run()

				exitCode := 0
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					}
				}

				fmt.Printf("Claude exited (%d), restarting in %ds...\n", exitCode, cfg.RestartDelay)
				time.Sleep(time.Duration(cfg.RestartDelay) * time.Second)

				// After first iteration, always resume
				isFresh = false
			}
		},
	}
}

func freshMarkerDir() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hermitclaw")
}
