package docker

import (
	"fmt"

	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/repository"
	"github.com/mart337i/odooctl/internal/templates"
)

// ensureSourceCommits upgrades older environments that predate source locks
// and keeps their generated files consistent with the persisted state.
func ensureSourceCommits(state *config.State) (bool, error) {
	changed := false
	if state.OdooCommit == "" {
		commit, err := repository.ResolveBranchCommit(repository.OdooRepository, state.OdooVersion, repository.Credentials{})
		if err != nil {
			return false, fmt.Errorf("failed to resolve Odoo %s source revision: %w", state.OdooVersion, err)
		}
		state.OdooCommit = commit
		changed = true
	}
	if state.Enterprise && state.EnterpriseCommit == "" {
		commit, err := repository.ResolveBranchCommit(
			repository.EnterpriseRepository,
			state.OdooVersion,
			repository.Credentials{Token: state.EnterpriseGitHubToken, SSHKeyPath: state.EnterpriseSSHKeyPath},
		)
		if err != nil {
			return false, fmt.Errorf("failed to resolve Enterprise %s source revision: %w", state.OdooVersion, err)
		}
		state.EnterpriseCommit = commit
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := templates.Render(state); err != nil {
		return false, fmt.Errorf("failed to render locked source configuration: %w", err)
	}
	if err := state.Save(); err != nil {
		return false, fmt.Errorf("failed to save source lock: %w", err)
	}
	if err := config.SaveProjectLink(state); err != nil {
		return false, fmt.Errorf("failed to save project link: %w", err)
	}
	return true, nil
}
