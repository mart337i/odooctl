package module

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultExcludePatterns are patterns to exclude from hash calculation
var DefaultExcludePatterns = []string{
	"*.pyc", "*.pyo", "*.pyd",
	"__pycache__/*",
	"static/*",
	"tests/*",
	"i18n/*.pot",
	"i18n_extra/*.pot",
	".git/*",
}

// IsModule checks if a directory is an Odoo module
func IsModule(dir string) bool {
	manifest := filepath.Join(dir, "__manifest__.py")
	_, err := os.Stat(manifest)
	return err == nil
}

// FindModules finds all Odoo modules in a directory
func FindModules(root string) ([]string, error) {
	var modules []string

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if IsModule(filepath.Join(root, entry.Name())) {
			modules = append(modules, entry.Name())
		}
	}

	sort.Strings(modules)
	return modules, nil
}

// ExpandPatterns expands glob patterns to module names
func ExpandPatterns(patterns []string, available []string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		if strings.ContainsAny(pattern, "*?[") {
			// Glob pattern
			for _, mod := range available {
				matched, _ := filepath.Match(pattern, mod)
				if matched && !seen[mod] {
					result = append(result, mod)
					seen[mod] = true
				}
			}
		} else {
			// Exact match
			if !seen[pattern] {
				result = append(result, pattern)
				seen[pattern] = true
			}
		}
	}

	return result
}

// Hash calculates SHA256 hash of an Odoo module directory
func Hash(moduleDir string) (string, error) {
	hasher := sha256.New()

	var files []string
	err := filepath.Walk(moduleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(moduleDir, path)

		// Check exclusions
		if shouldExclude(relPath) {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort for consistent ordering
	sort.Strings(files)

	for _, path := range files {
		relPath, _ := filepath.Rel(moduleDir, path)

		// Hash the relative path
		hasher.Write([]byte(relPath))

		// Hash file contents
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}

		if _, err := io.Copy(hasher, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func shouldExclude(relPath string) bool {
	// Normalize path separators
	relPath = filepath.ToSlash(relPath)

	for _, pattern := range DefaultExcludePatterns {
		// Check if path matches pattern
		matched, _ := filepath.Match(pattern, relPath)
		if matched {
			return true
		}

		// Check if any parent directory matches
		parts := strings.Split(relPath, "/")
		for i := range parts {
			partial := strings.Join(parts[:i+1], "/")
			matched, _ = filepath.Match(pattern, partial)
			if matched {
				return true
			}
		}
	}

	return false
}
