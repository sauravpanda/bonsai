package github

import (
	"encoding/json"
	"fmt"
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

