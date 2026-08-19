package cmd

import (
	"testing"

	"github.com/sauravpanda/bonsai/internal/config"
)

func TestConfigCheckHasJSONFlag(t *testing.T) {
	if configCheckCmd.Flags().Lookup("json") == nil {
		t.Fatal("config check command is missing --json")
	}
}

func TestDiagnosticLocation(t *testing.T) {
	got := diagnosticLocation(config.Diagnostic{Path: "/tmp/.bonsai.toml", Line: 3, Key: "default_base"})
	want := "/tmp/.bonsai.toml:3 [default_base]"
	if got != want {
		t.Fatalf("diagnosticLocation() = %q, want %q", got, want)
	}
}
