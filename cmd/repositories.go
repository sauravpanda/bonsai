package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func defaultScanRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, "Github"),
		filepath.Join(home, "GitHub"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Developer"),
		filepath.Join(home, "Code"),
		filepath.Join(home, "src"),
	}
	var roots []string
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.IsDir() && !containsDirectory(roots, info) {
			if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
				candidate = resolved
			}
			roots = append(roots, filepath.Clean(candidate))
		}
	}
	return roots
}

func normalizeScanRoots(requested []string) ([]string, error) {
	if len(requested) == 0 {
		roots := defaultScanRoots()
		if len(roots) == 0 {
			return nil, fmt.Errorf("no default development roots found; pass --root <directory>")
		}
		return roots, nil
	}

	home, _ := os.UserHomeDir()
	roots := make([]string, 0, len(requested))
	for _, value := range requested {
		value = strings.TrimSpace(value)
		if value == "~" {
			value = home
		} else if strings.HasPrefix(value, "~/") {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("resolve scan root %q: %w", value, err)
		}
		absolute = filepath.Clean(absolute)
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("scan root %s: %w", absolute, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("scan root is not a directory: %s", absolute)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
			absolute = resolved
		}
		if !containsDirectory(roots, info) {
			roots = append(roots, filepath.Clean(absolute))
		}
	}
	return roots, nil
}

func containsDirectory(paths []string, candidate os.FileInfo) bool {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && os.SameFile(info, candidate) {
			return true
		}
	}
	return false
}

// discoverRepositories finds main git working directories beneath bounded
// scan roots. Once a repository or linked worktree is found, its contents are
// not traversed; git worktree list provides the authoritative linked paths.
func discoverRepositories(roots []string) ([]string, error) {
	repositories := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.IsDir() {
				return nil
			}

			gitPath := filepath.Join(path, ".git")
			if info, err := os.Lstat(gitPath); err == nil {
				if info.IsDir() {
					repositories[filepath.Clean(path)] = true
				}
				// A .git file identifies a linked worktree. Its main repository is
				// discovered separately and is the authoritative source.
				return filepath.SkipDir
			}

			if path != root && shouldSkipDiscoveryDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", root, err)
		}
	}

	result := make([]string, 0, len(repositories))
	for repository := range repositories {
		result = append(result, repository)
	}
	sort.Strings(result)
	return result, nil
}

func shouldSkipDiscoveryDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "target", "dist", "build", "__pycache__", "Library", "Applications", "Movies", "Music", "Pictures":
		return true
	default:
		return false
	}
}
