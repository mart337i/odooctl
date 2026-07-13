package docker

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagOpenJSON bool

type openReport struct {
	Target string `json:"target"`
	URL    string `json:"url"`
	Opened bool   `json:"opened"`
	Error  string `json:"error,omitempty"`
}

var openCmd = &cobra.Command{
	Use:          "open [odoo|mailhog|debug]",
	Short:        "Open or print useful development URLs",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runOpen,
}

func init() {
	openCmd.Flags().BoolVar(&flagOpenJSON, "json", false, "Print JSON output")
}

func runOpen(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	target := "odoo"
	if len(args) > 0 {
		target = args[0]
	}
	url, err := targetURL(state, target)
	if err != nil {
		return err
	}
	if flagOpenJSON {
		return output.PrintJSON(openReport{Target: target, URL: url})
	}
	openErr := openURL(url)
	if openErr != nil {
		fmt.Printf("%s Could not open browser: %v\n", color.YellowString("!"), openErr)
	}
	fmt.Println(url)
	return nil
}

func targetURL(state *config.State, target string) (string, error) {
	switch target {
	case "odoo", "web", "":
		return fmt.Sprintf("http://localhost:%d", state.Ports.Odoo), nil
	case "mailhog", "mail":
		return fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog), nil
	case "debug", "debugpy":
		return fmt.Sprintf("localhost:%d", state.Ports.Debug), nil
	default:
		return "", fmt.Errorf("unknown target %q (supported: odoo, mailhog, debug)", target)
	}
}

func openURL(url string) error {
	if len(url) < 4 || url[:4] != "http" {
		return fmt.Errorf("not a browser URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
