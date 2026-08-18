package git

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Worktree represents a git worktree with enriched metadata.
type Worktree struct {
	Path               string
	RepositoryRoot     string
	Branch             string
	HEAD               string
	IsMain             bool
	IsDetached         bool
	IsLocked           bool
	LockReason         string
	IsInUse            bool
	InUseKnown         bool
	IsPrunable         bool
	PrunableReason     string
	Age                time.Duration
	CreatedAt          time.Time
	ActivityAt         time.Time
	LastCommit         string
	AheadBase          int
	BehindBase         int
	PRStatus           string // "merged", "open", "closed", "none", "unknown"
	PRURL              string
	PRNumber           int
	PRHeadOID          string
	HasUnpushed        bool
	UnpushedCommits    int
	UnpushedKnown      bool
	RemoteBranchExists bool
	MergedIntoBase     bool
	BaseStatusKnown    bool
	DirtyFiles         int
	StagedFiles        int
	UntrackedFiles     int
	StatusKnown        bool
}

// HasWorkingChanges reports whether removing the worktree would discard
// staged, modified, or untracked files.
func (wt *Worktree) HasWorkingChanges() bool {
	return wt.DirtyFiles > 0 || wt.StagedFiles > 0 || wt.UntrackedFiles > 0
}

// List returns all git worktrees parsed from `git worktree list --porcelain`.
func List() ([]*Worktree, error) {
	return ListAt(".")
}

// ListAt returns all worktrees registered to the repository containing path.
func ListAt(path string) ([]*Worktree, error) {
	out, err := run("git", "-C", path, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list at %s: %w", path, err)
	}
	worktrees := parsePorcelain(out)
	if len(worktrees) > 0 {
		repositoryRoot := worktrees[0].Path
		for _, wt := range worktrees {
			wt.RepositoryRoot = repositoryRoot
		}
	}
	return worktrees, nil
}

func parsePorcelain(out string) []*Worktree {
	var worktrees []*Worktree
	var cur *Worktree
	first := true

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur = &Worktree{
				Path:   strings.TrimPrefix(line, "worktree "),
				IsMain: first,
			}
			first = false
			worktrees = append(worktrees, cur)
		case cur != nil && strings.HasPrefix(line, "HEAD "):
			cur.HEAD = strings.TrimPrefix(line, "HEAD ")
		case cur != nil && strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case cur != nil && line == "detached":
			cur.IsDetached = true
			cur.Branch = "(detached)"
		case cur != nil && (line == "locked" || strings.HasPrefix(line, "locked ")):
			cur.IsLocked = true
			cur.LockReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
		case cur != nil && (line == "prunable" || strings.HasPrefix(line, "prunable ")):
			cur.IsPrunable = true
			cur.PrunableReason = strings.TrimSpace(strings.TrimPrefix(line, "prunable"))
		}
	}
	return worktrees
}

// Enrich fetches activity, working tree state, ahead/behind, and recovery
// information used by both display and safe cleanup commands.
func Enrich(wt *Worktree, base, remote string) {
	wt.CreatedAt = worktreeCreatedAt(wt.Path)
	latestActivity := wt.CreatedAt

	out, err := runIn(wt.Path, "git", "log", "-1", "--format=%ct|%s")
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
		if len(parts) == 2 {
			if unix, parseErr := strconv.ParseInt(parts[0], 10, 64); parseErr == nil {
				commitAt := time.Unix(unix, 0)
				if commitAt.After(latestActivity) {
					latestActivity = commitAt
				}
			}
			wt.LastCommit = parts[1]
		}
	}
	wt.ActivityAt = latestActivity
	if !latestActivity.IsZero() && time.Now().After(latestActivity) {
		wt.Age = time.Since(latestActivity)
	}

	wt.DirtyFiles, wt.StagedFiles, wt.UntrackedFiles, wt.StatusKnown = WorkingTreeStatus(wt.Path)

	if wt.IsDetached || wt.Branch == "" || wt.Branch == "(detached)" {
		return
	}

	wt.AheadBase, wt.BehindBase = aheadBehind(wt.Path, wt.Branch, base, remote)
	wt.UnpushedCommits, wt.RemoteBranchExists, wt.UnpushedKnown = unpushedStatus(wt.Path, wt.Branch, base, remote)
	wt.HasUnpushed = wt.UnpushedKnown && wt.UnpushedCommits > 0
	wt.MergedIntoBase, wt.BaseStatusKnown = mergedIntoBase(wt.Path, wt.Branch, base, remote)
}

func worktreeCreatedAt(path string) time.Time {
	// Added worktrees contain a .git pointer file created with the worktree.
	// Its timestamp avoids treating a new worktree based on an old commit as
	// stale. Main worktrees have a .git directory and are never cleanup targets.
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// WorkingTreeStatus counts modified, staged, and untracked entries. known is
// false when git status cannot safely inspect the worktree.
func WorkingTreeStatus(path string) (dirty, staged, untracked int, known bool) {
	out, err := runIn(path, "git", "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return 0, 0, 0, false
	}
	dirty, staged, untracked = parseWorkingTreeStatus(out)
	return dirty, staged, untracked, true
}

func parseWorkingTreeStatus(out string) (dirty, staged, untracked int) {
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			untracked++
			continue
		}
		if x != ' ' && x != '?' {
			staged++
		}
		if y != ' ' && y != '?' {
			dirty++
		}
	}
	return dirty, staged, untracked
}

