package browser

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var snapshotFlags = defaultBrowserFlags()

var snapshotCmd = &cobra.Command{
	Use:          "snapshot [path-or-url]",
	Short:        "Print a compact visible-text snapshot of a page",
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
		report, err := runPage(state, target, snapshotFlags, "", "")
		if err != nil {
			return err
		}
		if snapshotFlags.JSON {
			return output.PrintJSON(report)
		}
		fmt.Printf("# %s\n\n", report.Title)
		fmt.Println(strings.Join(report.VisibleText, "\n"))
		return nil
	},
}

func init() {
	addPageFlags(snapshotCmd, &snapshotFlags)
}
