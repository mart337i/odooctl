package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/odoo"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/mart337i/odooctl/internal/project"
	"github.com/mart337i/odooctl/internal/scaffold"
	"github.com/mart337i/odooctl/pkg/prompt"
	"github.com/spf13/cobra"
)

var (
	flagAuthor       string
	flagVersion      string
	flagDepends      string
	flagDescription  string
	flagWithModel    bool
	flagScaffoldJSON bool
)

type scaffoldReport struct {
	Module      string   `json:"module"`
	Location    string   `json:"location"`
	OdooVersion string   `json:"odoo_version"`
	Depends     []string `json:"depends"`
	WithModel   bool     `json:"with_model"`
	Model       string   `json:"model,omitempty"`
	NextSteps   []string `json:"next_steps"`
}

var scaffoldCmd = &cobra.Command{
	Use:   "scaffold <name>",
	Short: "Create a new Odoo module",
	Long: `Creates a new Odoo module with the standard directory structure.

Examples:
  odooctl module scaffold my_module
  odooctl module scaffold my_module --author "My Company"
  odooctl module scaffold my_module --depends sale,purchase --model`,
	Args: cobra.ExactArgs(1),
	RunE: runScaffold,
}

func init() {
	scaffoldCmd.Flags().StringVarP(&flagAuthor, "author", "a", "", "Module author")
	scaffoldCmd.Flags().StringVarP(&flagVersion, "odoo-version", "v", "", "Odoo version ("+odoo.VersionsString()+")")
	scaffoldCmd.Flags().StringVarP(&flagDepends, "depends", "d", "base", "Dependencies (comma-separated)")
	scaffoldCmd.Flags().StringVar(&flagDescription, "description", "", "Module description")
	scaffoldCmd.Flags().BoolVarP(&flagWithModel, "model", "m", false, "Include a model with the same name")
	scaffoldCmd.Flags().BoolVar(&flagScaffoldJSON, "json", false, "Print JSON output")
}

func runScaffold(cmd *cobra.Command, args []string) error {
	moduleName := args[0]

	// Validate module name
	if !isValidModuleName(moduleName) {
		return fmt.Errorf("invalid module name %q: use lowercase letters, numbers, and underscores", moduleName)
	}

	// Check if directory already exists
	if _, err := os.Stat(moduleName); err == nil {
		return fmt.Errorf("Module %q already exists", moduleName)
	}

	// Detect Odoo version from context
	odooVersion := flagVersion
	if odooVersion == "" {
		ctx := project.Detect(".")
		if ctx.OdooVersion != "" {
			odooVersion = ctx.OdooVersion
		} else {
			// Prompt for version
			var err error
			odooVersion, err = prompt.SelectVersion()
			if err != nil {
				return err
			}
		}
	}

	// Build module config
	depends := []string{"base"}
	if flagDepends != "" && flagDepends != "base" {
		depends = strings.Split(flagDepends, ",")
		for i := range depends {
			depends[i] = strings.TrimSpace(depends[i])
		}
	}

	config := scaffold.ModuleConfig{
		Name:        moduleName,
		Author:      flagAuthor,
		Version:     odooVersion,
		Depends:     depends,
		Description: flagDescription,
		WithModel:   flagWithModel,
	}

	// Set defaults
	if config.Author == "" {
		config.Author = "My Company"
	}
	if config.Description == "" {
		config.Description = fmt.Sprintf("%s module", toTitle(moduleName))
	}

	// Create module
	if err := scaffold.CreateModule(moduleName, config); err != nil {
		return fmt.Errorf("failed to create module: %w", err)
	}
	if flagScaffoldJSON {
		return output.PrintJSON(buildScaffoldReport(moduleName, odooVersion, depends, flagWithModel))
	}

	// Print summary
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Println()
	fmt.Printf("%s Module created: %s\n\n", green("✓"), cyan(moduleName))
	fmt.Printf("  Location:  %s\n", cyan(filepath.Join(".", moduleName)))
	fmt.Printf("  Version:   %s\n", cyan(odooVersion))
	fmt.Printf("  Depends:   %s\n", cyan(strings.Join(depends, ", ")))

	if flagWithModel {
		fmt.Printf("  Model:     %s\n", cyan(strings.ReplaceAll(moduleName, "_", ".")))
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit %s to customize the module\n", cyan(filepath.Join(moduleName, "__manifest__.py")))
	if flagWithModel {
		fmt.Printf("  2. Edit %s to add fields\n", cyan(filepath.Join(moduleName, "models", moduleName+".py")))
	}
	fmt.Println()

	return nil
}

func buildScaffoldReport(moduleName, odooVersion string, depends []string, withModel bool) scaffoldReport {
	report := scaffoldReport{
		Module:      moduleName,
		Location:    filepath.Join(".", moduleName),
		OdooVersion: odooVersion,
		Depends:     append([]string{}, depends...),
		WithModel:   withModel,
		NextSteps: []string{
			fmt.Sprintf("Edit %s", filepath.Join(moduleName, "__manifest__.py")),
			fmt.Sprintf("odooctl docker install %s", moduleName),
		},
	}
	if withModel {
		report.Model = strings.ReplaceAll(moduleName, "_", ".")
		report.NextSteps = append(report.NextSteps, fmt.Sprintf("Edit %s", filepath.Join(moduleName, "models", moduleName+".py")))
	}
	return report
}

func isValidModuleName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if c >= 'a' && c <= 'z' {
			continue
		}
		if c >= '0' && c <= '9' && i > 0 {
			continue
		}
		if c == '_' && i > 0 {
			continue
		}
		return false
	}
	return true
}

func toTitle(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
