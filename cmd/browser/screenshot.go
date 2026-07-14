package browser

import (
	"fmt"
	"path/filepath"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var screenshotFlags = defaultBrowserFlags()

var screenshotCmd = &cobra.Command{
	Use:          "screenshot [path-or-url]",
	Short:        "Capture a page screenshot with Playwright Chromium",
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
		localPath, containerPath, err := artifactPaths(state, screenshotFlags.Output, "screenshot", ".png")
		if err != nil {
			return err
		}
		report, err := runPage(state, target, screenshotFlags, containerPath, "")
		if err != nil {
			return err
		}
		if err := copyArtifactIfNeeded(state, localPath, containerPath); err != nil {
			return err
		}
		report.Screenshot = filepath.Clean(localPath)
		if screenshotFlags.JSON {
			return output.PrintJSON(report)
		}
		fmt.Printf("Screenshot: %s\n", report.Screenshot)
		return nil
	},
}

func init() {
	addPageFlags(screenshotCmd, &screenshotFlags)
	screenshotCmd.Flags().StringVarP(&screenshotFlags.Output, "output", "o", "", "Output PNG path")
}
