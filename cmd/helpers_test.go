package cmd

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sauravpanda/bonsai/internal/git"
)

func TestCommandWithPathArg(t *testing.T) {
	cmd, err := commandWithPathArg("code -r", "/tmp/worktree")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cmd.Path, "code"; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "-r" || cmd.Args[2] != "/tmp/worktree" {
		t.Fatalf("unexpected args: %#v", cmd.Args)
	}
}

func TestCommandWithPathArgSupportsQuotedArgs(t *testing.T) {
	cmd, err := commandWithPathArg(`"Visual Studio Code" --reuse-window`, "/tmp/worktree")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cmd.Path, "Visual Studio Code"; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "--reuse-window" || cmd.Args[2] != "/tmp/worktree" {
		t.Fatalf("unexpected args: %#v", cmd.Args)
	}
}

func TestClassifyCleanupFreshWorktreeIsNotACandidate(t *testing.T) {
	state, reasons, candidate := classifyCleanup(&git.Worktree{
		Age:                time.Hour,
		PRStatus:           "none",
		StatusKnown:        true,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, "/repo", 14*24*time.Hour)

	if candidate || state != "" || len(reasons) != 0 {
		t.Fatalf("expected no candidate, got state=%q reasons=%v", state, reasons)
	}
}

func TestClassifyCleanupStaleRemoteBackedWorktreeIsSafe(t *testing.T) {
	state, reasons, candidate := classifyCleanup(&git.Worktree{
		Path:               "/repo/.claude/worktrees/old",
		Branch:             "worktree-old",
		Age:                30 * 24 * time.Hour,
		PRStatus:           "none",
		StatusKnown:        true,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, "/repo", 14*24*time.Hour)

	if !candidate || state != cleanupSafe {
		t.Fatalf("expected safe candidate, got state=%q reasons=%v", state, reasons)
	}
}

func TestClassifyCleanupMergedPRHeadIsSafeAfterSquash(t *testing.T) {
	state, reasons, candidate := classifyCleanup(&git.Worktree{
		Path:            "/repo/.claude/worktrees/merged",
		Branch:          "worktree-merged",
		HEAD:            "abc123",
		PRStatus:        "merged",
		PRHeadOID:       "abc123",
		StatusKnown:     true,
		UnpushedKnown:   true,
		UnpushedCommits: 2,
	}, "/repo", 14*24*time.Hour)

	if !candidate || state != cleanupSafe {
		t.Fatalf("expected merged PR head to be safe, got state=%q reasons=%v", state, reasons)
	}
}

func TestTruncateLeftKeepsTail(t *testing.T) {
	got := truncateLeft("/very/long/path/to/worktrees/feature-x", 20)
	if got != "…worktrees/feature-x" {
		t.Fatalf("truncateLeft = %q", got)
	}
	if short := truncateLeft("wt/feature", 20); short != "wt/feature" {
		t.Fatalf("short string modified: %q", short)
	}
	// Double-width runes must count as 2 columns.
	if wide := truncateLeft("ここ/日本語/パス", 6); wide != "…/パス" {
		t.Fatalf("wide truncation = %q", wide)
	}
}

func TestClassifyCleanupProtectsUnknownPRState(t *testing.T) {
	state, reasons, candidate := classifyCleanup(&git.Worktree{
		Path:               "/repo/.claude/worktrees/unknown",
		Branch:             "worktree-unknown",
		Age:                30 * 24 * time.Hour,
		PRStatus:           "unknown",
		StatusKnown:        true,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, "/repo", 14*24*time.Hour)

	if !candidate || state != cleanupProtected {
		t.Fatalf("expected protected candidate, got state=%q reasons=%v", state, reasons)
	}
}

func TestClassifyCleanupProtectsWorkingChanges(t *testing.T) {
	state, _, candidate := classifyCleanup(&git.Worktree{
		Path:               "/repo/.claude/worktrees/dirty",
		Branch:             "worktree-dirty",
		Age:                30 * 24 * time.Hour,
		StatusKnown:        true,
		DirtyFiles:         1,
		UntrackedFiles:     2,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, "/repo", 14*24*time.Hour)

	if !candidate || state != cleanupProtected {
		t.Fatalf("expected dirty worktree to be protected, got %q", state)
	}
}

func TestClassifyCleanupProtectsCurrentWorktree(t *testing.T) {
	path := "/repo/.claude/worktrees/current"
	state, _, candidate := classifyCleanup(&git.Worktree{
		Path:               path,
		Branch:             "worktree-current",
		Age:                30 * 24 * time.Hour,
		StatusKnown:        true,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, path, 14*24*time.Hour)

	if !candidate || state != cleanupProtected {
		t.Fatalf("expected current worktree to be protected, got %q", state)
	}
}

func TestClassifyCleanupProtectsWorktreeInUse(t *testing.T) {
	state, _, candidate := classifyCleanup(&git.Worktree{
		Path:               "/repo/.claude/worktrees/active",
		Branch:             "worktree-active",
		Age:                30 * 24 * time.Hour,
		IsInUse:            true,
		StatusKnown:        true,
		UnpushedKnown:      true,
		RemoteBranchExists: true,
	}, "/repo", 14*24*time.Hour)

	if !candidate || state != cleanupProtected {
		t.Fatalf("expected in-use worktree to be protected, got %q", state)
	}
}

func TestInspectSnapshotPrefersOriginalPathMetadata(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "snapshot.tar.gz")
	if err := writeTestSnapshot(archive, "/tmp/original/path", "branch-dir"); err != nil {
		t.Fatal(err)
	}

	info, err := inspectSnapshot(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.OriginalPath, "/tmp/original/path"; got != want {
		t.Fatalf("OriginalPath = %q, want %q", got, want)
	}
	if got, want := info.RootDir, "branch-dir"; got != want {
		t.Fatalf("RootDir = %q, want %q", got, want)
	}
}

func TestExtractTarGzAllowsRelativeDestination(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "snapshot.tar.gz")
	if err := writeTestSnapshot(archive, "/tmp/original/path", "branch-dir"); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	restoreDir := filepath.Join(root, "restore")
	if err := os.MkdirAll(restoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(restoreDir); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archive, "."); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(restoreDir, "branch-dir", "file.txt")); err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
}

func writeTestSnapshot(path, originalPath, rootDir string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	meta := []byte(originalPath + "\n")
	if err := tw.WriteHeader(&tar.Header{
		Name: snapshotMetaName,
		Mode: 0o600,
		Size: int64(len(meta)),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(meta); err != nil {
		return err
	}

	content := []byte("hello")
	if err := tw.WriteHeader(&tar.Header{
		Name: rootDir + "/file.txt",
		Mode: 0o644,
		Size: int64(len(content)),
	}); err != nil {
		return err
	}
	_, err = tw.Write(content)
	return err
}
