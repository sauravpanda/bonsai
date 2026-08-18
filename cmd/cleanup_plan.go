package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"golang.org/x/sync/errgroup"
)

const (
	cleanupSchemaVersion = 2
	cleanupPlanTTL       = 15 * time.Minute
	cleanupWorkers       = 8
)

type cleanupState string

const (
	cleanupSafe      cleanupState = "safe"
	cleanupReview    cleanupState = "review"
	cleanupProtected cleanupState = "protected"
)

type cleanupItem struct {
	RepositoryRoot     string       `json:"repository_root"`
	RepositoryName     string       `json:"repository_name"`
	Path               string       `json:"path"`
	ShortPath          string       `json:"short_path"`
	Branch             string       `json:"branch"`
	HEAD               string       `json:"head"`
	State              cleanupState `json:"state"`
	Reasons            []string     `json:"reasons"`
	AgeSeconds         int64        `json:"age_seconds"`
	AgeHuman           string       `json:"age_human"`
	PRStatus           string       `json:"pr_status,omitempty"`
	PRNumber           int          `json:"pr_number,omitempty"`
	PRURL              string       `json:"pr_url,omitempty"`
	DirtyFiles         int          `json:"dirty_files"`
	StagedFiles        int          `json:"staged_files"`
	UntrackedFiles     int          `json:"untracked_files"`
	UnpushedCommits    int          `json:"unpushed_commits"`
	RemoteBranchExists bool         `json:"remote_branch_exists"`
	MergedIntoBase     bool         `json:"merged_into_base"`
	IsInUse            bool         `json:"is_in_use"`
	InUseKnown         bool         `json:"in_use_known"`
	DefaultBase        string       `json:"default_base"`
	DefaultRemote      string       `json:"default_remote"`
	StaleThresholdDays int          `json:"stale_threshold_days"`
	DiskBytes          int64        `json:"disk_bytes"`
	DeleteBranch       bool         `json:"delete_local_branch"`
	Fingerprint        string       `json:"fingerprint"`
}

type cleanupSummary struct {
	Repositories     int   `json:"repositories"`
	Scanned          int   `json:"scanned"`
	Safe             int   `json:"safe"`
	Review           int   `json:"review"`
	Protected        int   `json:"protected"`
	ReclaimableBytes int64 `json:"reclaimable_bytes"`
}

type cleanupPlan struct {
	SchemaVersion      int            `json:"schema_version"`
	PlanID             string         `json:"plan_id"`
	GeneratedAt        time.Time      `json:"generated_at"`
	ExpiresAt          time.Time      `json:"expires_at"`
	Scope              string         `json:"scope"`
	ScanRoots          []string       `json:"scan_roots,omitempty"`
	Warnings           []string       `json:"warnings"`
	StaleThresholdDays int            `json:"stale_threshold_days"`
	Summary            cleanupSummary `json:"summary"`
	Safe               []cleanupItem  `json:"safe"`
	Review             []cleanupItem  `json:"review"`
	Protected          []cleanupItem  `json:"protected"`
}

type cleanupPlanOptions struct {
	ClaudeOnly       bool
	Offline          bool
	Progress         bool
	Global           bool
	ScanRoots        []string
	IncludeAll       bool
	StaleOverride    int
	HasStaleOverride bool
}

type cleanupTarget struct {
	wt   *git.Worktree
	repo string
	cfg  *config.Config
}

