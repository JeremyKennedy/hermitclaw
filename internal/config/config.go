package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	WorkingDirectory string `toml:"working_directory"`
	InitialPrompt    string `toml:"initial_prompt"`
	SessionName      string `toml:"session_name"`
	Channels         string `toml:"channels"`
	ExtraArgs        string `toml:"extra_args"`
	AutoResume       bool   `toml:"auto_resume"`
	ClaudeBinary     string `toml:"claude_binary"`
	RestartDelay     int    `toml:"restart_delay"`
}

func DefaultConfig() *Config {
	return &Config{
		WorkingDirectory: os.Getenv("HOME"),
		InitialPrompt:    "",
		SessionName:      "hermitclaw",
		Channels:         "",
		ExtraArgs:        "",
		AutoResume:       true,
		ClaudeBinary:     "claude",
		RestartDelay:     5,
	}
}

func DefaultConfigPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "hermitclaw", "config.toml")
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ExpandHome expands ~ at the start of a path.
func ExpandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	if path == "~" {
		return os.Getenv("HOME")
	}
	return path
}
