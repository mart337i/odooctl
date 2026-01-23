package docker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/module"
	"github.com/egeskov/odooctl/internal/odoo"
	"github.com/egeskov/odooctl/internal/project"
	"github.com/egeskov/odooctl/internal/templates"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagName            string
	flagOdooVersion     string
	flagModules         string
	flagEnterprise      bool
	flagWithoutDemo     bool
	flagPip             string
	flagAddonsPaths     []string
	flagAutoDiscoverPip bool
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Docker development environment",
	Long:  `Generates Docker Compose, Dockerfile, and configuration files for Odoo development.`,
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&flagName, "name", "n", "", "Environment name (used as subdirectory, allows multiple environments per project)")
	createCmd.Flags().StringVarP(&flagOdooVersion, "odoo-version", "v", "", "Odoo version ("+odoo.VersionsString()+")")
	createCmd.Flags().StringVarP(&flagModules, "modules", "m", "", "Modules to install (comma-separated)")
	createCmd.Flags().BoolVarP(&flagEnterprise, "enterprise", "e", false, "Include Odoo Enterprise")
	createCmd.Flags().BoolVar(&flagWithoutDemo, "without-demo", false, "Initialize without demo data")
	createCmd.Flags().StringVarP(&flagPip, "pip", "p", "", "Extra pip packages (comma-separated or path to requirements.txt)")
	createCmd.Flags().StringArrayVarP(&flagAddonsPaths, "addons-path", "a", nil, "Additional addons directories (can specify multiple times)")
	createCmd.Flags().BoolVar(&flagAutoDiscoverPip, "auto-discover-deps", true, "Auto-discover Python dependencies from manifests")
}

func runCreate(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Detect project context
	ctx := project.Detect(cwd)

	// Handle --name flag based on git repo context
	// In git repo: --name overrides project name (existing behavior preserved for backwards compat)
	// Outside git repo: --name sets the environment name (branch), allowing multiple environments
	if ctx.IsGitRepo {
		// In git repo: branch comes from git, --name can override project name
		if flagName != "" {
			ctx.Name = flagName
		}
	} else {
		// Outside git repo: --name sets the environment name
		// Default to project name if --name not provided (creates projectname/projectname)
		if flagName != "" {
			ctx.Branch = flagName
		} else {
			ctx.Branch = ctx.Name
		}
	}

	if flagOdooVersion != "" {
		ctx.OdooVersion = flagOdooVersion
	}

	// Prompt for version if not determined
	if ctx.OdooVersion == "" {
		version, err := prompt.SelectVersion()
		if err != nil {
			return err
		}
		ctx.OdooVersion = version
	}

	// Check for existing environment
	if config.EnvironmentExists(ctx.Name, ctx.Branch) {
		return fmt.Errorf("environment '%s/%s' already exists. Use a different --name or remove the existing environment with 'odooctl docker reset'", ctx.Name, ctx.Branch)
	}

	// Parse modules
	var modules []string
	if flagModules != "" {
		modules = strings.Split(flagModules, ",")
		for i := range modules {
			modules[i] = strings.TrimSpace(modules[i])
		}
	}

	// Parse pip packages (supports comma-separated or requirements.txt)
	pipPkgs := parsePipPackages(flagPip)

	// Parse and validate addons paths
	var addonsPaths []string
	for _, path := range flagAddonsPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("%s Invalid addons path: %s\n", color.YellowString("⚠️"), path)
			continue
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			fmt.Printf("%s Addons path does not exist or is not a directory: %s\n", color.YellowString("⚠️"), path)
			continue
		}
		addonsPaths = append(addonsPaths, absPath)
		fmt.Printf("%s Added addons path: %s\n", color.CyanString("📁"), absPath)
	}

	// Auto-discover Python dependencies from manifests
	if flagAutoDiscoverPip {
		scanDirs := []string{ctx.Root}
		scanDirs = append(scanDirs, addonsPaths...)
		discoveredPkgs := discoverPythonDeps(scanDirs, pipPkgs)
		pipPkgs = append(pipPkgs, discoveredPkgs...)
	}

	branchOrVersion := ctx.Branch
	if branchOrVersion == "" {
		branchOrVersion = ctx.OdooVersion
	}

	dockerRoot := filepath.Join(ctx.Root, ctx.Name, branchOrVersion, "docker")

	// Ensure directory exists
	if err := os.MkdirAll(dockerRoot, 0755); err != nil {
		return fmt.Errorf("failed to create docker directory: %w", err)
	}

	// Build state
	state := &config.State{
		ProjectName: ctx.Name,
		OdooVersion: ctx.OdooVersion,
		Branch:      ctx.Branch,
		IsGitRepo:   ctx.IsGitRepo,
		ProjectRoot: ctx.Root,
		Modules:     modules,
		Enterprise:  flagEnterprise,
		WithoutDemo: flagWithoutDemo,
		PipPackages: pipPkgs,
		AddonsPaths: addonsPaths,
		Ports:       config.CalculatePorts(ctx.OdooVersion),
		CreatedAt:   time.Now(),
	}

	// Render templates
	if err := templates.Render(state); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	// Save state
	if err := state.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	// Print summary
	printCreateSummary(state)

	return nil
}

