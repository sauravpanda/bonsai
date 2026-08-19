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
	return CheckAvailable() == nil
}

// CheckAvailable verifies that gh is installed and authenticated, preserving
// the command's diagnostic output for callers that need to explain failures.
func CheckAvailable() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		return commandError("gh auth status", err, out)
	}
	return nil
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
		return nil, commandError(fmt.Sprintf("gh pr view %q", branch), err, out)
	}
	var pr PRInfo
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func commandError(operation string, err error, output []byte) error {
	detail := strings.Join(strings.Fields(string(output)), " ")
	if detail == "" {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s: %s", operation, detail)
}