func buildCleanupPlan(cfg *config.Config, opts cleanupPlanOptions) (*cleanupPlan, error) {
	currentRoot, _ := git.RootDir()
	activeDirs, activeDirsKnown := activeWorkingDirectories()
	ghOK := !opts.Offline && github.IsAvailable()

	var (
		repositories []string
		scanRoots    []string
		warnings     []string
	)
	if opts.Global {
		var err error
		scanRoots, err = normalizeScanRoots(opts.ScanRoots)
		if err != nil {
			return nil, err
		}
		repositories, err = discoverRepositories(scanRoots)
		if err != nil {
			return nil, err
		}
	} else {
		mainRoot, err := git.MainRoot()
		if err != nil {
			return nil, fmt.Errorf("find main worktree: %w", err)
		}
		repositories = []string{mainRoot}
	}

	var targets []cleanupTarget
	for _, repository := range repositories {
		repoCfg := cfg
		if opts.Global {
			var err error
			repoCfg, err = config.LoadForRepo(repository)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: load config: %v", repository, err))
				continue
			}
		}
		if opts.HasStaleOverride {
			repoCfg.StaleThresholdDays = opts.StaleOverride
		}
		worktrees, err := git.ListAt(repository)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		for _, wt := range worktrees {
			if wt.IsMain {
				continue
			}
			if opts.ClaudeOnly && !isClaudeWorktree(repository, wt.Path) {
				continue
			}
			targets = append(targets, cleanupTarget{wt: wt, repo: repository, cfg: repoCfg})
		}
	}

	var spin *tui.Spinner
	if opts.Progress {
		spin = tui.Start(fmt.Sprintf("analyzing %d worktree(s) safely…", len(targets)))
	}
	sizes := make([]int64, len(targets))
	var group errgroup.Group
	group.SetLimit(cleanupWorkers)
	for i, target := range targets {
		i, target := i, target
		group.Go(func() error {
			git.Enrich(target.wt, target.cfg.DefaultBase, target.cfg.DefaultRemote)
			sizes[i] = dirSize(target.wt.Path)
			if ghOK && !target.wt.IsDetached && target.wt.Branch != "" {
				pr, prErr := github.GetPRAt(target.repo, target.wt.Branch)
				switch {
				case prErr == nil:
					target.wt.PRStatus = strings.ToLower(pr.State)
					target.wt.PRURL = pr.URL
					target.wt.PRNumber = pr.Number
					target.wt.PRHeadOID = pr.HeadRefOID
				case errors.Is(prErr, github.ErrNoPR):
					target.wt.PRStatus = "none"
				default:
					target.wt.PRStatus = "unknown"
				}
			}
			return nil
		})
	}
	_ = group.Wait()
	if spin != nil {
		spin.Stop()
	}

	now := time.Now().UTC()
	plan := &cleanupPlan{
		SchemaVersion:      cleanupSchemaVersion,
		GeneratedAt:        now,
		ExpiresAt:          now.Add(cleanupPlanTTL),
		Scope:              "all",
		ScanRoots:          scanRoots,
		Warnings:           warnings,
		StaleThresholdDays: cfg.StaleThresholdDays,
		Safe:               []cleanupItem{},
		Review:             []cleanupItem{},
		Protected:          []cleanupItem{},
	}
	if opts.ClaudeOnly {
		plan.Scope = "claude"
	}
	if opts.Global {
		plan.Scope = "global"
		if opts.ClaudeOnly {
			plan.Scope = "global-claude"
		}
	}

	for i, target := range targets {
		wt := target.wt
		wt.InUseKnown = activeDirsKnown
		wt.IsInUse = pathContainsAny(wt.Path, activeDirs)
		staleDur := time.Duration(target.cfg.StaleThresholdDays) * 24 * time.Hour
		state, reasons, candidate := classifyCleanup(wt, currentRoot, staleDur)
		if !candidate {
			if !opts.IncludeAll {
				continue
			}
			state = cleanupReview
			reasons = []string{"recent activity; not a cleanup candidate"}
		}
		item := cleanupItem{
			RepositoryRoot:     target.repo,
			RepositoryName:     filepath.Base(target.repo),
			Path:               wt.Path,
			ShortPath:          git.ShortenPath(wt.Path, target.repo),
			Branch:             wt.Branch,
			HEAD:               wt.HEAD,
			State:              state,
			Reasons:            reasons,
			AgeSeconds:         int64(wt.Age / time.Second),
			AgeHuman:           git.FormatAge(wt.Age),
			PRStatus:           wt.PRStatus,
			PRNumber:           wt.PRNumber,
			PRURL:              wt.PRURL,
			DirtyFiles:         wt.DirtyFiles,
			StagedFiles:        wt.StagedFiles,
			UntrackedFiles:     wt.UntrackedFiles,
			UnpushedCommits:    wt.UnpushedCommits,
			RemoteBranchExists: wt.RemoteBranchExists,
			MergedIntoBase:     wt.MergedIntoBase,
			IsInUse:            wt.IsInUse,
			InUseKnown:         wt.InUseKnown,
			DefaultBase:        target.cfg.DefaultBase,
			DefaultRemote:      target.cfg.DefaultRemote,
			StaleThresholdDays: target.cfg.StaleThresholdDays,
			DiskBytes:          sizes[i],
			DeleteBranch:       state == cleanupSafe && wt.Branch != "" && !wt.IsDetached,
			Fingerprint:        cleanupFingerprint(wt, currentRoot),
		}
		switch state {
		case cleanupSafe:
			plan.Safe = append(plan.Safe, item)
			plan.Summary.ReclaimableBytes += item.DiskBytes
		case cleanupReview:
			plan.Review = append(plan.Review, item)
		case cleanupProtected:
			plan.Protected = append(plan.Protected, item)
		}
	}

	plan.Summary.Repositories = len(repositories)
	plan.Summary.Scanned = len(targets)
	plan.Summary.Safe = len(plan.Safe)
	plan.Summary.Review = len(plan.Review)
	plan.Summary.Protected = len(plan.Protected)
	plan.PlanID = cleanupPlanID(plan)
	return plan, nil
}

