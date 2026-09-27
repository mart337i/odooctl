package cmd

import (
	"fmt"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/mart337i/odooctl/internal/selfupdate"
	"github.com/spf13/cobra"
)

var (
	flagUpdateCheckOnly bool
	flagUpdateDryRun    bool
	flagUpdateJSON      bool
	flagUpdateForce     bool
	flagUpdateMethod    string
	flagUpdateSourceDir string
)

var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "Update odooctl",
	SilenceUsage: true,
	RunE:         runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&flagUpdateCheckOnly, "check", false, "Check for updates without installing")
	updateCmd.Flags().BoolVar(&flagUpdateDryRun, "dry-run", false, "Show update plan without installing")
	updateCmd.Flags().BoolVar(&flagUpdateJSON, "json", false, "Print JSON output")
	updateCmd.Flags().BoolVar(&flagUpdateForce, "force", false, "Allow updating dev or dirty builds")
	updateCmd.Flags().StringVar(&flagUpdateMethod, "method", "", "Update method: auto, apt, release, go, source")
	updateCmd.Flags().StringVar(&flagUpdateSourceDir, "source-dir", "", "Source checkout directory for --method source")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	plan, err := selfupdate.Plan(selfupdate.Options{
		CurrentVersion: version,
		MethodOverride: flagUpdateMethod,
		SourceDir:      flagUpdateSourceDir,
		Force:          flagUpdateForce,
	})
	if err != nil {
		return err
	}
	if flagUpdateJSON {
		return output.PrintJSON(plan)
	}
	fmt.Println(plan.Summary())
	if flagUpdateCheckOnly || flagUpdateDryRun || !plan.UpdateAvailable {
		return nil
	}
	return selfupdate.Execute(plan)
}
