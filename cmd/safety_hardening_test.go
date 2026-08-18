package cmd

import (
	"strings"
	"testing"

	"github.com/sauravpanda/bonsai/internal/git"
)

func TestMatchesNoPRFilterExcludesUnknownStatus(t *testing.T) {
	tests := []struct {
		name string
		wt   *git.Worktree
		want bool
	}{
		{name: "main", wt: &git.Worktree{IsMain: true, PRStatus: "unknown"}, want: true},
		{name: "no PR", wt: &git.Worktree{PRStatus: "none"}, want: true},
		{name: "unknown", wt: &git.Worktree{PRStatus: "unknown"}, want: false},
		{name: "open", wt: &git.Worktree{PRStatus: "open"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesNoPRFilter(test.wt); got != test.want {
				t.Fatalf("matchesNoPRFilter() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestReconcileMergedPRStatusClearsFalseUnpushedWarning(t *testing.T) {
	wt := &git.Worktree{
		HEAD:            "abc123",
		PRStatus:        "merged",
		PRHeadOID:       "abc123",
		HasUnpushed:     true,
		UnpushedCommits: 3,
	}
	reconcileMergedPRStatus(wt)
	if wt.HasUnpushed || wt.UnpushedCommits != 0 || !wt.UnpushedKnown {
		t.Fatalf("matching merged PR head was not reconciled: %+v", wt)
	}
}

func TestReconcileMergedPRStatusKeepsCommitsAfterPRHead(t *testing.T) {
	wt := &git.Worktree{
		HEAD:            "new-work",
		PRStatus:        "merged",
		PRHeadOID:       "merged-head",
		HasUnpushed:     true,
		UnpushedCommits: 1,
	}
	reconcileMergedPRStatus(wt)
	if !wt.HasUnpushed || wt.UnpushedCommits != 1 {
		t.Fatalf("post-merge work was incorrectly cleared: %+v", wt)
	}
}

func TestParseWorktreeNumberUsesPortableRange(t *testing.T) {
	if got, err := parseWorktreeNumber("2", 3); err != nil || got != 2 {
		t.Fatalf("parseWorktreeNumber() = %d, %v", got, err)
	}
	_, err := parseWorktreeNumber("4", 3)
	if err == nil || !strings.Contains(err.Error(), "1-3") {
		t.Fatalf("expected ASCII range in error, got %v", err)
	}
	if strings.Contains(err.Error(), "–") {
		t.Fatalf("error contains a non-ASCII en dash: %v", err)
	}
}

func TestApprovePushRemoval(t *testing.T) {
	prompted := false
	yes, quit := approvePushRemoval(true, func() (bool, bool) {
		prompted = true
		return false, false
	})
	if !yes || quit || prompted {
		t.Fatalf("--yes should approve without prompting: yes=%t quit=%t prompted=%t", yes, quit, prompted)
	}

	yes, quit = approvePushRemoval(false, func() (bool, bool) {
		return false, true
	})
	if yes || !quit {
		t.Fatalf("manual cancellation was not preserved: yes=%t quit=%t", yes, quit)
	}
}

func TestAbortFailedSyncReportsAbortFailure(t *testing.T) {
	err := abortFailedSync(t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "git rebase --abort") {
		t.Fatalf("expected actionable abort failure, got %v", err)
	}
}
