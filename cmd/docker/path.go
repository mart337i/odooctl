package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagPathJSON bool

type pathReport struct {
	Location     string       `json:"location"`
	Project      string       `json:"project"`
	Environment  string       `json:"environment"`
	OdooVersion  string       `json:"odoo_version"`
	Ports        config.Ports `json:"ports"`
	Enterprise   bool         `json:"enterprise"`
	FilesReady   bool         `json:"files_ready"`
	FilesPresent []string     `json:"files_present"`
	FilesMissing []string     `json:"files_missing"`
	AddonsPaths  []string     `json:"addons_paths"`
}

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show docker environment location and status",
	Long:  `Display the location and status of the Docker environment files.`,
	RunE:  runPath,
}

func init() {
	pathCmd.Flags().BoolVar(&flagPathJSON, "json", false, "Print JSON output")
}

func runPath(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return err
	}
	report := dockerPathReport(state, dir)
	if flagPathJSON {
		return output.PrintJSON(report)
	}

	fmt.Printf("%s Location: %s\n", cyan("📁"), dir)
	fmt.Printf("%s Version:  Odoo %s\n", cyan("🔢"), state.OdooVersion)
	fmt.Printf("%s Ports:    Odoo=%d, MailHog=%d, Debug=%d\n",
		cyan("🌐"), state.Ports.Odoo, state.Ports.Mailhog, state.Ports.Debug)

	if state.Enterprise {
		fmt.Printf("%s Edition:  Enterprise\n", cyan("🏢"))
	}

	if report.FilesReady {
		entries, _ := os.ReadDir(dir)
		fmt.Printf("\n%s %d files ready\n", green("✓"), len(entries))
	} else {
		fmt.Printf("\n%s Not fully initialized - run 'odooctl docker create'\n", yellow("⚠️"))
	}

	// Show addons paths if configured
	if len(state.AddonsPaths) > 0 {
		fmt.Printf("\n%s Addons paths:\n", cyan("📦"))
		for i, path := range state.AddonsPaths {
			fmt.Printf("   %d. %s\n", i+1, path)
		}
	}

	return nil
}

func dockerPathReport(state *config.State, dir string) pathReport {
	report := pathReport{
		Location:    dir,
		Project:     state.ProjectName,
		Environment: state.Branch,
		OdooVersion: state.OdooVersion,
		Ports:       state.Ports,
		Enterprise:  state.Enterprise,
		AddonsPaths: append([]string{}, state.AddonsPaths...),
	}
	for _, file := range []string{"docker-compose.yml", "Dockerfile", "odoo.conf"} {
		if _, err := os.Stat(filepath.Join(dir, file)); os.IsNotExist(err) {
			report.FilesMissing = append(report.FilesMissing, file)
		} else {
			report.FilesPresent = append(report.FilesPresent, file)
		}
	}
	report.FilesReady = len(report.FilesMissing) == 0
	return report
}
