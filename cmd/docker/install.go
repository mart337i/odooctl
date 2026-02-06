package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/egeskov/odooctl/internal/module"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagInstallListOnly      bool
	flagInstallComputeHashes bool
	flagInstallIgnore        string
	flagInstallUpdateAll     bool
	flagInstallIgnoreCore    bool
)

var installCmd = &cobra.Command{
	Use:   "install [modules...]",
	Short: "Install or update Odoo modules",
	Long: `Install or update Odoo modules with hash-based change detection.

For LOCAL modules (in your project directory):
  - Calculates hashes to detect changes
  - Only updates modules that have actually changed
  - Supports wildcards: sale_*, purchase_*, *_account
  - Use "all" to detect all local modules

For EXTERNAL modules (Odoo core like sale, purchase):
  - Passes directly to odoo-bin without hash checking

Examples:
  odooctl docker install                  # Auto-detect changed local modules
  odooctl docker install sale purchase    # Install core modules
  odooctl docker install my_module        # Install local module  
  odooctl docker install sale_*           # Wildcard for local modules
  odooctl docker install all              # All local modules
  odooctl docker install --list-only      # Dry run
  odooctl docker install --update-all     # Force -u base (full upgrade)
  odooctl docker install --compute-hashes # Store hashes without updating`,
	RunE: runInstall,
}

func init() {
	installCmd.Flags().BoolVarP(&flagInstallListOnly, "list-only", "l", false, "List modules that would be updated")
	installCmd.Flags().BoolVar(&flagInstallComputeHashes, "compute-hashes", false, "Only compute and store hashes")
	installCmd.Flags().StringVar(&flagInstallIgnore, "ignore", "", "Modules to ignore (comma-separated)")
	installCmd.Flags().BoolVar(&flagInstallUpdateAll, "update-all", false, "Force complete upgrade (-u base)")
	installCmd.Flags().BoolVar(&flagInstallIgnoreCore, "ignore-core", false, "Ignore Odoo core addons in change detection")
}

