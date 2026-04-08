package cmd

import (
	"fmt"
	"os/exec"
)

func commandWithPathArg(commandLine, path string) (*exec.Cmd, error) {
	parts, err := splitCommandLine(commandLine)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("no editor found; set $EDITOR or use --editor")
	}
	return exec.Command(parts[0], append(parts[1:], path)...), nil
}

func splitCommandLine(s string) ([]string, error) {
	var (
		parts   []string
		current []rune
		quote   rune
		escape  bool
	)

	flush := func() {
		if len(current) > 0 {
			parts = append(parts, string(current))
			current = nil
		}
	}

	for _, r := range s {
		switch {
		case escape:
			current = append(current, r)
			escape = false
		case r == '\\':
			escape = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current = append(current, r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			current = append(current, r)
		}
	}

	if escape {
		current = append(current, '\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command: %q", s)
	}
	flush()
	return parts, nil
}
