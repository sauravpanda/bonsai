package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage bonsai configuration",
	Long:  `View and initialize the bonsai configuration file (~/.config/bonsai/config.toml).`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write default config to the standard path",
	RunE:  runConfigInit,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(config.Path())
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current effective configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		fmt.Printf("stale_threshold_days = %d\n", cfg.StaleThresholdDays)
		fmt.Printf("default_remote       = %q\n", cfg.DefaultRemote)
		fmt.Printf("default_base         = %q\n", cfg.DefaultBase)
		fmt.Printf("ticket_pattern       = %q\n", cfg.TicketPattern)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)

	configInitCmd.Flags().Bool("force", false, "overwrite existing config file")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	p := config.Path()

	if _, err := os.Stat(p); err == nil && !force {
		return fmt.Errorf("config file already exists at %s (use --force to overwrite)", p)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(config.Default()); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("wrote default config to %s\n", p)
	return nil
}