func classifyCleanup(wt *git.Worktree, currentRoot string, staleDur time.Duration) (cleanupState, []string, bool) {
	var reasons []string
	mergedPR := wt.PRStatus == "merged"
	stale := wt.Age > 0 && wt.Age >= staleDur
	if mergedPR {
		if wt.PRNumber > 0 {
			reasons = append(reasons, fmt.Sprintf("PR #%d merged", wt.PRNumber))
		} else {
			reasons = append(reasons, "merged PR")
		}
	}
	if stale {
		reasons = append(reasons, fmt.Sprintf("inactive for %s", git.FormatAge(wt.Age)))
	}
	if !mergedPR && !stale && !wt.IsPrunable {
		return "", nil, false
	}

	protect := func(reason string) (cleanupState, []string, bool) {
		return cleanupProtected, append(reasons, reason), true
	}
	if samePath(wt.Path, currentRoot) {
		return protect("current worktree")
	}
	if wt.IsInUse {
		return protect("another process is using this worktree")
	}
	if wt.IsLocked {
		reason := "worktree is locked"
		if wt.LockReason != "" {
			reason += ": " + wt.LockReason
		}
		return protect(reason)
	}
	if wt.IsDetached {
		return protect("detached HEAD")
	}
	if wt.IsPrunable {
		reason := "broken worktree metadata needs review"
		if wt.PrunableReason != "" {
			reason += ": " + wt.PrunableReason
		}
		return cleanupReview, append(reasons, reason), true
	}
	if !wt.StatusKnown {
		return protect("working tree status could not be inspected")
	}
	if wt.HasWorkingChanges() {
		return protect(formatWorkingChanges(wt))
	}
	if !wt.UnpushedKnown {
		return protect("unpushed commit status could not be verified")
	}
	if wt.PRStatus == "open" {
		return protect("pull request is still open")
	}
	if wt.PRStatus == "unknown" {
		return protect("pull request status is unknown")
	}

	prHeadRecoverable := mergedPR && wt.PRHeadOID != "" && wt.HEAD == wt.PRHeadOID
	if wt.UnpushedCommits > 0 && !prHeadRecoverable {
		return protect(fmt.Sprintf("%d unpushed commit(s)", wt.UnpushedCommits))
	}
	if wt.PRStatus == "closed" {
		return cleanupReview, append(reasons, "pull request was closed without merging"), true
	}

	recoverable := prHeadRecoverable || wt.RemoteBranchExists || (wt.BaseStatusKnown && wt.MergedIntoBase)
	if !recoverable {
		return cleanupReview, append(reasons, "branch recovery could not be proven"), true
	}
	if wt.RemoteBranchExists {
		reasons = append(reasons, "backed up on remote")
	} else if wt.MergedIntoBase {
		reasons = append(reasons, "contained in base branch")
	} else if prHeadRecoverable {
		reasons = append(reasons, "head recorded by merged PR")
	}
	reasons = append(reasons, "working tree clean")
	return cleanupSafe, reasons, true
}