func aheadBehind(path, branch, base, remote string) (int, int) {
	baseRef := remote + "/" + base
	out, err := runIn(path, "git", "rev-list", "--left-right", "--count", branch+"..."+baseRef)
	if err != nil {
		out, err = runIn(path, "git", "rev-list", "--left-right", "--count", branch+"..."+base)
		if err != nil {
			return 0, 0
		}
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) != 2 {
		return 0, 0
	}
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[1])
	return a, b
}

func unpushedStatus(path, branch, base, remote string) (count int, remoteExists, known bool) {
	remoteRef := remote + "/" + branch
	remoteExists = refExists(path, "refs/remotes/"+remoteRef)
	if remoteExists {
		out, err := runIn(path, "git", "rev-list", "--count", remoteRef+".."+branch)
		if err != nil {
			return 0, true, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(out))
		return n, true, err == nil
	}

	// No remote branch exists yet. Count only commits unique to this branch
	// relative to the configured base branch instead of the full history.
	baseRef := remote + "/" + base
	out, err := runIn(path, "git", "rev-list", "--count", baseRef+".."+branch)
	if err != nil {
		out, err = runIn(path, "git", "rev-list", "--count", base+".."+branch)
		if err != nil {
			return 0, false, false
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	return n, false, err == nil
}

func mergedIntoBase(path, branch, base, remote string) (bool, bool) {
	baseRef := remote + "/" + base
	if !refExists(path, "refs/remotes/"+baseRef) {
		baseRef = base
		if !refExists(path, "refs/heads/"+base) {
			return false, false
		}
	}
	err := exec.Command("git", "-C", path, "merge-base", "--is-ancestor", branch, baseRef).Run()
	if err == nil {
		return true, true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, true
	}
	return false, false
}

func refExists(path, ref string) bool {
	return exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", ref).Run() == nil
}

// Remove removes a worktree (with optional --force).
func Remove(path string, force bool) error {
	return RemoveAt(".", path, force)
}

// RemoveAt removes a worktree registered to the repository containing repoPath.
func RemoveAt(repoPath, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", path)
	args = append([]string{"-C", repoPath}, args...)
	_, err := run("git", args...)
	if err != nil {
		return fmt.Errorf("git worktree remove %s: %w", path, err)
	}
	return nil
}

// DeleteBranch deletes a local branch after its worktree has been removed.
// Callers must establish that the branch is recoverable before using force.
func DeleteBranch(branch string, force bool) error {
	return DeleteBranchAt(".", branch, force)
}

// DeleteBranchAt deletes a local branch in a specific repository.
func DeleteBranchAt(repoPath, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run("git", "-C", repoPath, "branch", flag, "--", branch)
	if err != nil {
		return fmt.Errorf("git branch %s %s: %w", flag, branch, err)
	}
	return nil
}

// RootDir returns the root of the git repository.
func RootDir() (string, error) {
	out, err := run("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// MainRoot returns the root of the main (primary) worktree.
// This differs from RootDir when called from inside an added worktree.
func MainRoot() (string, error) {
	// --git-common-dir gives the path to the shared .git dir
	out, err := run("git", "rev-parse", "--git-common-dir")
	if err != nil {
		return RootDir()
	}
	gitDir := strings.TrimSpace(out)
	// git-common-dir is either an absolute path or relative to cwd.
	// For a main worktree: .git
	// For an added worktree: /path/to/main/.git
	if gitDir == ".git" {
		return RootDir()
	}
	// Strip trailing /.git (or /.git/worktrees/name)
	dir := gitDir
	for {
		base := filepath.Base(dir)
		if base == ".git" {
			parent := filepath.Dir(dir)
			return parent, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return RootDir()
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	// Fold git's stderr into the error so callers surface the actual reason
	// (e.g. "contains modified or untracked files") instead of "exit status 128".
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return string(out), err
}

func runIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// ParseRelativeAge converts git's relative age strings ("3 days ago") into a duration.
func ParseRelativeAge(s string) time.Duration {
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), " ago"))
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	switch {
	case strings.HasPrefix(parts[1], "second"):
		return time.Duration(n) * time.Second
	case strings.HasPrefix(parts[1], "minute"):
		return time.Duration(n) * time.Minute
	case strings.HasPrefix(parts[1], "hour"):
		return time.Duration(n) * time.Hour
	case strings.HasPrefix(parts[1], "day"):
		return time.Duration(n) * 24 * time.Hour
	case strings.HasPrefix(parts[1], "week"):
		return time.Duration(n) * 7 * 24 * time.Hour
	case strings.HasPrefix(parts[1], "month"):
		return time.Duration(n) * 30 * 24 * time.Hour
	case strings.HasPrefix(parts[1], "year"):
		return time.Duration(n) * 365 * 24 * time.Hour
	}
	return 0
}

// FormatAge formats a duration as a short human-readable string.
func FormatAge(d time.Duration) string {
	switch {
	case d == 0:
		return "—"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}

// ShortenPath shortens a path for display:
//  1. Strips the git root prefix (shows relative path)
//  2. Falls back to collapsing the home directory as ~
func ShortenPath(path, root string) string {
	if root != "" && strings.HasPrefix(path, root+"/") {
		return strings.TrimPrefix(path, root+"/")
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
