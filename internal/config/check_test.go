package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourcesMergesLayersAndWarnsOnUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "config.toml")
	repoPath := filepath.Join(dir, ".bonsai.toml")
	writeConfigTestFile(t, globalPath, "stale_threshold_days = 21\ndefault_remote = \"upstream\"\n")
	writeConfigTestFile(t, repoPath, "default_base = \"develop\"\nstale_threshhold_days = 7\n")

	result := checkSources([]Source{
		{Scope: "global", Path: globalPath},
		{Scope: "repository", Path: repoPath},
	})
	if !result.Valid || result.Effective == nil {
		t.Fatalf("expected valid config, got %+v", result)
	}
	if result.Effective.StaleThresholdDays != 21 || result.Effective.DefaultRemote != "upstream" || result.Effective.DefaultBase != "develop" {
		t.Fatalf("layers were not merged: %+v", result.Effective)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Key != "stale_threshhold_days" || result.Warnings[0].Line != 2 {
		t.Fatalf("unexpected warnings: %+v", result.Warnings)
	}
}

func TestCheckSourcesReportsSyntaxErrorWithPathAndLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigTestFile(t, path, "default_base = \"main\"\nticket_pattern = [\n")

	result := checkSources([]Source{{Scope: "global", Path: path}})
	if result.Valid || result.Effective != nil || len(result.Errors) != 1 {
		t.Fatalf("expected one invalid result: %+v", result)
	}
	if result.Errors[0].Path != path || result.Errors[0].Line != 2 {
		t.Fatalf("syntax location missing: %+v", result.Errors[0])
	}
	if strings.Contains(result.Errors[0].Message, "toml: line") {
		t.Fatalf("syntax message duplicates the structured line: %+v", result.Errors[0])
	}
}

func TestLoadForRepoIncludesInvalidSourcePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	path := filepath.Join(repo, ".bonsai.toml")
	writeConfigTestFile(t, path, "stale_threshold_days = \"soon\"\n")

	_, err := LoadForRepo(repo)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected source path in load error, got %v", err)
	}
}

func TestCheckSourcesReportsTypeAndRangeErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
		message string
	}{
		{name: "wrong type", content: "stale_threshold_days = \"soon\"\n", key: "stale_threshold_days", message: "incompatible types"},
		{name: "nonpositive stale threshold", content: "stale_threshold_days = 0\n", key: "stale_threshold_days", message: "positive integer"},
		{name: "empty remote", content: "default_remote = \"\"\n", key: "default_remote", message: "must not be empty"},
		{name: "invalid regexp", content: "ticket_pattern = \"[\"\n", key: "ticket_pattern", message: "regular expression"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			writeConfigTestFile(t, path, test.content)
			result := checkSources([]Source{{Scope: "global", Path: path}})
			if result.Valid || len(result.Errors) != 1 {
				t.Fatalf("expected one error, got %+v", result)
			}
			if result.Errors[0].Key != test.key || result.Errors[0].Line != 1 || !strings.Contains(result.Errors[0].Message, test.message) {
				t.Fatalf("unexpected diagnostic: %+v", result.Errors[0])
			}
		})
	}
}

func TestCheckSourcesUsesDefaultsWhenFilesAreMissing(t *testing.T) {
	result := checkSources([]Source{{Scope: "global", Path: filepath.Join(t.TempDir(), "missing.toml")}})
	if !result.Valid || result.Effective == nil || result.Sources[0].Exists || !result.Sources[0].Valid {
		t.Fatalf("missing source should use defaults: %+v", result)
	}
	if *result.Effective != *Default() {
		t.Fatalf("unexpected defaults: %+v", result.Effective)
	}
}

func writeConfigTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
