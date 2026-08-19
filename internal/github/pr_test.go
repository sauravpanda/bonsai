package github

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandErrorIncludesCommandOutput(t *testing.T) {
	err := commandError("gh pr view feature", errors.New("exit status 1"), []byte("HTTP 503\ntry again"))
	if !strings.Contains(err.Error(), "HTTP 503 try again") {
		t.Fatalf("command output missing from error: %v", err)
	}
}

func TestCommandErrorFallsBackToExitError(t *testing.T) {
	err := commandError("gh auth status", errors.New("exit status 1"), nil)
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("exit error missing: %v", err)
	}
}
