package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestShouldDisableColor(t *testing.T) {
	tests := []struct {
		name        string
		flag        bool
		noColorEnv  string
		stdoutIsTTY bool
		want        bool
	}{
		{name: "interactive default", stdoutIsTTY: true, want: false},
		{name: "flag", flag: true, stdoutIsTTY: true, want: true},
		{name: "NO_COLOR", noColorEnv: "1", stdoutIsTTY: true, want: true},
		{name: "NO_COLOR zero is nonempty", noColorEnv: "0", stdoutIsTTY: true, want: true},
		{name: "redirected stdout", stdoutIsTTY: false, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldDisableColor(test.flag, test.noColorEnv, test.stdoutIsTTY); got != test.want {
				t.Fatalf("shouldDisableColor() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRootHasNoColorFlag(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("no-color") == nil {
		t.Fatal("root command is missing --no-color")
	}
}

func TestConfigureColorDisablesANSI(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(originalProfile)
	lipgloss.SetColorProfile(termenv.ANSI)

	configureColor(false, "", false)
	if got := lipgloss.ColorProfile(); got != termenv.Ascii {
		t.Fatalf("color profile = %v, want ASCII", got)
	}
}

func TestWritePRStatusNoteReportsAvailabilityFailureOnce(t *testing.T) {
	var output bytes.Buffer
	writePRStatusNote(&output, errors.New("gh auth status: authentication failed"), nil)
	got := output.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, "authentication failed") {
		t.Fatalf("unexpected note: %q", got)
	}
}

func TestWritePRStatusNoteAggregatesLookupFailures(t *testing.T) {
	var output bytes.Buffer
	writePRStatusNote(&output, nil, []error{
		errors.New("gh pr view feature-a: network unavailable"),
		errors.New("gh pr view feature-b: network unavailable"),
	})
	got := output.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, "2 GitHub PR lookups failed") || !strings.Contains(got, "network unavailable") {
		t.Fatalf("unexpected note: %q", got)
	}
}
