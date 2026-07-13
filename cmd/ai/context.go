package ai

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/diagnostics"
	modlib "github.com/mart337i/odooctl/internal/module"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

type ContextReport struct {
	GeneratedAt  time.Time                    `json:"generated_at"`
	OK           bool                         `json:"ok"`
	Status       diagnostics.CheckStatus      `json:"status"`
	Project      *diagnostics.ProjectInfo     `json:"project,omitempty"`
	Environment  *diagnostics.EnvironmentInfo `json:"environment,omitempty"`
	Docker       diagnostics.DockerInfo       `json:"docker"`
	Modules      []modlib.ManifestInfo        `json:"modules,omitempty"`
	Module       *modlib.ManifestInfo         `json:"module,omitempty"`
	PythonDeps   *diagnostics.PythonDepsInfo  `json:"python_deps,omitempty"`
	Checks       []diagnostics.Check          `json:"checks"`
	Problems     []string                     `json:"problems,omitempty"`
	NextSteps    []string                     `json:"next_steps,omitempty"`
	SafeCommands []string                     `json:"safe_commands"`
}

var (
	flagContextModule string
	flagContextFormat string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Print compact AI-ready project context",
	Long:  `Prints redacted project, Docker, module, and dependency context for use with AI tools.`,
	RunE:  runContext,
}

func init() {
	contextCmd.Flags().StringVarP(&flagContextModule, "module", "m", "", "Focus context on one module")
	contextCmd.Flags().StringVar(&flagContextFormat, "format", "markdown", "Output format: markdown or json")
}

func runContext(cmd *cobra.Command, args []string) error {
	report, err := buildContextReport(flagContextModule)
	if err != nil {
		return err
	}
	switch flagContextFormat {
	case "json":
		return output.PrintJSON(report)
	case "markdown", "md", "":
		printContextMarkdown(report)
		return nil
	default:
		return fmt.Errorf("unsupported --format %q (supported: markdown, json)", flagContextFormat)
	}
}

func buildContextReport(moduleName string) (ContextReport, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ContextReport{}, err
	}
	doctor := diagnostics.Collect(cwd)
	report := ContextReport{
		GeneratedAt:  doctor.GeneratedAt,
		OK:           doctor.OK,
		Status:       doctor.Status,
		Project:      doctor.Project,
		Environment:  doctor.Environment,
		Docker:       doctor.Docker,
		PythonDeps:   doctor.PythonDeps,
		Checks:       doctor.Checks,
		Problems:     doctor.Problems,
		NextSteps:    doctor.NextSteps,
		SafeCommands: doctor.SafeCommands,
	}

	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return report, nil
	}
	targets := []string(nil)
	if moduleName != "" {
		targets = []string{moduleName}
	}
	manifests, err := diagnostics.FindModuleManifests(state, targets)
	if err != nil {
		return report, err
	}
	if moduleName != "" {
		if len(manifests) == 0 {
			return report, fmt.Errorf("module %q not found", moduleName)
		}
		report.Module = &manifests[0]
	} else {
		report.Modules = manifests
	}
	return report, nil
}

func printContextMarkdown(report ContextReport) {
	fmt.Println("# odooctl AI Context")
	fmt.Printf("Generated: %s\n\n", report.GeneratedAt.Format(time.RFC3339))

	if report.Project == nil {
		fmt.Println("## Project")
		fmt.Println("No odooctl environment was found for the current directory.")
		printList("Next Steps", report.NextSteps)
		return
	}

	fmt.Println("## Project")
	fmt.Printf("- Name: `%s`\n", report.Project.Name)
	fmt.Printf("- Odoo version: `%s`\n", report.Project.OdooVersion)
	fmt.Printf("- Branch/environment: `%s`\n", report.Project.Branch)
	fmt.Printf("- Database: `%s`\n", report.Project.Database)
	fmt.Printf("- Root: `%s`\n", report.Project.Root)
	if report.Environment != nil {
		fmt.Printf("- Env dir: `%s`\n", report.Environment.Dir)
		fmt.Printf("- Ports: Odoo `%d`, MailHog `%d`, Debug `%d`\n", report.Environment.Ports.Odoo, report.Environment.Ports.Mailhog, report.Environment.Ports.Debug)
	}

	fmt.Println("\n## Docker")
	fmt.Printf("- Context: `%s`\n", valueOrUnknown(report.Docker.Context))
	fmt.Printf("- Daemon OK: `%t`\n", report.Docker.DaemonOK)
	fmt.Printf("- Bind mount OK: `%t`\n", report.Docker.BindMountOK)
	if len(report.Docker.Services) > 0 {
		fmt.Println("- Services:")
		for _, svc := range report.Docker.Services {
			fmt.Printf("  - `%s`: `%s` (%s)\n", svc.Name, svc.State, valueOrDash(svc.Status))
		}
	}
	if report.Docker.OdooURL != "" {
		fmt.Printf("- Odoo URL: `%s`\n", report.Docker.OdooURL)
	}

	if report.Module != nil {
		printModuleMarkdown("Module", *report.Module)
	} else if len(report.Modules) > 0 {
		fmt.Println("\n## Modules")
		for _, manifest := range report.Modules {
			fmt.Printf("- `%s`: %s\n", manifest.Module, valueOrDash(manifest.Name))
		}
	}

	if report.PythonDeps != nil {
		fmt.Println("\n## Python Dependencies")
		fmt.Printf("- Configured: `%s`\n", joinOrDash(report.PythonDeps.Configured))
		fmt.Printf("- Missing: `%s`\n", joinOrDash(report.PythonDeps.Missing))
		fmt.Printf("- Synced: `%t`\n", report.PythonDeps.Synced)
	}

	printList("Problems", report.Problems)
	printList("Next Steps", report.NextSteps)
	printList("Safe Commands", report.SafeCommands)
}

func printModuleMarkdown(title string, manifest modlib.ManifestInfo) {
	fmt.Printf("\n## %s\n", title)
	fmt.Printf("- Module: `%s`\n", manifest.Module)
	fmt.Printf("- Name: `%s`\n", valueOrDash(manifest.Name))
	fmt.Printf("- Version: `%s`\n", valueOrDash(manifest.Version))
	fmt.Printf("- Path: `%s`\n", manifest.Path)
	fmt.Printf("- Depends: `%s`\n", joinOrDash(manifest.Depends))
	fmt.Printf("- Python deps: `%s`\n", joinOrDash(manifest.ExternalPython))
	fmt.Printf("- Installable: `%t`\n", manifest.Installable)
}

func printList(title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("\n## %s\n", title)
	for _, value := range uniqueStrings(values) {
		fmt.Printf("- `%s`\n", value)
	}
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
