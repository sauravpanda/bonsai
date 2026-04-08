package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open <n>",
	Short: "Open a worktree in the editor by number",
	Long: `Open the nth worktree (as numbered by bonsai list) in the editor.

Editor resolution order:
  1. --editor flag
  2. $BONSAI_EDITOR environment variable
  3. $EDITOR environment variable
  4. Common defaults: code (VS Code), cursor, nano

Examples:
  bonsai open 2
  bonsai open 3 --editor cursor`,
	Args: cobra.ExactArgs(1),
	RunE: runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)
	openCmd.Flags().String("editor", "", "editor command to use (overrides $EDITOR)")
}

func runOpen(cmd *cobra.Command, args []string) error {
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || n < 1 {
		return fmt.Errorf("invalid worktree number %q", args[0])
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	added := AddedWorktrees(worktrees)
	if len(added) == 0 {
		return fmt.Errorf("no added worktrees")
	}
	if n > len(added) {
		return fmt.Errorf("worktree number %d out of range (1–%d)", n, len(added))
	}

	wt := added[n-1]

	// Enrich to show branch name in output.
	git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)

	editor, _ := cmd.Flags().GetString("editor")
	if editor == "" {
		editor = os.Getenv("BONSAI_EDITOR")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		// Try common editors in order.
		for _, e := range []string{"code", "cursor", "nano"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found; set $EDITOR or use --editor")
	}

	fmt.Printf("  opening %s in %s…\n", wt.Branch, editor)
	openCmd, err := commandWithPathArg(editor, wt.Path)
	if err != nil {
		return err
	}
	openCmd.Stdout = os.Stdout
	openCmd.Stderr = os.Stderr
	openCmd.Stdin = os.Stdin
	if err := openCmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", editor, err)
	}
	return nil
}
