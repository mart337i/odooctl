package presets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mart337i/odooctl/internal/config"
)

type Definition struct {
	Name              string
	Description       string
	Repositories      []Repository
	PipPackages       []string
	Modules           []string
	ServerWideModules []string
	OdooConfig        map[string]string
	Environment       map[string]string
}

type Repository struct {
	Name       string
	URL        string
	Branch     string
	AddonsPath bool
}

type Expansion struct {
	Presets           []string
	Repositories      []config.ManagedRepository
	PipPackages       []string
	Modules           []string
	ServerWideModules []string
	OdooConfig        map[string]string
	Environment       map[string]string
}

var definitions = []Definition{
	{
		Name:        "queue-job",
		Description: "OCA queue_job addon with jobrunner-friendly Odoo config",
		Repositories: []Repository{
			{Name: "oca-queue", URL: "https://github.com/OCA/queue.git", Branch: "{version}", AddonsPath: true},
		},
		Modules:           []string{"queue_job"},
		ServerWideModules: []string{"queue_job"},
		OdooConfig: map[string]string{
			"workers":          "2",
			"max_cron_threads": "1",
		},
		Environment: map[string]string{
			"ODOO_QUEUE_JOB_CHANNELS": "root:4",
		},
	},
	{
		Name:        "migration",
		Description: "OCA OpenUpgrade addons plus Odoo upgrade-util checkout and openupgradelib",
		Repositories: []Repository{
			{Name: "oca-openupgrade", URL: "https://github.com/OCA/OpenUpgrade.git", Branch: "{version}", AddonsPath: true},
			{Name: "odoo-upgrade-util", URL: "https://github.com/odoo/upgrade-util.git", Branch: "master", AddonsPath: false},
		},
		PipPackages: []string{"openupgradelib"},
	},
}

func List() []Definition {
	items := append([]Definition{}, definitions...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func Names() []string {
	defs := List()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func Expand(names []string, version string) (Expansion, error) {
	expansion := Expansion{
		OdooConfig:  map[string]string{},
		Environment: map[string]string{},
	}
	defs := make(map[string]Definition, len(definitions))
	for _, def := range definitions {
		defs[def.Name] = def
	}

	for _, name := range CleanNames(names) {
		def, ok := defs[name]
		if !ok {
			return Expansion{}, fmt.Errorf("unknown preset %q (available: %s)", name, strings.Join(Names(), ", "))
		}
		expansion.Presets = appendUnique(expansion.Presets, def.Name)
		expansion.PipPackages = appendUnique(expansion.PipPackages, def.PipPackages...)
		expansion.Modules = appendUnique(expansion.Modules, def.Modules...)
		expansion.ServerWideModules = appendUnique(expansion.ServerWideModules, def.ServerWideModules...)
		for key, value := range def.OdooConfig {
			expansion.OdooConfig[key] = value
		}
		for key, value := range def.Environment {
			expansion.Environment[key] = value
		}
		for _, repo := range def.Repositories {
			expansion.Repositories = mergeRepository(expansion.Repositories, config.ManagedRepository{
				Name:       repo.Name,
				URL:        repo.URL,
				Branch:     strings.ReplaceAll(repo.Branch, "{version}", version),
				AddonsPath: repo.AddonsPath,
			})
		}
	}
	return expansion, nil
}

func CleanNames(values []string) []string {
	names := []string{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				names = appendUnique(names, part)
			}
		}
	}
	return names
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func mergeRepository(repositories []config.ManagedRepository, addition config.ManagedRepository) []config.ManagedRepository {
	for i, repo := range repositories {
		if repo.Name != addition.Name {
			continue
		}
		if repo.Commit == "" {
			repositories[i] = addition
		}
		return repositories
	}
	return append(repositories, addition)
}
