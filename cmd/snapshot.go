package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Archive a worktree as a safety net before deletion",
	Long: `bonsai snapshot creates compressed tar archives of worktrees so you can
restore them if needed after deletion.

Subcommands:
  create <n>           snapshot worktree #n to ~/.local/share/bonsai/snapshots/
  list                 list all saved snapshots
  restore <snapshot>   restore a snapshot to its original path`,
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create <n>",
	Short: "Create a snapshot of worktree #n",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotCreate,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snapshots",
	RunE:  runSnapshotList,
}

var snapshotRestoreCmd = &cobra.Command{
	Use:   "restore <snapshot-file>",
	Short: "Restore a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotRestore,
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
}

func snapshotDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "bonsai", "snapshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

var (
	snapBold = lipgloss.NewStyle().Bold(true)
	snapDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	snapOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

func runSnapshotCreate(cmd *cobra.Command, args []string) error {
	worktrees, err := git.List()
	if err != nil {
		return err
	}

	added := AddedWorktrees(worktrees)
	if len(added) == 0 {
		return fmt.Errorf("no added worktrees")
	}

	n := 0
	if _, err := fmt.Sscan(args[0], &n); err != nil || n < 1 || n > len(added) {
		return fmt.Errorf("invalid worktree number %q (valid range: 1–%d)", args[0], len(added))
	}

	wt := added[n-1]
	dir, err := snapshotDir()
	if err != nil {
		return err
	}

	branch := strings.ReplaceAll(wt.Branch, "/", "-")
	ts := time.Now().UTC().Format("20060102-150405")
	archiveName := fmt.Sprintf("%s-%s.tar.gz", branch, ts)
	archivePath := filepath.Join(dir, archiveName)

	fmt.Printf("  creating snapshot of %s … ", snapBold.Render(wt.Branch))

	if err := createTarGz(wt.Path, archivePath); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("create archive: %w", err)
	}

	info, _ := os.Stat(archivePath)
	size := ""
	if info != nil {
		size = " (" + formatBytes(info.Size()) + ")"
	}
	fmt.Printf("%s\n", snapOK.Render("done"))
	fmt.Printf("  saved to: %s%s\n", archivePath, size)
	return nil
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	dir, err := snapshotDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read snapshot dir: %w", err)
	}

	var snapshots []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tar.gz") {
			snapshots = append(snapshots, e)
		}
	}

	if len(snapshots) == 0 {
		fmt.Println(snapDim.Render("  no snapshots found"))
		return nil
	}

	fmt.Printf("  %d snapshot(s) in %s:\n\n", len(snapshots), dir)
	for _, e := range snapshots {
		info, _ := e.Info()
		size := ""
		if info != nil {
			size = " " + snapDim.Render("("+formatBytes(info.Size())+")")
		}
		fmt.Printf("  %s%s\n", e.Name(), size)
	}
	return nil
}

func runSnapshotRestore(cmd *cobra.Command, args []string) error {
	target := args[0]
	if !filepath.IsAbs(target) {
		dir, err := snapshotDir()
		if err != nil {
			return err
		}
		target = filepath.Join(dir, target)
	}

	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("snapshot not found: %s", target)
	}

	// Determine destination from archive contents.
	dest, err := tarGzRootDir(target)
	if err != nil {
		return fmt.Errorf("inspect archive: %w", err)
	}

	fmt.Printf("  restoring %s to %s … ", filepath.Base(target), dest)
	if err := extractTarGz(target, filepath.Dir(dest)); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("extract archive: %w", err)
	}
	fmt.Println(snapOK.Render("done"))
	return nil
}

func createTarGz(srcDir, destFile string) error {
	f, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	base := filepath.Base(srcDir)
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		name := filepath.Join(base, rel)

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		hdr.Name = name

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !d.IsDir() {
			ff, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer ff.Close()
			_, err = io.Copy(tw, ff)
			return err
		}
		return nil
	})
}

func tarGzRootDir(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		return "", err
	}
	// Return the top-level directory in the archive.
	parts := strings.SplitN(hdr.Name, "/", 2)
	return parts[0], nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize path to prevent directory traversal.
		target := filepath.Join(destDir, filepath.Clean("/"+hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue // skip suspicious paths
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
