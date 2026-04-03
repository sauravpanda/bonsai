package github

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// PRInfo holds pull request metadata.
type PRInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"` // OPEN, CLOSED, MERGED
	URL    string `json:"url"`
}

// IsAvailable returns true if gh CLI is installed and authenticated.
func IsAvailable() bool {
	return exec.Command("gh", "auth", "status").Run() == nil
}

// GetPR fetches PR info for the given branch. Returns nil if no PR exists.
func GetPR(branch string) (*PRInfo, error) {
	out, err := exec.Command(
		"gh", "pr", "view", branch,
		"--json", "number,title,state,url",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("no PR for branch %q", branch)
	}
	var pr PRInfo
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// CreatePR opens a PR for the current branch using gh pr create --fill.
// If web is true, opens the browser instead of printing the URL.
func CreatePR(web bool) error {
	args := []string{"pr", "create", "--fill"}
	if web {
		args = append(args, "--web")
	}
	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr create: %w", err)
	}
	return nil
}
