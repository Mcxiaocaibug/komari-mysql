package filepathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

func RelativeToBase(baseDir, name string) (string, error) {
	canonicalBase, err := Canonical(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve JavaScript BaseDir: %w", err)
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", fmt.Errorf("resolve JavaScript path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if !WithinBase(canonicalBase, absolute) {
		if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil && resolved != absolute {
			return "", fmt.Errorf("JavaScript path symlink escapes BaseDir: %s", name)
		}
		return "", fmt.Errorf("JavaScript path escapes BaseDir: %s", name)
	}
	relative, err := filepath.Rel(canonicalBase, absolute)
	if err != nil {
		return "", fmt.Errorf("make JavaScript path relative to BaseDir: %w", err)
	}
	if relative == "." {
		return ".", nil
	}
	return relative, nil
}

// Canonical normalizes a path's existing symlink prefix while preserving a
// missing final component. This avoids macOS /var versus /private/var aliases
// being treated as different roots.
func Canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	candidate := absolute
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return absolute, nil
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}

func WithinBase(baseDir, path string) bool {
	canonicalBase, err := Canonical(baseDir)
	if err != nil {
		return false
	}
	canonicalPath, err := Canonical(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(canonicalBase, canonicalPath)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