func formatWorkingChanges(wt *git.Worktree) string {
	var parts []string
	if wt.StagedFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", wt.StagedFiles))
	}
	if wt.DirtyFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", wt.DirtyFiles))
	}
	if wt.UntrackedFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", wt.UntrackedFiles))
	}
	return strings.Join(parts, ", ")
}

func isClaudeWorktree(mainRoot, path string) bool {
	claudeRoot := filepath.Join(mainRoot, ".claude", "worktrees")
	return pathWithin(claudeRoot, path) && !samePath(claudeRoot, path)
}

func activeWorkingDirectories() ([]string, bool) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil, false
	}
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-Fn").Output()
	if err != nil {
		return nil, false
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/") {
			paths = append(paths, strings.TrimPrefix(line, "n"))
		}
	}
	return paths, true
}

func pathContainsAny(root string, paths []string) bool {
	for _, path := range paths {
		if pathWithin(root, path) {
			return true
		}
	}
	return false
}

func pathWithin(root, path string) bool {
	root = canonicalPath(root)
	path = canonicalPath(path)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return canonicalPath(left) == canonicalPath(right)
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = filepath.Clean(path)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute)
}

func cleanupFingerprint(wt *git.Worktree, currentRoot string) string {
	data := fmt.Sprintf("%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%t\x00%t\x00%t\x00%d\x00%d\x00%d\x00%t\x00%d\x00%t\x00%t\x00%t",
		filepath.Clean(wt.Path), wt.Branch, wt.HEAD,
		samePath(wt.Path, currentRoot), wt.IsInUse, wt.InUseKnown, wt.IsLocked, wt.IsDetached, wt.StatusKnown,
		wt.DirtyFiles, wt.StagedFiles, wt.UntrackedFiles,
		wt.UnpushedKnown, wt.UnpushedCommits, wt.RemoteBranchExists,
		wt.MergedIntoBase, wt.BaseStatusKnown,
	)
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func cleanupPlanID(plan *cleanupPlan) string {
	var fingerprints []string
	for _, item := range plan.Safe {
		fingerprints = append(fingerprints, strings.Join([]string{
			filepath.Clean(item.Path), item.Branch, item.HEAD, item.Fingerprint,
		}, "\x00"))
	}
	sort.Strings(fingerprints)
	payload := fmt.Sprintf("%d\x00%s\x00%s\x00%s", plan.SchemaVersion, plan.Scope, plan.GeneratedAt.Format(time.RFC3339Nano), strings.Join(fingerprints, "\x00"))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8])
}

func cleanupPlanDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "bonsai", "plans")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Plans are deliberately short-lived. Remove expired plan artifacts so
	// repeated agent analyses do not accumulate files in the user's cache.
	entries, _ := os.ReadDir(dir)
	cutoff := time.Now().Add(-cleanupPlanTTL)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return dir, nil
}

func saveCleanupPlan(plan *cleanupPlan) error {
	dir, err := cleanupPlanDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, plan.PlanID+".json"), data, 0o600)
}

