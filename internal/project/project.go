package project

import (
	"os"
	"path/filepath"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/git"
)

// Context holds all project detection results
type Context struct {
	Name        string
	OdooVersion string
	Branch      string
	IsGitRepo   bool
	Root        string
}

// Detect analyzes the current directory
func Detect(dir string) Context {
	absDir, _ := filepath.Abs(dir)
	ctx := Context{
		Name:   config.SanitizeName(filepath.Base(absDir)),
		Root:   absDir,
		Branch: config.SanitizeName("main"),
	}

	// Check for git repo
	gitInfo := git.Detect(dir)
	if gitInfo.IsRepo {
		ctx.IsGitRepo = true
		ctx.Name = config.SanitizeName(gitInfo.RepoName)
		ctx.Branch = config.SanitizeName(gitInfo.Branch)
		ctx.Root = gitInfo.Root
		ctx.OdooVersion = git.VersionFromBranch(gitInfo.Branch)
	}

	// Check for .odooversion file
	if ctx.OdooVersion == "" {
		if data, err := os.ReadFile(filepath.Join(ctx.Root, ".odooversion")); err == nil {
			ctx.OdooVersion = string(data)
		}
	}

	// Check ODOO_VERSION env var
	if ctx.OdooVersion == "" {
		ctx.OdooVersion = os.Getenv("ODOO_VERSION")
	}

	return ctx
}