func runInstall(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	// Handle --update-all flag (force -u base)
	if flagInstallUpdateAll {
		fmt.Println("Running full upgrade (-u base)...")

		// Stop the odoo container before running upgrade
		fmt.Println("Stopping Odoo container...")
		if err := docker.Compose(state, "stop", "odoo"); err != nil {
			fmt.Printf("%s Warning: failed to stop odoo container: %v\n", yellow("!"), err)
		}

		// Run the upgrade
		upgradeErr := runOdooUpdate(state, nil, []string{"base"})

		// Always restart the odoo container, even if upgrade failed
		fmt.Println("Restarting Odoo container...")
		if err := docker.Compose(state, "up", "-d", "odoo"); err != nil {
			fmt.Printf("%s Warning: failed to restart odoo container: %v\n", yellow("!"), err)
			if upgradeErr == nil {
				return fmt.Errorf("upgrade succeeded but failed to restart container: %w", err)
			}
		}

		// Return upgrade result
		if upgradeErr != nil {
			return upgradeErr
		}

		fmt.Printf("\n%s Full upgrade complete\n", green("✓"))
		return nil
	}

	// Find available LOCAL modules
	localModules, _ := module.FindModules(state.ProjectRoot)
	localModuleSet := make(map[string]bool)
	for _, m := range localModules {
		localModuleSet[m] = true
	}

	// Separate args into local vs external modules
	var localTargets []string
	var externalTargets []string

	if len(args) == 0 {
		// No args: auto-detect changed local modules
		localTargets = localModules
	} else if len(args) == 1 && strings.ToLower(args[0]) == "all" {
		// "all" means all LOCAL modules only
		localTargets = localModules
	} else {
		for _, arg := range args {
			// Check if it's a pattern
			if strings.ContainsAny(arg, "*?[") {
				// Expand pattern against local modules
				expanded := module.ExpandPatterns([]string{arg}, localModules)
				localTargets = append(localTargets, expanded...)
			} else if localModuleSet[arg] {
				// It's a local module
				localTargets = append(localTargets, arg)
			} else {
				// It's an external/core module
				if !flagInstallIgnoreCore {
					externalTargets = append(externalTargets, arg)
				}
			}
		}
	}

	// Apply ignore filter
	if flagInstallIgnore != "" {
		ignoreList := strings.Split(flagInstallIgnore, ",")
		ignoreMap := make(map[string]bool)
		for _, m := range ignoreList {
			ignoreMap[strings.TrimSpace(m)] = true
		}

		var filteredLocal []string
		for _, m := range localTargets {
			if !ignoreMap[m] {
				filteredLocal = append(filteredLocal, m)
			}
		}
		localTargets = filteredLocal

		var filteredExternal []string
		for _, m := range externalTargets {
			if !ignoreMap[m] {
				filteredExternal = append(filteredExternal, m)
			}
		}
		externalTargets = filteredExternal
	}

	// Handle hash-based detection for local modules
	var localInstall, localUpdate []string
	currentHashes := make(map[string]string)

	if len(localTargets) > 0 {
		storedHashes, err := loadHashes(state)
		if err != nil {
			storedHashes = make(map[string]string)
		}

		fmt.Printf("Checking %d local modules...\n", len(localTargets))

		for _, mod := range localTargets {
			modPath := filepath.Join(state.ProjectRoot, mod)
			hash, err := module.Hash(modPath)
			if err != nil {
				fmt.Printf("%s Failed to hash %q: %v\n", yellow("!"), mod, err)
				continue
			}
			currentHashes[mod] = hash

			storedHash, exists := storedHashes[mod]
			if !exists {
				localInstall = append(localInstall, mod)
			} else if storedHash != hash {
				localUpdate = append(localUpdate, mod)
			}
		}

		// Compute hashes only mode
		if flagInstallComputeHashes {
			for k, v := range currentHashes {
				storedHashes[k] = v
			}
			if err := saveHashes(state, storedHashes); err != nil {
				return fmt.Errorf("failed to save hashes: %w", err)
			}
			fmt.Printf("%s Computed and saved hashes for %d modules\n", green("✓"), len(currentHashes))
			return nil
		}

		// List only mode
		if flagInstallListOnly {
			if len(localInstall) > 0 {
				fmt.Printf("\nNew local modules to install (%d):\n", len(localInstall))
				for _, m := range localInstall {
					fmt.Printf("  %s %s\n", cyan("+"), m)
				}
			}
			if len(localUpdate) > 0 {
				fmt.Printf("\nChanged local modules to update (%d):\n", len(localUpdate))
				for _, m := range localUpdate {
					fmt.Printf("  %s %s\n", yellow("~"), m)
				}
			}
			if len(localInstall) == 0 && len(localUpdate) == 0 {
				fmt.Println("\nNo local modules need updating")
			}
			if len(externalTargets) > 0 {
				fmt.Printf("\nExternal modules to install: %s\n", cyan(strings.Join(externalTargets, ", ")))
			}
			return nil
		}
	}

	// Nothing to do?
	if len(localInstall) == 0 && len(localUpdate) == 0 && len(externalTargets) == 0 {
		if len(localTargets) > 0 {
			fmt.Printf("%s All local modules are up to date\n", green("✓"))
		} else if len(args) == 0 {
			fmt.Printf("%s No local modules found and no modules specified\n", yellow("!"))
		} else {
			fmt.Println("No modules to install")
		}
		return nil
	}

	// Combine install and update lists
	var allInstall, allUpdate []string

	// External modules are always treated as install (Odoo handles if already installed)
	allInstall = append(allInstall, externalTargets...)
	allInstall = append(allInstall, localInstall...)
	allUpdate = localUpdate

	// Print what we're doing
	if len(allInstall) > 0 {
		fmt.Printf("\nInstalling: %s\n", cyan(strings.Join(allInstall, ", ")))
	}
	if len(allUpdate) > 0 {
		fmt.Printf("Updating: %s\n", yellow(strings.Join(allUpdate, ", ")))
	}

	// Stop the odoo container before running install/update
	fmt.Println("\nStopping Odoo container...")
	if err := docker.Compose(state, "stop", "odoo"); err != nil {
		fmt.Printf("%s Warning: failed to stop odoo container: %v\n", yellow("!"), err)
	}

	// Run odoo-bin via docker compose
	fmt.Println("Running install/update...")
	installErr := runOdooUpdate(state, allInstall, allUpdate)

	// Always restart the odoo container, even if install failed
	fmt.Println("Restarting Odoo container...")
	if err := docker.Compose(state, "up", "-d", "odoo"); err != nil {
		fmt.Printf("%s Warning: failed to restart odoo container: %v\n", yellow("!"), err)
		if installErr == nil {
			return fmt.Errorf("install succeeded but failed to restart container: %w", err)
		}
	}

	// If install failed, return that error now
	if installErr != nil {
		return installErr
	}

	// Save new hashes for local modules
	if len(currentHashes) > 0 {
		storedHashes, _ := loadHashes(state)
		if storedHashes == nil {
			storedHashes = make(map[string]string)
		}
		for k, v := range currentHashes {
			storedHashes[k] = v
		}
		if err := saveHashes(state, storedHashes); err != nil {
			fmt.Printf("%s Warning: failed to save hashes: %v\n", yellow("!"), err)
		}
	}

	fmt.Printf("\n%s Installation complete\n", green("✓"))
	return nil
}

func runOdooUpdate(state *config.State, install, update []string) error {
	// Build odoo-bin command
	args := []string{
		"run", "--rm", "odoo",
		"odoo", "-c", "/etc/odoo/odoo.conf",
		"-d", state.DBName(),
	}

	if len(install) > 0 {
		args = append(args, "-i", strings.Join(install, ","))
	}
	if len(update) > 0 {
		args = append(args, "-u", strings.Join(update, ","))
	}
	args = append(args, "--stop-after-init")

	return docker.Compose(state, args...)
}

func hashFilePath(state *config.State) (string, error) {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "module-hashes.json"), nil
}

func loadHashes(state *config.State) (map[string]string, error) {
	path, err := hashFilePath(state)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var hashes map[string]string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, err
	}

	return hashes, nil
}

func saveHashes(state *config.State, hashes map[string]string) error {
	path, err := hashFilePath(state)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
