package browser

import (
	"fmt"

	"github.com/fatih/color"
	internalbrowser "github.com/mart337i/odooctl/internal/browser"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagDoctorJSON bool

var doctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "Check Playwright Chromium browser runtime",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := loadState()
		if err != nil {
			return err
		}
		check := internalbrowser.CheckRuntime(state)
		if flagDoctorJSON {
			return output.PrintJSON(check)
		}
		if check.CanLaunch {
			fmt.Printf("%s Browser runtime ready\n", color.GreenString("✓"))
			fmt.Printf("Playwright: %s\n", check.PlaywrightVersion)
			fmt.Printf("Chromium:   %s\n", check.ChromiumPath)
			return nil
		}
		fmt.Printf("%s Browser runtime is not ready\n", color.YellowString("!"))
		if check.Error != "" {
			fmt.Println(check.Error)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&flagDoctorJSON, "json", false, "Print JSON output")
}
