package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Source identifies one config layer in merge order.
type Source struct {
	Scope string `json:"scope"`
	Path  string `json:"path"`
}

// SourceStatus describes whether a config source exists and passed validation.
type SourceStatus struct {
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Valid  bool   `json:"valid"`
}

// Diagnostic is a source-aware config warning or error.
type Diagnostic struct {
	Path    string `json:"path"`
	Line    int    `json:"line,omitempty"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
}

// CheckResult is the stable machine-readable result returned by config check.
type CheckResult struct {
	Valid     bool           `json:"valid"`
	Sources   []SourceStatus `json:"sources"`
	Warnings  []Diagnostic   `json:"warnings"`
	Errors    []Diagnostic   `json:"errors"`
	Effective *Config        `json:"effective,omitempty"`
}

var knownKeys = map[string]struct{}{
	"stale_threshold_days": {},
	"default_remote":       {},
	"default_base":         {},
	"ticket_pattern":       {},
}

var tomlLinePattern = regexp.MustCompile(`^toml: line ([0-9]+)(?: \(last key "([^"]+)"\))?: (.*)$`)

// Check validates the global and current-repository config layers.
func Check() CheckResult {
	return CheckForRepo(gitRepoRoot())
}

// CheckForRepo validates the global and repository config layers for repoRoot.
func CheckForRepo(repoRoot string) CheckResult {
	sources := []Source{{Scope: "global", Path: Path()}}
	if repoRoot != "" {
		if absolute, err := filepath.Abs(repoRoot); err == nil {
			repoRoot = absolute
		}
		sources = append(sources, Source{
			Scope: "repository",
			Path:  filepath.Join(repoRoot, ".bonsai.toml"),
		})
	}
	return checkSources(sources)
}

func checkSources(sources []Source) CheckResult {
	result := CheckResult{
		Sources:  make([]SourceStatus, 0, len(sources)),
		Warnings: make([]Diagnostic, 0),
		Errors:   make([]Diagnostic, 0),
	}
	effective := Default()

	for _, source := range sources {
		status := SourceStatus{Scope: source.Scope, Path: source.Path, Valid: true}
		info, err := os.Stat(source.Path)
		if errors.Is(err, os.ErrNotExist) {
			result.Sources = append(result.Sources, status)
			continue
		}
		if err != nil {
			status.Valid = false
			result.Errors = append(result.Errors, Diagnostic{Path: source.Path, Message: err.Error()})
			result.Sources = append(result.Sources, status)
			continue
		}
		status.Exists = true
		if info.IsDir() {
			status.Valid = false
			result.Errors = append(result.Errors, Diagnostic{Path: source.Path, Message: "config path is a directory"})
			result.Sources = append(result.Sources, status)
			continue
		}

		next := *effective
		metadata, decodeErr := toml.DecodeFile(source.Path, &next)
		result.Warnings = append(result.Warnings, unknownKeyDiagnostics(source.Path, metadata)...)
		if decodeErr != nil {
			status.Valid = false
			result.Errors = append(result.Errors, decodeDiagnostic(source.Path, decodeErr))
			result.Sources = append(result.Sources, status)
			continue
		}

		validationErrors := validateDefinedValues(source.Path, metadata, &next)
		if len(validationErrors) > 0 {
			status.Valid = false
			result.Errors = append(result.Errors, validationErrors...)
		} else {
			effective = &next
		}
		result.Sources = append(result.Sources, status)
	}

	result.Valid = len(result.Errors) == 0
	if result.Valid {
		result.Effective = effective
	}
	return result
}

func unknownKeyDiagnostics(path string, metadata toml.MetaData) []Diagnostic {
	var diagnostics []Diagnostic
	seen := make(map[string]struct{})
	for _, key := range metadata.Undecoded() {
		name := key.String()
		if _, known := knownKeys[name]; known {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		diagnostics = append(diagnostics, Diagnostic{
			Path: path, Line: findKeyLine(path, name), Key: name, Message: "unknown configuration key",
		})
	}
	return diagnostics
}

func validateDefinedValues(path string, metadata toml.MetaData, cfg *Config) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(key, message string) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path, Line: findKeyLine(path, key), Key: key, Message: message,
		})
	}
	if metadata.IsDefined("stale_threshold_days") && cfg.StaleThresholdDays <= 0 {
		add("stale_threshold_days", "must be a positive integer")
	}
	if metadata.IsDefined("default_remote") && strings.TrimSpace(cfg.DefaultRemote) == "" {
		add("default_remote", "must not be empty")
	}
	if metadata.IsDefined("default_base") && strings.TrimSpace(cfg.DefaultBase) == "" {
		add("default_base", "must not be empty")
	}
	if metadata.IsDefined("ticket_pattern") && cfg.TicketPattern != "" {
		if _, err := regexp.Compile(cfg.TicketPattern); err != nil {
			add("ticket_pattern", fmt.Sprintf("must be a valid Go regular expression: %v", err))
		}
	}
	return diagnostics
}

func decodeDiagnostic(path string, err error) Diagnostic {
	diagnostic := Diagnostic{Path: path, Message: err.Error()}
	var parseErr toml.ParseError
	if errors.As(err, &parseErr) {
		diagnostic.Line = parseErr.Position.Line
		diagnostic.Key = parseErr.LastKey
		if parseErr.Message != "" {
			diagnostic.Message = parseErr.Message
			return diagnostic
		}
	}
	if matches := tomlLinePattern.FindStringSubmatch(err.Error()); matches != nil {
		diagnostic.Line, _ = strconv.Atoi(matches[1])
		if diagnostic.Key == "" {
			diagnostic.Key = matches[2]
		}
		diagnostic.Message = matches[3]
	}
	return diagnostic
}

func findKeyLine(path, key string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, key) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, key))
		if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "]") {
			return lineNumber
		}
	}
	return 0
}
