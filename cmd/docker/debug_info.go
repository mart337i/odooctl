package docker

import (
	"fmt"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagDebugInfoJSON bool

type debugInfoReport struct {
	Project       string `json:"project"`
	OdooVersion   string `json:"odoo_version"`
	Database      string `json:"database"`
	OdooURL       string `json:"odoo_url"`
	MailHogURL    string `json:"mailhog_url"`
	DebugEndpoint string `json:"debug_endpoint"`
	EnvDir        string `json:"env_dir"`
	OdooConfig    string `json:"odoo_config"`
	VSCodeAttach  string `json:"vscode_attach"`
}

var debugInfoCmd = &cobra.Command{
	Use:          "debug-info",
	Short:        "Show URLs, database name, config paths, and debugger attach info",
	SilenceUsage: true,
	RunE:         runDebugInfo,
}

func init() {
	debugInfoCmd.Flags().BoolVar(&flagDebugInfoJSON, "json", false, "Print JSON output")
}

func runDebugInfo(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	report, err := buildDebugInfoReport(state)
	if err != nil {
		return err
	}
	if flagDebugInfoJSON {
		return output.PrintJSON(report)
	}
	cyan := color.New(color.FgCyan).SprintFunc()
	fmt.Printf("Project:  %s\n", cyan(report.Project))
	fmt.Printf("Odoo:     %s\n", cyan(report.OdooVersion))
	fmt.Printf("Database: %s\n", cyan(report.Database))
	fmt.Printf("Odoo URL: %s\n", cyan(report.OdooURL))
	fmt.Printf("MailHog:  %s\n", cyan(report.MailHogURL))
	fmt.Printf("Debugpy:  %s\n", cyan(report.DebugEndpoint))
	fmt.Printf("Env dir:  %s\n", report.EnvDir)
	fmt.Printf("Config:   %s\n\n", report.OdooConfig)
	fmt.Println("VS Code attach config:")
	fmt.Println(report.VSCodeAttach)
	return nil
}

func buildDebugInfoReport(state *config.State) (debugInfoReport, error) {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return debugInfoReport{}, err
	}
	debugEndpoint := fmt.Sprintf("localhost:%d", state.Ports.Debug)
	return debugInfoReport{
		Project:       state.ProjectName,
		OdooVersion:   state.OdooVersion,
		Database:      state.DBName(),
		OdooURL:       fmt.Sprintf("http://localhost:%d", state.Ports.Odoo),
		MailHogURL:    fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog),
		DebugEndpoint: debugEndpoint,
		EnvDir:        dir,
		OdooConfig:    filepath.Join(dir, "odoo.conf"),
		VSCodeAttach:  vscodeAttachConfig("localhost", state.Ports.Debug),
	}, nil
}

func vscodeAttachConfig(host string, port int) string {
	return fmt.Sprintf(`{
  "name": "Attach to odooctl Odoo",
  "type": "python",
  "request": "attach",
  "connect": { "host": %q, "port": %d },
  "pathMappings": [
    { "localRoot": "${workspaceFolder}", "remoteRoot": "/mnt/extra-addons" }
  ]
}`, host, port)
}
