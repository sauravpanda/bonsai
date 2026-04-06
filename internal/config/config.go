package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds bonsai configuration.
type Config struct {
	StaleThresholdDays int    `toml:"stale_threshold_days"`
	DefaultRemote      string `toml:"default_remote"`
	DefaultBase        string `toml:"default_base"`
	// TicketPattern is a Go regexp with a capturing group that extracts a
	// ticket ID from a branch name (e.g. "([A-Z]+-\\d+)").
	// If empty, ticket auto-linking is disabled.
	TicketPattern string `toml:"ticket_pattern"`
}

func Default() *Config {
	return &Config{
		StaleThresholdDays: 14,
		DefaultRemote:      "origin",
		DefaultBase:        "main",
		TicketPattern:      `([A-Z]+-\d+)`,
	}
}

// Load returns the effective configuration using this merge order:
//  1. Built-in defaults
//  2. Global config (~/.config/bonsai/config.toml)
//  3. Per-repo config (.bonsai.toml in git repo root) — wins on any key it specifies
func Load() (*Config, error) {
	cfg := Default()

	// 1. Apply global config.
	if p := Path(); fileExists(p) {
		if _, err := toml.DecodeFile(p, cfg); err != nil {
			return cfg, err
		}
	}

	// 2. Apply per-repo config (.bonsai.toml at git root), if any.
	if repoRoot := gitRepoRoot(); repoRoot != "" {
		repoPath := filepath.Join(repoRoot, ".bonsai.toml")
		if fileExists(repoPath) {
			if _, err := toml.DecodeFile(repoPath, cfg); err != nil {
				return cfg, err
			}
		}
	}

	return cfg, nil
}

// Path returns the global config file path.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bonsai", "config.toml")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// gitRepoRoot returns the top-level directory of the current git repo, or "".
func gitRepoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
