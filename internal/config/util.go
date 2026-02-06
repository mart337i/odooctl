package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ExpandPath expands ~ to the user's home directory
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

// MaskToken shows only the prefix and last 4 chars of a token
func MaskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	// Show prefix (ghp_ or github_pat_) + enough to identify, mask the rest
	prefixEnd := 4
	if strings.HasPrefix(token, "github_pat_") {
		prefixEnd = 11
	}
	visible := token[:prefixEnd]
	last4 := token[len(token)-4:]
	masked := len(token) - prefixEnd - 4
	return visible + strings.Repeat("*", masked) + last4
}

// SanitizeName sanitizes project and branch names for safe use in file paths and Docker resource names
// Replaces / with - and removes any characters that aren't alphanumeric, hyphen, underscore, or dot
func SanitizeName(name string) string {
	// Replace / with - (common convention for branch names like feature/my-feature)
	name = strings.ReplaceAll(name, "/", "-")

	// Remove any characters that aren't alphanumeric, hyphen, underscore, or dot
	re := regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
	name = re.ReplaceAllString(name, "")

	return name
}
