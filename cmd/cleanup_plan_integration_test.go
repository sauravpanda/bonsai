package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sauravpanda/bonsai/internal/config"
)

func TestCleanupPlanClassifiesAndAppliesSafely(t *testing.T) {
	repo := setupCleanupTestRepo(t)
	cfg := config.Default()
	cfg.StaleThresholdDays = 14

	oldSafe := addTestWorktree(t, repo, "old-safe", true)
	dirty := addTestWorktree(t, repo, "dirty", true)
	unique := addTestWorktree(t, repo, "unique", true)
	_ = addTestWorktree(t, repo, "fresh", false)

	if err := os.WriteFile(filepath.Join(dirty, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unique, "unique.txt"), []byte("unique\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, unique, "add", "unique.txt")
	runGitWithOldDate(t, unique, "commit", "-m", "unique work")
	touchWorktreePointerOld(t, unique)

	plan, err := buildCleanupPlan(cfg, cleanupPlanOptions{ClaudeOnly: true, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Scanned != 4 || len(plan.Safe) != 1 || len(plan.Protected) != 2 || len(plan.Review) != 0 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if got := plan.Safe[0].Branch; got != "worktree-old-safe" {
		t.Fatalf("safe branch = %q, want worktree-old-safe", got)
	}

	// A change after planning invalidates the whole plan before any removal.
	if err := saveCleanupPlan(plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldSafe, "tracked.txt"), []byte("changed after plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyCleanupPlan(plan.PlanID); err == nil || !strings.Contains(err.Error(), "changed since planning") {
		t.Fatalf("expected changed-plan rejection, got %v", err)
	}
	if _, err := os.Stat(oldSafe); err != nil {
		t.Fatalf("safe worktree was removed after a stale plan: %v", err)
	}

	if err := os.WriteFile(filepath.Join(oldSafe, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = buildCleanupPlan(cfg, cleanupPlanOptions{ClaudeOnly: true, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCleanupPlan(plan); err != nil {
		t.Fatal(err)
	}
	result, err := applyCleanupPlan(plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.BranchesDeleted != 1 || result.ReclaimedBytes <= 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if _, err := os.Stat(oldSafe); !os.IsNotExist(err) {
		t.Fatalf("safe worktree still exists after apply: %v", err)
	}
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/worktree-old-safe").Run(); err == nil {
		t.Fatal("safe local branch still exists after apply")
	}
	if _, err := os.Stat(dirty); err != nil {
		t.Fatalf("protected dirty worktree was removed: %v", err)
	}
	if _, err := os.Stat(unique); err != nil {
		t.Fatalf("protected unpushed worktree was removed: %v", err)
	}
}

func TestGlobalCleanupDiscoversAndAppliesAcrossRepositories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scanRoot := filepath.Join(home, "Github")
	repoA := filepath.Join(scanRoot, "repo-a")
	repoB := filepath.Join(scanRoot, "team", "repo-b")
	initCleanupTestRepo(t, repoA)
	initCleanupTestRepo(t, repoB)
	worktreeA := addTestWorktree(t, repoA, "old-a", true)
	worktreeB := addTestWorktree(t, repoB, "old-b", true)
	chdirForTest(t, repoA)

	cfg := config.Default()
	plan, err := buildCleanupPlan(cfg, cleanupPlanOptions{
		Global:     true,
		ClaudeOnly: true,
		Offline:    true,
		ScanRoots:  []string{scanRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Repositories != 2 || plan.Summary.Scanned != 2 || len(plan.Safe) != 2 {
		t.Fatalf("unexpected global plan: summary=%+v safe=%+v warnings=%v", plan.Summary, plan.Safe, plan.Warnings)
	}
	if err := saveCleanupPlan(plan); err != nil {
		t.Fatal(err)
	}
	result, err := applyCleanupPlan(plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 2 || result.BranchesDeleted != 2 {
		t.Fatalf("unexpected global apply result: %+v", result)
	}
	for _, path := range []string{worktreeA, worktreeB} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("global cleanup did not remove %s: %v", path, err)
		}
	}
}

func setupCleanupTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	initCleanupTestRepo(t, repo)
	chdirForTest(t, repo)
	return repo
}

func initCleanupTestRepo(t *testing.T, repo string) {
	t.Helper()
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Bonsai Test")
	runGit(t, repo, "config", "user.email", "bonsai@example.test")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".claude/worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore", "tracked.txt")
	runGitWithOldDate(t, repo, "commit", "-m", "old base")
}

func chdirForTest(t *testing.T, path string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func addTestWorktree(t *testing.T, repo, name string, stale bool) string {
	t.Helper()
	path := filepath.Join(repo, ".claude", "worktrees", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "worktree", "add", "-b", "worktree-"+name, path, "main")
	if stale {
		touchWorktreePointerOld(t, path)
	}
	return path
}

func touchWorktreePointerOld(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(path, ".git"), old, old); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func runGitWithOldDate(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	date := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
