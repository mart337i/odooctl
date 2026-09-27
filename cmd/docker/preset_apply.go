package docker

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/presets"
	"github.com/mart337i/odooctl/internal/repository"
)

func applyPresetExpansion(state *config.State, expansion presets.Expansion) ([]string, bool) {
	changed := false
	var addedPip []string

	state.Presets, changed = mergeStrings(state.Presets, expansion.Presets, changed)
	state.Modules, changed = mergeStrings(state.Modules, expansion.Modules, changed)
	state.ServerWideModules, changed = mergeStrings(state.ServerWideModules, expansion.ServerWideModules, changed)

	for _, pkg := range expansion.PipPackages {
		if !stringInSlice(state.PipPackages, pkg) {
			state.PipPackages = append(state.PipPackages, pkg)
			addedPip = append(addedPip, pkg)
			changed = true
		}
	}

	if len(expansion.OdooConfig) > 0 && state.OdooConfig == nil {
		state.OdooConfig = map[string]string{}
	}
	for key, value := range expansion.OdooConfig {
		if state.OdooConfig[key] != value {
			state.OdooConfig[key] = value
			changed = true
		}
	}

	if len(expansion.Environment) > 0 && state.Environment == nil {
		state.Environment = map[string]string{}
	}
	for key, value := range expansion.Environment {
		if state.Environment[key] != value {
			state.Environment[key] = value
			changed = true
		}
	}

	for _, repo := range expansion.Repositories {
		var repoChanged bool
		state.Repositories, repoChanged = mergeManagedRepository(state.Repositories, repo)
		if repoChanged {
			changed = true
		}
	}

	return addedPip, changed
}

func cloneState(state *config.State) config.State {
	clone := *state
	clone.Modules = append([]string{}, state.Modules...)
	clone.PipPackages = append([]string{}, state.PipPackages...)
	clone.Presets = append([]string{}, state.Presets...)
	clone.Repositories = append([]config.ManagedRepository{}, state.Repositories...)
	clone.ServerWideModules = append([]string{}, state.ServerWideModules...)
	clone.AddonsPaths = append([]string{}, state.AddonsPaths...)
	if state.OdooConfig != nil {
		clone.OdooConfig = make(map[string]string, len(state.OdooConfig))
		for key, value := range state.OdooConfig {
			clone.OdooConfig[key] = value
		}
	}
	if state.Environment != nil {
		clone.Environment = make(map[string]string, len(state.Environment))
		for key, value := range state.Environment {
			clone.Environment[key] = value
		}
	}
	return clone
}

func ensureManagedRepositories(state *config.State) (bool, error) {
	if len(state.Repositories) == 0 {
		return false, nil
	}
	changed := false
	for i := range state.Repositories {
		repo := &state.Repositories[i]
		if strings.TrimSpace(repo.Name) == "" {
			return false, fmt.Errorf("managed repository name cannot be empty")
		}
		if repo.Commit == "" {
			commit, err := repository.ResolveBranchCommit(repo.URL, repo.Branch, repository.Credentials{})
			if err != nil {
				return false, fmt.Errorf("failed to resolve %s %s: %w", repo.URL, repo.Branch, err)
			}
			repo.Commit = commit
			changed = true
		}
		destination, err := config.ManagedRepositoryDir(state.ProjectName, state.Branch, repo.Name)
		if err != nil {
			return false, err
		}
		if err := repository.EnsureCheckout(destination, repo.URL, repo.Branch, repo.Commit); err != nil {
			return false, fmt.Errorf("failed to prepare repository %s: %w", repo.Name, err)
		}
	}
	return changed, nil
}

func mergeStrings(existing, additions []string, changed bool) ([]string, bool) {
	for _, value := range additions {
		if !stringInSlice(existing, value) {
			existing = append(existing, value)
			changed = true
		}
	}
	return existing, changed
}

func stringInSlice(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func mergeManagedRepository(existing []config.ManagedRepository, addition config.ManagedRepository) ([]config.ManagedRepository, bool) {
	for i, repo := range existing {
		if repo.Name != addition.Name {
			continue
		}
		updated := repo
		if repo.URL != addition.URL || repo.Branch != addition.Branch {
			updated.URL = addition.URL
			updated.Branch = addition.Branch
			updated.Commit = ""
		}
		updated.AddonsPath = addition.AddonsPath
		if updated != repo {
			existing[i] = updated
			return existing, true
		}
		return existing, false
	}
	return append(existing, addition), true
}