func loadCleanupPlan(planID string) (*cleanupPlan, string, error) {
	if len(planID) != 16 {
		return nil, "", fmt.Errorf("invalid cleanup plan ID %q", planID)
	}
	if _, err := hex.DecodeString(planID); err != nil {
		return nil, "", fmt.Errorf("invalid cleanup plan ID %q", planID)
	}
	dir, err := cleanupPlanDir()
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, planID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("cleanup plan %s was not found; create a new plan", planID)
		}
		return nil, "", err
	}
	var plan cleanupPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, "", fmt.Errorf("read cleanup plan: %w", err)
	}
	if plan.SchemaVersion != cleanupSchemaVersion || plan.PlanID != planID || cleanupPlanID(&plan) != planID {
		return nil, "", fmt.Errorf("cleanup plan %s is invalid or incompatible", planID)
	}
	if time.Now().After(plan.ExpiresAt) {
		return nil, "", fmt.Errorf("cleanup plan %s expired; create a new plan", planID)
	}
	return &plan, path, nil
}

type cleanupApplyResult struct {
	Removed          int      `json:"removed"`
	BranchesDeleted  int      `json:"branches_deleted"`
	ReclaimedBytes   int64    `json:"reclaimed_bytes"`
	BranchDeleteFail []string `json:"branch_delete_failures,omitempty"`
}

func validateCleanupItem(item cleanupItem) error {
	currentRoot, _ := git.RootDir()
	activeDirs, activeDirsKnown := activeWorkingDirectories()
	listed, err := git.ListAt(item.RepositoryRoot)
	if err != nil {
		return err
	}
	for _, wt := range listed {
		if !samePath(wt.Path, item.Path) {
			continue
		}
		wt.InUseKnown = activeDirsKnown
		wt.IsInUse = pathContainsAny(wt.Path, activeDirs)
		git.Enrich(wt, item.DefaultBase, item.DefaultRemote)
		if got := cleanupFingerprint(wt, currentRoot); got != item.Fingerprint {
			return fmt.Errorf("worktree changed since planning: %s; create a new plan", item.ShortPath)
		}
		return nil
	}
	return fmt.Errorf("worktree changed since planning: %s no longer exists", item.ShortPath)
}

func applyCleanupPlan(planID string) (cleanupApplyResult, error) {
	plan, planPath, err := loadCleanupPlan(planID)
	if err != nil {
		return cleanupApplyResult{}, err
	}
	if len(plan.Safe) == 0 {
		_ = os.Remove(planPath)
		return cleanupApplyResult{}, nil
	}

	currentRoot, _ := git.RootDir()
	activeDirs, activeDirsKnown := activeWorkingDirectories()
	// Validate every target before mutating any worktree. This makes plan/apply
	// atomic with respect to safety checks even though git removals themselves
	// are necessarily executed one at a time.
	for _, item := range plan.Safe {
		listed, listErr := git.ListAt(item.RepositoryRoot)
		if listErr != nil {
			return cleanupApplyResult{}, listErr
		}
		var wt *git.Worktree
		for _, candidate := range listed {
			if samePath(candidate.Path, item.Path) {
				wt = candidate
				break
			}
		}
		if wt == nil {
			return cleanupApplyResult{}, fmt.Errorf("worktree changed since planning: %s no longer exists", item.ShortPath)
		}
		wt.InUseKnown = activeDirsKnown
		wt.IsInUse = pathContainsAny(wt.Path, activeDirs)
		git.Enrich(wt, item.DefaultBase, item.DefaultRemote)
		if got := cleanupFingerprint(wt, currentRoot); got != item.Fingerprint {
			return cleanupApplyResult{}, fmt.Errorf("worktree changed since planning: %s; create a new plan", item.ShortPath)
		}
	}

	var result cleanupApplyResult
	for _, item := range plan.Safe {
		if err := git.RemoveAt(item.RepositoryRoot, item.Path, false); err != nil {
			return result, fmt.Errorf("remove %s: %w", item.ShortPath, err)
		}
		result.Removed++
		result.ReclaimedBytes += item.DiskBytes
		if item.DeleteBranch {
			if err := git.DeleteBranchAt(item.RepositoryRoot, item.Branch, true); err != nil {
				result.BranchDeleteFail = append(result.BranchDeleteFail, item.Branch+": "+err.Error())
			} else {
				result.BranchesDeleted++
			}
		}
	}
	_ = os.Remove(planPath)
	return result, nil
}
