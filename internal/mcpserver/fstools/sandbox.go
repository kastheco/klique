// Package fstools provides filesystem tool handlers for the kasmos MCP server.
// It enforces directory sandboxing so tools cannot access paths outside the
// workspace root or configured allowed directories.
package fstools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox restricts filesystem access to a set of allowed directories.
type Sandbox struct {
	allowedDirs []string
}

// NewSandbox creates a Sandbox that permits access only to the given directories
// and their descendants. Each directory is resolved to an absolute, symlink-free
// path so that symlink-escape attacks are caught at validation time.
func NewSandbox(dirs []string) *Sandbox {
	cleaned := make([]string, 0, len(dirs))
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			abs = filepath.Clean(d)
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			cleaned = append(cleaned, resolved)
		} else {
			cleaned = append(cleaned, abs)
		}
	}
	return &Sandbox{allowedDirs: cleaned}
}

// Validate resolves path to an absolute, symlink-free path and checks that it
// falls within one of the sandbox's allowed directories. It returns the resolved
// path on success, or an error if the path is outside all allowed directories.
func (s *Sandbox) Validate(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	abs = filepath.Clean(abs)

	resolved := abs
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = r
	}
	resolved = filepath.Clean(resolved)

	for _, allowed := range s.allowedDirs {
		if resolved == allowed || strings.HasPrefix(resolved, allowed+string(os.PathSeparator)) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed directories", path)
}

// DefaultDir returns the first allowed directory, or "." when no directories
// were configured.
func (s *Sandbox) DefaultDir() string {
	if len(s.allowedDirs) == 0 {
		return "."
	}
	return s.allowedDirs[0]
}
