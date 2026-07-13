package module

import (
	"fmt"
	"strings"

	modlib "github.com/mart337i/odooctl/internal/module"
	"github.com/spf13/cobra"
)

var flagManifestJSON bool

var manifestCmd = &cobra.Command{
	Use:   "manifest <module>",
	Short: "Inspect a module manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runManifest,
}

func init() {
	manifestCmd.Flags().BoolVar(&flagManifestJSON, "json", false, "Print JSON output")
}

func runManifest(cmd *cobra.Command, args []string) error {
	dirs, _, err := moduleScanDirs()
	if err != nil {
		return err
	}
	moduleDir, ok := findModuleDir(args[0], dirs)
	if !ok {
		return fmt.Errorf("module %q not found", args[0])
	}
	manifest, err := modlib.ParseManifest(moduleDir)
	if err != nil {
		return err
	}
	if flagManifestJSON {
		return printJSON(manifest)
	}
	fmt.Printf("Module:      %s\n", manifest.Module)
	fmt.Printf("Name:        %s\n", manifest.Name)
	fmt.Printf("Version:     %s\n", manifest.Version)
	fmt.Printf("Depends:     %s\n", strings.Join(manifest.Depends, ", "))
	fmt.Printf("Python deps: %s\n", strings.Join(manifest.ExternalPython, ", "))
	fmt.Printf("Installable: %v\n", manifest.Installable)
	fmt.Printf("Application: %v\n", manifest.Application)
	fmt.Printf("Path:        %s\n", manifest.Path)
	return nil
}
