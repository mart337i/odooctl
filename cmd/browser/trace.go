package browser

import (
	"fmt"
	"path/filepath"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var traceFlags = defaultBrowserFlags()

var traceCmd = &cobra.Command{
	Use:          "trace [path-or-url]",
	Short:        "Record a Playwright trace for a page load",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := loadState()
		if err != nil {
			return err
		}
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		localPath, containerPath, err := artifactPaths(state, traceFlags.Output, "trace", ".zip")
		if err != nil {
			return err
		}
		report, err := runPage(state, target, traceFlags, "", containerPath)
		if err != nil {
			return err
		}
		if err := copyArtifactIfNeeded(state, localPath, containerPath); err != nil {
			return err
		}
		report.Trace = filepath.Clean(localPath)
		if traceFlags.JSON {
			return output.PrintJSON(report)
		}
		fmt.Printf("Trace: %s\n", report.Trace)
		return nil
	},
}

func init() {
	addPageFlags(traceCmd, &traceFlags)
	traceCmd.Flags().StringVarP(&traceFlags.Output, "output", "o", "", "Output trace ZIP path")
}
