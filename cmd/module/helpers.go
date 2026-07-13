package module

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mart337i/odooctl/internal/config"
	modlib "github.com/mart337i/odooctl/internal/module"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/mart337i/odooctl/internal/project"
)

func loadModuleState() (*config.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, fmt.Errorf("no Docker environment found. Run 'odooctl docker create' first")
	}
	return state, nil
}

func moduleScanDirs() ([]string, *config.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	state, err := config.LoadFromDir(cwd)
	if err == nil {
		dirs := []string{state.ProjectRoot}
		dirs = append(dirs, state.AddonsPaths...)
		return dirs, state, nil
	}
	ctx := project.Detect(cwd)
	return []string{ctx.Root}, nil, nil
}

func findModuleDir(name string, dirs []string) (string, bool) {
	for _, dir := range dirs {
		candidate := filepath.Join(dir, name)
		if modlib.IsModule(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func collectManifests(dirs []string, targets []string) ([]modlib.ManifestInfo, error) {
	targetSet := make(map[string]bool)
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target != "" {
			targetSet[target] = true
		}
	}

	manifests := []modlib.ManifestInfo{}
	seen := make(map[string]bool)
	for _, dir := range dirs {
		modules, err := modlib.FindModules(dir)
		if err != nil {
			continue
		}
		for _, name := range modules {
			if len(targetSet) > 0 && !targetSet[name] {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			manifest, err := modlib.ParseManifest(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, manifest)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Module < manifests[j].Module })
	return manifests, nil
}

func printJSON(value any) error {
	return output.PrintJSON(value)
}