func printCreateSummary(state *config.State) {
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Println()
	fmt.Printf("%s Docker environment created!\n\n", green("✓"))
	fmt.Printf("  Project:     %s\n", cyan(state.ProjectName))
	fmt.Printf("  Environment: %s\n", cyan(state.Branch))
	fmt.Printf("  Odoo:        %s\n", cyan(state.OdooVersion))
	fmt.Printf("  Port:        %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)))
	fmt.Printf("  Mailhog:     %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)))

	dir, _ := config.EnvironmentDir(state.ProjectName, state.Branch)
	fmt.Printf("  Files:       %s\n", cyan(dir))

	if len(state.AddonsPaths) > 0 {
		fmt.Printf("  Addons:      %d custom path(s)\n", len(state.AddonsPaths))
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. %s  # Initialize database and start containers\n", cyan("odooctl docker run"))
	fmt.Printf("  2. %s   # View container status\n", cyan("odooctl docker status"))
}

// parsePipPackages parses pip packages from comma-separated string or requirements.txt file
func parsePipPackages(input string) []string {
	if input == "" {
		return nil
	}

	// Check if it's a file path
	if strings.HasSuffix(input, ".txt") || strings.Contains(input, "/") {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return parseCommaSeparated(input)
		}

		file, err := os.Open(absPath)
		if err != nil {
			return parseCommaSeparated(input)
		}
		defer file.Close()

		var packages []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip empty lines and comments
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

	return parseCommaSeparated(input)
}

func parseCommaSeparated(input string) []string {
	var packages []string
	for _, pkg := range strings.Split(input, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			packages = append(packages, pkg)
		}
	}
	return packages
}

// discoverPythonDeps scans manifests for external_dependencies.python
func discoverPythonDeps(dirs []string, existingPkgs []string) []string {
	existingSet := make(map[string]bool)
	for _, pkg := range existingPkgs {
		// Normalize package name (remove version specs)
		name := strings.Split(pkg, "==")[0]
		name = strings.Split(name, ">=")[0]
		name = strings.Split(name, "<=")[0]
		name = strings.Split(name, "[")[0]
		existingSet[strings.ToLower(name)] = true
	}

	discovered := make(map[string][]string) // package -> modules requiring it

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
			fmt.Printf("   %s Skipped - module(s) may fail without it\n", color.YellowString("⚠️"))
		}
	}

	if len(selected) > 0 {
		fmt.Printf("\n%s Added %d Python packages from manifests\n", color.GreenString("✓"), len(selected))
	}

	return selected
}

// parseManifestPythonDeps extracts python deps from __manifest__.py
func parseManifestPythonDeps(manifestPath string) []string {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}

	// Simple regex-free parsing for external_dependencies.python
	text := string(content)

	// Find external_dependencies
	extIdx := strings.Index(text, "external_dependencies")
	if extIdx == -1 {
		return nil
	}

	// Find the next opening brace
	braceStart := strings.Index(text[extIdx:], "{")
	if braceStart == -1 {
		return nil
	}

	// Find matching closing brace
	braceCount := 1
	start := extIdx + braceStart + 1
	end := start
	for i := start; i < len(text) && braceCount > 0; i++ {
		switch text[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
		}
		end = i
	}

	extDeps := text[start:end]

	// Find python list
	pythonIdx := strings.Index(extDeps, "'python'")
	if pythonIdx == -1 {
		pythonIdx = strings.Index(extDeps, "\"python\"")
	}
	if pythonIdx == -1 {
		return nil
	}

	// Find the list
	listStart := strings.Index(extDeps[pythonIdx:], "[")
	if listStart == -1 {
		return nil
	}
	listEnd := strings.Index(extDeps[pythonIdx+listStart:], "]")
	if listEnd == -1 {
		return nil
	}

	listContent := extDeps[pythonIdx+listStart+1 : pythonIdx+listStart+listEnd]

	// Parse package names
	var packages []string
	for _, item := range strings.Split(listContent, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "'\"")
		if item != "" {
			packages = append(packages, item)
		}
	}

	return packages
}
