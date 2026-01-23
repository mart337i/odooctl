package docker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/docker"
	"github.com/egeskov/odooctl/internal/module"
	"github.com/egeskov/odooctl/internal/templates"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagReconfigAddPip       string
	flagReconfigAddPaths     []string
	flagReconfigAutoDiscover bool
	flagReconfigRebuild      bool
	flagReconfigStopFirst    bool
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

	if flagReconfigAddPip != "" {
		addedPkgs := parsePipPackagesForReconfigure(flagReconfigAddPip)
		for _, pkg := range addedPkgs {
			if !contains(newPipPackages, pkg) {
				newPipPackages = append(newPipPackages, pkg)
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
		discoveredPkgs := discoverPythonDepsForReconfigure(scanDirs, newPipPackages)
		newPipPackages = append(newPipPackages, discoveredPkgs...)
	}

	// Check if anything changed
	if len(newPipPackages) == len(state.PipPackages) && len(newAddonsPaths) == len(state.AddonsPaths) {
		fmt.Printf("%s No changes to apply\n", yellow("⚠️"))
		return nil
	}

	// Stop containers if requested
	if flagReconfigStopFirst {
		fmt.Println("Stopping containers...")
		docker.Compose(state, "down")
	}

	// Update state
	state.PipPackages = newPipPackages
	state.AddonsPaths = newAddonsPaths

	// Regenerate files
	if err := templates.Render(state); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	// Save updated state
	if err := state.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Printf("\n%s Docker configuration updated!\n", green("✓"))

	// Rebuild if requested
	if flagReconfigRebuild {
		fmt.Println("\nRebuilding container...")
		if err := docker.Compose(state, "build", "--no-cache"); err != nil {
			return fmt.Errorf("failed to rebuild: %w", err)
		}
		fmt.Printf("%s Container rebuilt successfully!\n", green("✓"))

		fmt.Print("\nStart containers now? [Y/n]: ")
		var response string
		fmt.Scanln(&response)
		if response == "" || strings.ToLower(response) == "y" || strings.ToLower(response) == "yes" {
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

func parsePipPackagesForReconfigure(input string) []string {
	if input == "" {
		return nil
	}

	// Check if it's a file path
	if strings.HasSuffix(input, ".txt") || strings.Contains(input, "/") {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return parseCommaSeparatedForReconfigure(input)
		}

		file, err := os.Open(absPath)
		if err != nil {
			return parseCommaSeparatedForReconfigure(input)
		}
		defer file.Close()

		var packages []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			packages = append(packages, line)
		}

		if len(packages) > 0 {
			fmt.Printf("%s Loaded %d packages from %s\n", color.CyanString("📦"), len(packages), input)
			return packages
		}
	}

	return parseCommaSeparatedForReconfigure(input)
}

func parseCommaSeparatedForReconfigure(input string) []string {
	var packages []string
	for _, pkg := range strings.Split(input, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			packages = append(packages, pkg)
		}
	}
	return packages
}

func discoverPythonDepsForReconfigure(dirs []string, existingPkgs []string) []string {
	existingSet := make(map[string]bool)
	for _, pkg := range existingPkgs {
		name := strings.Split(pkg, "==")[0]
		name = strings.Split(name, ">=")[0]
		name = strings.Split(name, "<=")[0]
		name = strings.Split(name, "[")[0]
		existingSet[strings.ToLower(name)] = true
	}

	discovered := make(map[string][]string)

	for _, dir := range dirs {
		modules, _ := module.FindModules(dir)
		for _, mod := range modules {
			manifestPath := filepath.Join(dir, mod, "__manifest__.py")
			deps := parseManifestPythonDeps(manifestPath)
			for _, dep := range deps {
				depLower := strings.ToLower(dep)
				if !existingSet[depLower] {
					discovered[dep] = append(discovered[dep], mod)
				}
			}
		}
	}

	if len(discovered) == 0 {
		return nil
	}

	fmt.Printf("\n%s Python dependencies detected in manifests:\n", color.CyanString("🔍"))

	var selected []string
	for pkg, mods := range discovered {
		fmt.Printf("\n%s %s\n", color.YellowString("📦"), pkg)
		fmt.Printf("   Required by: %s\n", color.HiBlackString(strings.Join(mods, ", ")))

		fmt.Printf("   Include %s? [Y/n]: ", pkg)
		var response string
		fmt.Scanln(&response)
		if response == "" || strings.ToLower(response) == "y" || strings.ToLower(response) == "yes" {
			selected = append(selected, pkg)
			fmt.Printf("   %s Will install %s\n", color.GreenString("✓"), pkg)
		} else {
			fmt.Printf("   %s Skipped\n", color.YellowString("⚠️"))
		}
	}

	if len(selected) > 0 {
		fmt.Printf("\n%s Added %d Python packages from manifests\n", color.GreenString("✓"), len(selected))
	}

	return selected
}
