package git

import (
	"bufio"
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
	Path        string
	Branch      string
	HEAD        string
	IsMain      bool
	IsDetached  bool
	Age         time.Duration
	LastCommit  string
	AheadBase   int
	BehindBase  int
	PRStatus    string // "merged", "open", "closed", "none", "unknown"
	PRURL       string
	HasUnpushed bool
}

// List returns all git worktrees parsed from `git worktree list --porcelain`.
func List() ([]*Worktree, error) {
	out, err := run("git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parsePorcelain(out), nil
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
		}
	}
	return worktrees
}

// Enrich fetches age, last commit message, ahead/behind, and unpushed status.
func Enrich(wt *Worktree, base, remote string) {
	out, err := runIn(wt.Path, "git", "log", "-1", "--format=%ar|%s")
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
		if len(parts) == 2 {
			wt.Age = ParseRelativeAge(parts[0])
			wt.LastCommit = parts[1]
		}
	}

	if wt.IsDetached || wt.Branch == "" || wt.Branch == "(detached)" {
		return
	}

	wt.AheadBase, wt.BehindBase = aheadBehind(wt.Path, wt.Branch, base, remote)
	wt.HasUnpushed = hasUnpushed(wt.Path, wt.Branch, base, remote)
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

func hasUnpushed(path, branch, base, remote string) bool {
	remoteRef := remote + "/" + branch
	out, err := runIn(path, "git", "rev-list", "--count", remoteRef+".."+branch)
	if err != nil {
		// No remote branch exists yet. Count only commits unique to this branch
		// relative to the configured base branch instead of the full history.
		baseRef := remote + "/" + base
		out2, err2 := runIn(path, "git", "rev-list", "--count", baseRef+".."+branch)
		if err2 != nil {
			out2, err2 = runIn(path, "git", "rev-list", "--count", base+".."+branch)
			if err2 != nil {
				return false
			}
		}
		n, _ := strconv.Atoi(strings.TrimSpace(out2))
		return n > 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n > 0
}

// Remove removes a worktree (with optional --force).
func Remove(path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := run("git", args...)
	if err != nil {
		return fmt.Errorf("git worktree remove %s: %w", path, err)
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
