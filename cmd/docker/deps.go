package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	pydeps "github.com/mart337i/odooctl/internal/deps"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

const pyDepsDir = "/opt/odoo-extra-python"

var flagDepsModules string

var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Manage Python dependencies discovered from Odoo modules",
}

var depsScanCmd = &cobra.Command{
	Use:   "scan [modules...]",
	Short: "Scan module manifests for external Python dependencies",
	Args:  cobra.ArbitraryArgs,
	RunE:  runDepsScan,
}

var depsSyncCmd = &cobra.Command{
	Use:          "sync [packages...]",
	Short:        "Install Python dependencies into the runtime dependency volume",
	SilenceUsage: true,
	Long: `Install Python dependencies into the runtime dependency volume.

If packages are omitted, odooctl scans module manifests and installs missing
external_dependencies['python'] packages.`,
	Args: cobra.ArbitraryArgs,
	RunE: runDepsSync,
}

var depsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Python dependencies recorded for this environment",
	RunE:  runDepsList,
}

var depsCleanCmd = &cobra.Command{
	Use:          "clean",
	Short:        "Remove installed runtime Python dependencies from the volume",
	SilenceUsage: true,
	RunE:         runDepsClean,
}

func init() {
	depsScanCmd.Flags().StringVarP(&flagDepsModules, "modules", "m", "", "Modules to scan (comma-separated)")
	depsSyncCmd.Flags().StringVarP(&flagDepsModules, "modules", "m", "", "Modules to scan when packages are omitted (comma-separated)")
	depsCmd.AddCommand(depsScanCmd)
	depsCmd.AddCommand(depsSyncCmd)
	depsCmd.AddCommand(depsListCmd)
	depsCmd.AddCommand(depsCleanCmd)
}

func runDepsScan(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	discovered := discoverStatePythonDeps(state, mergeStringLists(args, splitCSV(flagDepsModules)))
	if len(discovered) == 0 {
		fmt.Println("No Python dependencies found in module manifests")
		return nil
	}
	printDiscoveredPythonDeps(discovered, state.PipPackages)
	return nil
}

func runDepsSync(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if err := ensureDockerProjectAccess(state); err != nil {
		return err
	}

	packages := cleanStrings(args)
	if len(packages) == 0 {
		discovered := discoverStatePythonDeps(state, splitCSV(flagDepsModules))
		packages = pydeps.MissingPythonDeps(discovered, state.PipPackages)
		if len(packages) == 0 {
			fmt.Println("No missing Python dependencies found")
			return nil
		}
		printDiscoveredPythonDeps(discovered, state.PipPackages)
	}

	if err := syncPythonDeps(state, packages); err != nil {
		return err
	}
	merged, _ := pydeps.MergePackages(state.PipPackages, packages)
	state.PipPackages = merged
	markPythonDepsSynced(state)
	if err := state.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}
	if err := config.SaveProjectLink(state); err != nil {
		return fmt.Errorf("failed to save project link: %w", err)
	}
	fmt.Printf("%s Python dependencies synced\n", color.GreenString("✓"))
	return nil
}

func runDepsList(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if len(state.PipPackages) == 0 {
		fmt.Println("No Python dependencies recorded for this environment")
		return nil
	}
	for _, pkg := range state.PipPackages {
		fmt.Println(pkg)
	}
	return nil
}

func runDepsClean(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if err := ensureDockerProjectAccess(state); err != nil {
		return err
	}
	script := fmt.Sprintf("set -e; mkdir -p %[1]s; find %[1]s -mindepth 1 -maxdepth 1 -exec rm -rf {} +", pyDepsDir)
	if err := docker.Compose(state, "run", "--rm", "odoo", "sh", "-lc", script); err != nil {
		return fmt.Errorf("failed to clean Python dependency volume: %w", err)
	}
	state.PythonDepsHash = ""
	state.PythonDepsSyncedAt = nil
	if err := state.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}
	if err := config.SaveProjectLink(state); err != nil {
		return fmt.Errorf("failed to save project link: %w", err)
	}
	fmt.Printf("%s Runtime Python dependency volume cleaned\n", color.GreenString("✓"))
	return nil
}

func syncPythonDeps(state *config.State, packages []string) error {
	packages = cleanStrings(packages)
	if len(packages) == 0 {
		return nil
	}
	fmt.Printf("%s Installing Python dependencies: %s\n", color.CyanString("📦"), strings.Join(packages, ", "))
	script := fmt.Sprintf(`set -e
mkdir -p %[1]s
python -m pip install --target %[1]s --upgrade "$@"
python - "$@" > %[1]s/odooctl-python-deps.json <<'PY'
import json
import sys
import time

json.dump({"packages": sys.argv[1:], "installed_at": time.strftime("%%Y-%%m-%%dT%%H:%%M:%%SZ", time.gmtime())}, sys.stdout, indent=2)
print()
PY`, pyDepsDir)
	args := []string{"run", "--rm", "odoo", "sh", "-lc", script, "sh"}
	args = append(args, packages...)
	if err := docker.Compose(state, args...); err != nil {
		return fmt.Errorf("failed to sync Python dependencies: %w", err)
	}
	return nil
}

func ensureConfiguredPythonDepsSynced(state *config.State) (bool, error) {
	if len(state.PipPackages) == 0 || pythonDepsSynced(state) {
		return false, nil
	}
	if err := syncPythonDeps(state, state.PipPackages); err != nil {
		return false, err
	}
	markPythonDepsSynced(state)
	if err := state.Save(); err != nil {
		return false, fmt.Errorf("failed to save state: %w", err)
	}
	if err := config.SaveProjectLink(state); err != nil {
		return false, fmt.Errorf("failed to save project link: %w", err)
	}
	return true, nil
}

func pythonDepsSynced(state *config.State) bool {
	return state.PythonDepsHash != "" && state.PythonDepsHash == pythonDepsHash(state.PipPackages)
}

func markPythonDepsSynced(state *config.State) {
	now := time.Now()
	state.PythonDepsHash = pythonDepsHash(state.PipPackages)
	state.PythonDepsSyncedAt = &now
}

func pythonDepsHash(packages []string) string {
	packages = cleanStrings(packages)
	for i, pkg := range packages {
		packages[i] = strings.ToLower(pkg)
	}
	sort.Strings(packages)
	hash := sha256.Sum256([]byte(strings.Join(packages, "\n")))
	return hex.EncodeToString(hash[:])
}

func discoverStatePythonDeps(state *config.State, modules []string) map[string][]string {
	dirs := []string{state.ProjectRoot}
	dirs = append(dirs, state.AddonsPaths...)
	return pydeps.DiscoverPythonDepsForModules(dirs, cleanStrings(modules))
}

func printDiscoveredPythonDeps(discovered map[string][]string, existing []string) {
	missingSet := make(map[string]bool)
	for _, pkg := range pydeps.MissingPythonDeps(discovered, existing) {
		missingSet[pkg] = true
	}
	for _, pkg := range pydeps.SortedDiscoveredPackages(discovered) {
		status := color.GreenString("configured")
		if missingSet[pkg] {
			status = color.YellowString("missing")
		}
		fmt.Printf("%s %s (%s) required by %s\n", color.CyanString("📦"), pkg, status, strings.Join(discovered[pkg], ", "))
	}
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	return cleanStrings(strings.Split(value, ","))
}

func cleanStrings(values []string) []string {
	var cleaned []string
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func mergeStringLists(a, b []string) []string {
	return cleanStrings(append(append([]string{}, a...), b...))
}

func ciMode() bool {
	return strings.EqualFold(os.Getenv("CI"), "true") || os.Getenv("CI") == "1"
}
