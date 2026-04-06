package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <branch>",
	Short: "Create a new worktree + branch",
	Long: `Create a new git worktree with a new branch.

The worktree directory is placed alongside the main repo root, named after
the branch (slashes replaced with dashes). The new branch is created from
the configured base branch (default: main).

Examples:
  bonsai new feat/my-feature
  bonsai new fix/login-bug --base develop
  bonsai new spike/try-idea --open`,
	Args: cobra.ExactArgs(1),
	RunE: runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().String("base", "", "base branch to create from (default: config default_base)")
	newCmd.Flags().Bool("open", false, "open the worktree in $EDITOR after creation")
}

func runNew(cmd *cobra.Command, args []string) error {
	branch := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	base, _ := cmd.Flags().GetString("base")
	if base == "" {
		base = cfg.DefaultBase
	}
	open, _ := cmd.Flags().GetBool("open")

	// Derive worktree directory: place it next to the main repo root,
	// named after the branch with slashes replaced by dashes.
	root, err := git.MainRoot()
	if err != nil {
		return fmt.Errorf("find main root: %w", err)
	}

	dirName := strings.ReplaceAll(branch, "/", "-")
	wtPath := filepath.Join(filepath.Dir(root), dirName)

	fmt.Printf("  creating worktree %s from %s …\n", branch, base)

	// Fetch the base branch first so we're up to date.
	fetchCmd := exec.Command("git", "fetch", cfg.DefaultRemote, base)
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	_ = fetchCmd.Run() // non-fatal if offline

	// git worktree add <path> -b <branch> <base>
	addArgs := []string{"worktree", "add", wtPath, "-b", branch, cfg.DefaultRemote + "/" + base}
	addCmd := exec.Command("git", addArgs...)
	addCmd.Stdout = os.Stdout
	addCmd.Stderr = os.Stderr
	if err := addCmd.Run(); err != nil {
		// Fallback: try without remote prefix (useful when no remote tracking branch exists)
		addArgs2 := []string{"worktree", "add", wtPath, "-b", branch, base}
		addCmd2 := exec.Command("git", addArgs2...)
		addCmd2.Stdout = os.Stdout
		addCmd2.Stderr = os.Stderr
		if err2 := addCmd2.Run(); err2 != nil {
			return fmt.Errorf("git worktree add: %w", err)
		}
	}

	fmt.Printf("  worktree created at: %s\n", wtPath)

	if open {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			fmt.Fprintln(os.Stderr, "  note: $EDITOR not set, skipping --open")
			return nil
		}
		openCmd := exec.Command(editor, wtPath)
		openCmd.Stdout = os.Stdout
		openCmd.Stderr = os.Stderr
		openCmd.Stdin = os.Stdin
		if err := openCmd.Run(); err != nil {
			return fmt.Errorf("open editor: %w", err)
		}
	}

	return nil
}
