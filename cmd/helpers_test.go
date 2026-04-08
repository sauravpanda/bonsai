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

func TestCandidateReasonsDoesNotTreatUnknownPRAsNoPR(t *testing.T) {
	reasons := candidateReasons(&git.Worktree{
		Age:         time.Hour,
		PRStatus:    "unknown",
		HasUnpushed: false,
	}, float64(14*24*time.Hour))

	for _, reason := range reasons {
		if reason == "no unpushed commits" {
			t.Fatalf("unexpected reason set: %v", reasons)
		}
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
