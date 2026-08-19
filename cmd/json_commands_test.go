package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

func TestJSONFlagsAreAvailable(t *testing.T) {
	for _, command := range []*cobra.Command{statsCmd, statusCmd, syncCmd} {
		if command.Flags().Lookup("json") == nil {
			t.Fatalf("%s command is missing --json", command.Name())
		}
	}
}

func TestBuildStatsJSON(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "data"), []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	metrics := buildStatsJSON([]*git.Worktree{
		{Path: path, Age: 12 * time.Hour, PRStatus: "open", HasUnpushed: true},
		{Path: t.TempDir(), Age: 10 * 24 * time.Hour},
	}, float64(7*24*time.Hour), 7)

	if metrics.TotalWorktrees != 2 || metrics.StaleWorktrees != 1 || metrics.OpenPRs != 1 || metrics.UnpushedWorktrees != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if metrics.DiskBytes != 4 || metrics.AgeDistribution.UnderOneDay != 1 || metrics.AgeDistribution.SevenToThirtyDays != 1 {
		t.Fatalf("unexpected disk or age metrics: %+v", metrics)
	}
}

func TestBuildStatusJSON(t *testing.T) {
	statuses := []wtStatus{{
		wt: &git.Worktree{
			Path: "/tmp/example", Branch: "feature", HEAD: "abc", Age: 2 * time.Hour,
			AheadBase: 2, BehindBase: 1, LastCommit: "work", StatusKnown: true,
		},
		dirty: 1, staged: 2, untracked: 3,
	}}
	out := buildStatusJSON(statuses)
	if len(out) != 1 || out[0].Branch != "feature" || out[0].DirtyFiles != 1 || out[0].StagedFiles != 2 || out[0].UntrackedFiles != 3 {
		t.Fatalf("unexpected status output: %+v", out)
	}
}

func TestSyncJSONResultUsesStableResultValues(t *testing.T) {
	output := syncJSONOutput{
		Base: "main", Remote: "origin", Strategy: "rebase",
		Results: []syncJSONResult{
			{Path: "/tmp/a", Branch: "a", Result: "synced"},
			{Path: "/tmp/b", Branch: "b", Result: "skipped", Reason: "dirty"},
			{Path: "/tmp/c", Branch: "c", Result: "failed", Error: "conflict"},
		},
		Summary: syncJSONSummary{Synced: 1, Skipped: 1, Failed: 1},
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["results"]; !ok {
		t.Fatalf("results missing from JSON: %s", encoded)
	}
}
