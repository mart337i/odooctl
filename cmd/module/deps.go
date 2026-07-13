package module

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var flagDepsJSON bool

var depsCmd = &cobra.Command{
	Use:   "deps [modules...]",
	Short: "Show module manifest dependencies",
	Args:  cobra.ArbitraryArgs,
	RunE:  runDeps,
}

func init() {
	depsCmd.Flags().BoolVar(&flagDepsJSON, "json", false, "Print JSON output")
}

func runDeps(cmd *cobra.Command, args []string) error {
	dirs, _, err := moduleScanDirs()
	if err != nil {
		return err
	}
	manifests, err := collectManifests(dirs, args)
	if err != nil {
		return err
	}
	if flagDepsJSON {
		return printJSON(manifests)
	}
	if len(manifests) == 0 {
		fmt.Println("No matching Odoo modules found")
		return nil
	}
	for _, manifest := range manifests {
		fmt.Println(manifest.Module)
		fmt.Printf("  depends: %s\n", joinOrDash(manifest.Depends))
		fmt.Printf("  python:  %s\n", joinOrDash(manifest.ExternalPython))
	}
	return nil
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}
