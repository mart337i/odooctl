package repository

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

func EnsureCheckout(destination, repositoryURL, branch, commit string) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("repository destination cannot be empty")
	}
	if strings.TrimSpace(repositoryURL) == "" {
		return fmt.Errorf("repository URL cannot be empty")
	}
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("repository branch cannot be empty")
	}
	if strings.TrimSpace(commit) == "" {
		return fmt.Errorf("repository commit cannot be empty")
	}

	if ready, err := checkoutAtCommit(destination, commit); err != nil {
		return err
	} else if ready {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); os.IsNotExist(err) {
		if _, statErr := os.Stat(destination); statErr == nil {
			entries, readErr := os.ReadDir(destination)
			if readErr != nil {
				return readErr
			}
			if len(entries) > 0 {
				return fmt.Errorf("repository destination exists and is not a git checkout: %s", destination)
			}
		}
		if err := runGit("clone", "--branch", branch, "--single-branch", repositoryURL, destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := runGit("-C", destination, "fetch", "--depth=1", "origin", commit); err != nil {
		return err
	}
	if err := runGit("-C", destination, "checkout", "--detach", commit); err != nil {
		return err
	}
	return nil
}

func checkoutAtCommit(destination, commit string) (bool, error) {
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cmd := exec.Command("git", "-C", destination, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("inspect repository checkout %s: %w", destination, err)
	}
	return strings.TrimSpace(string(output)) == commit, nil
}

func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
