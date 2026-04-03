package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds bonsai configuration.
type Config struct {
	StaleThresholdDays int    `toml:"stale_threshold_days"`
	DefaultRemote      string `toml:"default_remote"`
	DefaultBase        string `toml:"default_base"`
}

func Default() *Config {
	return &Config{
		StaleThresholdDays: 14,
		DefaultRemote:      "origin",
		DefaultBase:        "main",
	}
}

func Load() (*Config, error) {
	cfg := Default()
	p := Path()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(p, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bonsai", "config.toml")
}
