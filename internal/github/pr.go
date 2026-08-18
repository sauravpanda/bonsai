package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// PRInfo holds pull request metadata.
type PRInfo struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"` // OPEN, CLOSED, MERGED
	URL        string `json:"url"`
	HeadRefOID string `json:"headRefOid"`
}

var ErrNoPR = errors.New("no pull request for branch")

// IsAvailable returns true if gh CLI is installed and authenticated.
func IsAvailable() bool {
	return exec.Command("gh", "auth", "status").Run() == nil
}

// GetPR fetches PR info for the given branch. Returns nil if no PR exists.
func GetPR(branch string) (*PRInfo, error) {
	return GetPRAt("", branch)
}

// GetPRAt fetches pull request metadata using the repository at repoPath.
func GetPRAt(repoPath, branch string) (*PRInfo, error) {
	cmd := exec.Command(
		"gh", "pr", "view", branch,
		"--json", "number,title,state,url,headRefOid",
	)
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.ToLower(string(out))
		if strings.Contains(msg, "no pull requests found") ||
			strings.Contains(msg, "could not find pull request") {
			return nil, fmt.Errorf("%w: %q", ErrNoPR, branch)
		}
		return nil, fmt.Errorf("gh pr view %q: %w", branch, err)
	}
	var pr PRInfo
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
