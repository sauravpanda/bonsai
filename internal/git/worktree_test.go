package git

import "testing"

func TestParsePorcelainIncludesSafetyMetadata(t *testing.T) {
	out := `worktree /repo
HEAD abc
branch refs/heads/main

worktree /repo/.claude/worktrees/task
HEAD def
branch refs/heads/worktree-task
locked session active
prunable gitdir file points to non-existent location
`
	worktrees := parsePorcelain(out)
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}
	wt := worktrees[1]
	if !wt.IsLocked || wt.LockReason != "session active" {
		t.Fatalf("locked metadata not parsed: %+v", wt)
	}
	if !wt.IsPrunable || wt.PrunableReason != "gitdir file points to non-existent location" {
		t.Fatalf("prunable metadata not parsed: %+v", wt)
	}
}

func TestParseWorkingTreeStatus(t *testing.T) {
	dirty, staged, untracked := parseWorkingTreeStatus(" M dirty.txt\nM  staged.txt\nMM both.txt\n?? new.txt\n")
	if dirty != 2 || staged != 2 || untracked != 1 {
		t.Fatalf("got dirty=%d staged=%d untracked=%d", dirty, staged, untracked)
	}
}

func TestParseRelativeAge(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"3 days ago", "72h0m0s"},
		{"2 weeks ago", "336h0m0s"},
		{"15 minutes ago", "15m0s"},
		{"bad input", "0s"},
	}

	for _, tt := range tests {
		if got := ParseRelativeAge(tt.in).String(); got != tt.want {
			t.Fatalf("ParseRelativeAge(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
