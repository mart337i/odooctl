package git

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/odoo"
)

// Info contains git repository information
type Info struct {
	IsRepo   bool
	RepoName string
	Branch   string
	Root     string
}

// Detect checks if the directory is a git repository
func Detect(dir string) Info {
	info := Info{IsRepo: false}

	// Check if we're in a git repo
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return info
	}

	info.IsRepo = true
	info.Root = strings.TrimSpace(string(output))
	info.RepoName = filepath.Base(info.Root)

	// Get current branch
	cmd = exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	output, err = cmd.Output()
	if err == nil {
		info.Branch = strings.TrimSpace(string(output))
	}

	return info
}

// VersionFromBranch extracts Odoo version from branch name
// e.g., "17.0" -> "17.0", "17.0-feature" -> "17.0"
func VersionFromBranch(branch string) string {
	for _, v := range odoo.OdooVersions {
		if strings.HasPrefix(branch, v) {
			return v
		}
	}
	return ""
}
