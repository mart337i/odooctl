package repository

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

const (
	OdooRepository       = "https://github.com/odoo/odoo.git"
	EnterpriseRepository = "https://github.com/odoo/enterprise.git"
)

// Credentials describes the supported GitHub authentication methods.
type Credentials struct {
	Token      string
	SSHKeyPath string
}

// ResolveBranchCommit returns the commit currently pointed to by a branch.
// The caller can persist the result to make subsequent builds reproducible.
func ResolveBranchCommit(repositoryURL, branch string, credentials Credentials) (string, error) {
	if strings.TrimSpace(repositoryURL) == "" {
		return "", fmt.Errorf("repository URL cannot be empty")
	}
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("repository branch cannot be empty")
	}

	resolvedURL := repositoryURL
	if credentials.Token != "" {
		parsed, err := url.Parse(repositoryURL)
		if err != nil {
			return "", fmt.Errorf("parse repository URL: %w", err)
		}
		parsed.User = url.UserPassword("x-access-token", credentials.Token)
		resolvedURL = parsed.String()
	}

	cmd := exec.Command("git", "ls-remote", resolvedURL, "refs/heads/"+branch)
	if credentials.SSHKeyPath != "" {
		cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -o IdentitiesOnly=yes -i "+shellQuote(credentials.SSHKeyPath))
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve %s branch %q: %w", repositoryURL, branch, err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 1 || len(fields[0]) != 40 {
		return "", fmt.Errorf("repository %s has no branch %q", repositoryURL, branch)
	}
	return fields[0], nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
