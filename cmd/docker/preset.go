package docker

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/mart337i/odooctl/internal/presets"
	"github.com/spf13/cobra"
)

var flagPresetJSON bool

type presetReport struct {
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Modules           []string                 `json:"modules,omitempty"`
	PipPackages       []string                 `json:"pip_packages,omitempty"`
	ServerWideModules []string                 `json:"server_wide_modules,omitempty"`
	OdooConfig        map[string]string        `json:"odoo_config,omitempty"`
	Environment       map[string]string        `json:"environment,omitempty"`
	Repositories      []presetRepositoryReport `json:"repositories,omitempty"`
}

type presetRepositoryReport struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Branch     string `json:"branch"`
	AddonsPath bool   `json:"addons_path"`
}

var presetCmd = &cobra.Command{
	Use:   "preset",
	Short: "List built-in environment presets",
}

var presetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List built-in environment presets",
	RunE:  runPresetList,
}

func init() {
	presetListCmd.Flags().BoolVar(&flagPresetJSON, "json", false, "Print JSON output")
	presetCmd.AddCommand(presetListCmd)
}

func runPresetList(cmd *cobra.Command, args []string) error {
	report := buildPresetReport()
	if flagPresetJSON {
		return output.PrintJSON(report)
	}
	cyan := color.New(color.FgCyan).SprintFunc()
	for _, preset := range report {
		fmt.Printf("%s %s\n", cyan(preset.Name), preset.Description)
		if len(preset.Modules) > 0 {
			fmt.Printf("  modules: %v\n", preset.Modules)
		}
		if len(preset.PipPackages) > 0 {
			fmt.Printf("  pip: %v\n", preset.PipPackages)
		}
		if len(preset.ServerWideModules) > 0 {
			fmt.Printf("  server-wide modules: %v\n", preset.ServerWideModules)
		}
		if len(preset.Repositories) > 0 {
			fmt.Println("  repositories:")
			for _, repo := range preset.Repositories {
				kind := "tooling"
				if repo.AddonsPath {
					kind = "addons"
				}
				fmt.Printf("    %s (%s, branch %s)\n", repo.URL, kind, repo.Branch)
			}
		}
	}
	return nil
}

func buildPresetReport() []presetReport {
	definitions := presets.List()
	report := make([]presetReport, 0, len(definitions))
	for _, definition := range definitions {
		item := presetReport{
			Name:              definition.Name,
			Description:       definition.Description,
			Modules:           append([]string{}, definition.Modules...),
			PipPackages:       append([]string{}, definition.PipPackages...),
			ServerWideModules: append([]string{}, definition.ServerWideModules...),
			OdooConfig:        cloneMap(definition.OdooConfig),
			Environment:       cloneMap(definition.Environment),
		}
		for _, repo := range definition.Repositories {
			item.Repositories = append(item.Repositories, presetRepositoryReport{
				Name:       repo.Name,
				URL:        repo.URL,
				Branch:     repo.Branch,
				AddonsPath: repo.AddonsPath,
			})
		}
		report = append(report, item)
	}
	return report
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
