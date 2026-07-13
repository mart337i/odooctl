package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/deps"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/templates"
	"github.com/mart337i/odooctl/pkg/prompt"
	"github.com/spf13/cobra"
)

var (
	flagReconfigAddPip       string
	flagReconfigAddPaths     []string
	flagReconfigAutoDiscover bool
	flagReconfigRebuild      bool
	flagReconfigStopFirst    bool
	flagReconfigNoCache      bool
)

var reconfigureCmd = &cobra.Command{
	Use:   "reconfigure",
	Short: "Add pip packages or addons paths to existing environment",
	Long: `Add pip packages or addons paths to an existing Docker environment
without having to recreate everything from scratch.

Examples:
  # Add pip packages
  odooctl docker reconfigure --add-pip requests,pandas

  # Add from requirements.txt
  odooctl docker reconfigure --add-pip ./requirements.txt

  # Add addons path
  odooctl docker reconfigure --add-addons-path ~/odoo-addons

  # Auto-discover dependencies
  odooctl docker reconfigure --auto-discover-deps

  # Combine options
  odooctl docker reconfigure --add-pip requests --add-addons-path ~/addons --rebuild`,
	RunE: runReconfigure,
}

func init() {
	reconfigureCmd.Flags().StringVar(&flagReconfigAddPip, "add-pip", "", "Add pip packages (comma-separated or path to requirements.txt)")
	reconfigureCmd.Flags().StringArrayVar(&flagReconfigAddPaths, "add-addons-path", nil, "Add additional addons directories (can specify multiple times)")
	reconfigureCmd.Flags().BoolVar(&flagReconfigAutoDiscover, "auto-discover-deps", false, "Auto-discover Python dependencies from manifests")
	reconfigureCmd.Flags().BoolVar(&flagReconfigRebuild, "rebuild", true, "Rebuild container after reconfiguring")
	reconfigureCmd.Flags().BoolVar(&flagReconfigStopFirst, "stop-first", true, "Stop containers before reconfiguring")
	reconfigureCmd.Flags().BoolVar(&flagReconfigNoCache, "no-cache", false, "Rebuild without Docker layer cache")
}

func runReconfigure(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	// Parse new pip packages
	newPipPackages := make([]string, len(state.PipPackages))
	copy(newPipPackages, state.PipPackages)
	var addedPipPackages []string

	if flagReconfigAddPip != "" {
		addedPkgs := deps.ParsePipPackages(flagReconfigAddPip)
		for _, pkg := range addedPkgs {
			if !contains(newPipPackages, pkg) {
				newPipPackages = append(newPipPackages, pkg)
				addedPipPackages = append(addedPipPackages, pkg)
				fmt.Printf("%s Adding pip package: %s\n", cyan("📦"), pkg)
			}
		}
	}

	// Parse and validate new addons paths
	newAddonsPaths := make([]string, len(state.AddonsPaths))
	copy(newAddonsPaths, state.AddonsPaths)

	for _, path := range flagReconfigAddPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("%s Invalid addons path: %s\n", yellow("⚠️"), path)
			continue
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			fmt.Printf("%s Addons path does not exist or is not a directory: %s\n", yellow("⚠️"), path)
			continue
		}
		if !contains(newAddonsPaths, absPath) {
			newAddonsPaths = append(newAddonsPaths, absPath)
			fmt.Printf("%s Adding addons path: %s\n", cyan("📁"), absPath)
		}
	}

	// Auto-discover dependencies
	if flagReconfigAutoDiscover {
		scanDirs := []string{state.ProjectRoot}
		scanDirs = append(scanDirs, newAddonsPaths...)
		discoveredPkgs := deps.DiscoverPythonDeps(scanDirs, newPipPackages)
		var added []string
		newPipPackages, added = deps.MergePackages(newPipPackages, discoveredPkgs)
		addedPipPackages = append(addedPipPackages, added...)
	}

	// Check if anything changed
	if len(newPipPackages) == len(state.PipPackages) && len(newAddonsPaths) == len(state.AddonsPaths) {
		fmt.Printf("%s No changes to apply\n", yellow("⚠️"))
		return nil
	}

	// Stop containers if requested
	if flagReconfigStopFirst {
		fmt.Println("Stopping containers...")
		if err := docker.Compose(state, "down"); err != nil {
			fmt.Printf("%s Warning: failed to stop containers: %v\n", color.YellowString("⚠️"), err)
		}
	}

	// Update state
	state.PipPackages = newPipPackages
	state.AddonsPaths = newAddonsPaths

	// Regenerate files
	if err := templates.Render(state); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	if len(addedPipPackages) > 0 {
		if err := syncPythonDeps(state, addedPipPackages); err != nil {
			return err
		}
		markPythonDepsSynced(state)
	}

	// Save updated state after runtime dependency sync succeeds.
	if err := state.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}
	if err := config.SaveProjectLink(state); err != nil {
		return fmt.Errorf("failed to save project link: %w", err)
	}

	fmt.Printf("\n%s Docker configuration updated!\n", green("✓"))

	// Rebuild if requested
	if flagReconfigRebuild {
		fmt.Println("\nRebuilding container...")
		buildArgs := []string{"build"}
		if flagReconfigNoCache {
			buildArgs = append(buildArgs, "--no-cache")
		}
		if err := docker.Compose(state, buildArgs...); err != nil {
			return fmt.Errorf("failed to rebuild: %w", err)
		}
		fmt.Printf("%s Container rebuilt successfully!\n", green("✓"))

		confirmed, err := prompt.Confirm("\nStart containers now?", true)
		if err == nil && confirmed {
			if err := docker.Compose(state, "up", "-d"); err != nil {
				return fmt.Errorf("failed to start containers: %w", err)
			}
			fmt.Printf("\n%s Odoo: http://localhost:%d\n", cyan("🌐"), state.Ports.Odoo)

			if len(newAddonsPaths) > 0 {
				fmt.Printf("\n%s Next steps: Apps → Update Apps List → Search module\n", yellow("📋"))
			}
		}
	} else {
		fmt.Printf("\n%s Remember to rebuild: odooctl docker run --build\n", yellow("⚠️"))
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
