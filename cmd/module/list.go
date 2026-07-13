package module

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var flagListJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Odoo modules in the current project",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&flagListJSON, "json", false, "Print JSON output")
}

func runList(cmd *cobra.Command, args []string) error {
	dirs, _, err := moduleScanDirs()
	if err != nil {
		return err
	}
	manifests, err := collectManifests(dirs, nil)
	if err != nil {
		return err
	}
	if flagListJSON {
		return printJSON(manifests)
	}
	if len(manifests) == 0 {
		fmt.Println("No Odoo modules found")
		return nil
	}
	fmt.Printf("%-32s %-28s %-12s %s\n", "MODULE", "NAME", "VERSION", "DEPENDS")
	fmt.Println(strings.Repeat("-", 92))
	for _, manifest := range manifests {
		fmt.Printf("%-32s %-28s %-12s %s\n", manifest.Module, trimForTable(manifest.Name, 28), manifest.Version, strings.Join(manifest.Depends, ","))
	}
	return nil
}

func trimForTable(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
