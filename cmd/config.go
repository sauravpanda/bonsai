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

var configCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate global and repository configuration",
	Long: `Validate every active bonsai configuration source, report syntax,
type, range, and unknown-key diagnostics, then show the merged effective
configuration. Missing config files are valid and use built-in defaults.

Pass --json for a stable machine-readable report suitable for CI and agents.`,
	RunE: runConfigCheck,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configCheckCmd)

	configInitCmd.Flags().Bool("force", false, "overwrite existing config file")
	configCheckCmd.Flags().Bool("json", false, "emit the validation report as JSON to stdout")
}

func runConfigCheck(cmd *cobra.Command, args []string) error {
	result := config.Check()
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		if err := writeJSON(result); err != nil {
			return err
		}
	} else {
		printConfigCheck(result)
	}
	if !result.Valid {
		return fmt.Errorf("configuration is invalid")
	}
	return nil
}

func printConfigCheck(result config.CheckResult) {
	for _, source := range result.Sources {
		switch {
		case !source.Exists:
			fmt.Printf("  %s %-10s %s %s\n", dimStyle.Render("–"), source.Scope, source.Path, dimStyle.Render("(not found)"))
		case source.Valid:
			fmt.Printf("  %s %-10s %s\n", okStyle.Render("✓"), source.Scope, source.Path)
		default:
			fmt.Printf("  %s %-10s %s\n", errStyle.Render("✗"), source.Scope, source.Path)
		}
	}
	for _, warning := range result.Warnings {
		fmt.Printf("  %s %s: %s\n", warnStyle.Render("warning:"), diagnosticLocation(warning), warning.Message)
	}
	for _, validationErr := range result.Errors {
		fmt.Printf("  %s %s: %s\n", errStyle.Render("error:"), diagnosticLocation(validationErr), validationErr.Message)
	}
	if !result.Valid {
		return
	}

	fmt.Println("\n  " + boldStyle.Render("Effective configuration:"))
	fmt.Printf("  stale_threshold_days = %d\n", result.Effective.StaleThresholdDays)
	fmt.Printf("  default_remote       = %q\n", result.Effective.DefaultRemote)
	fmt.Printf("  default_base         = %q\n", result.Effective.DefaultBase)
	fmt.Printf("  ticket_pattern       = %q\n", result.Effective.TicketPattern)
	if len(result.Warnings) == 0 {
		fmt.Println("\n  " + okStyle.Render("✓ configuration is valid"))
	} else {
		fmt.Printf("\n  %s configuration is valid with %d warning(s)\n", warnStyle.Render("!"), len(result.Warnings))
	}
}

func diagnosticLocation(diagnostic config.Diagnostic) string {
	location := diagnostic.Path
	if diagnostic.Line > 0 {
		location += fmt.Sprintf(":%d", diagnostic.Line)
	}
	if diagnostic.Key != "" {
		location += " [" + diagnostic.Key + "]"
	}
	return location
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
