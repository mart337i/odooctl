package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/deps"
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
	pipPkgs := deps.ParsePipPackages(flagPip)

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
		discoveredPkgs := deps.DiscoverPythonDeps(scanDirs, pipPkgs)
		pipPkgs = append(pipPkgs, discoveredPkgs...)
	}

	// Handle enterprise authentication if needed
	var enterpriseToken, enterpriseSSHKeyPath string
	if flagEnterprise {
		var err error
		enterpriseToken, enterpriseSSHKeyPath, err = promptEnterpriseAuth()
		if err != nil {
			return fmt.Errorf("enterprise authentication failed: %w", err)
		}
	}

	// Build state
	state := &config.State{
		ProjectName:           ctx.Name,
		OdooVersion:           ctx.OdooVersion,
		Branch:                ctx.Branch,
		IsGitRepo:             ctx.IsGitRepo,
		ProjectRoot:           ctx.Root,
		Modules:               modules,
		Enterprise:            flagEnterprise,
		EnterpriseGitHubToken: enterpriseToken,
		EnterpriseSSHKeyPath:  enterpriseSSHKeyPath,
		WithoutDemo:           flagWithoutDemo,
		PipPackages:           pipPkgs,
		AddonsPaths:           addonsPaths,
		Ports:                 config.CalculatePorts(ctx.OdooVersion),
		CreatedAt:             time.Now(),
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

// promptEnterpriseAuth returns (token, sshKeyPath, error).
// It loads global config first and offers to reuse saved credentials.
// Any new credentials entered are persisted to global config for future use.
func promptEnterpriseAuth() (string, string, error) {
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return "", "", err
	}

	fmt.Println()
	fmt.Printf("%s Enterprise access requires authentication\n\n", green("🔐"))

	// If we already have a saved token or SSH key, offer to reuse
	if globalCfg.GitHubToken != "" {
		fmt.Printf("%s Saved GitHub token found (%s)\n", cyan("ℹ"), config.MaskToken(globalCfg.GitHubToken))
		reuse, err := prompt.Confirm("Use saved GitHub token?", true)
		if err != nil {
			return "", "", err
		}
		if reuse {
			fmt.Printf("%s Using saved GitHub token\n\n", green("✓"))
			return globalCfg.GitHubToken, "", nil
		}
	} else if globalCfg.SSHKeyPath != "" {
		fmt.Printf("%s Saved SSH key found (%s)\n", cyan("ℹ"), globalCfg.SSHKeyPath)
		reuse, err := prompt.Confirm("Use saved SSH key?", true)
		if err != nil {
			return "", "", err
		}
		if reuse {
			fmt.Printf("%s Using saved SSH key\n\n", green("✓"))
			return "", globalCfg.SSHKeyPath, nil
		}
	}

	// Discover available SSH keys on the system
	home, _ := os.UserHomeDir()
	var detectedSSHKeys []string
	if home != "" {
		candidates := []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_rsa"),
			filepath.Join(home, ".ssh", "id_ecdsa"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				detectedSSHKeys = append(detectedSSHKeys, p)
			}
		}
	}

	// Build choice menu
	fmt.Println("Choose authentication method:")
	if len(detectedSSHKeys) > 0 {
		fmt.Printf("  [1] SSH Key %s\n", cyan("(key detected)"))
	} else {
		fmt.Printf("  [1] SSH Key %s\n", yellow("(enter path manually)"))
	}
	fmt.Printf("  [2] Personal Access Token %s\n", cyan("(recommended)"))
	fmt.Println()

	choice, err := prompt.InputString("Select option [1-2]:", "2")
	if err != nil {
		return "", "", err
	}

	if choice == "1" {
		return promptSSHKey(globalCfg, detectedSSHKeys)
	}
	return promptToken(globalCfg)
}

func promptSSHKey(globalCfg *config.GlobalConfig, detectedKeys []string) (string, string, error) {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	var keyPath string

	if len(detectedKeys) == 1 {
		// Only one key found, use it directly
		keyPath = detectedKeys[0]
		fmt.Printf("%s Using detected SSH key: %s\n", green("✓"), keyPath)
	} else if len(detectedKeys) > 1 {
		// Multiple keys -- let user pick
		fmt.Println("\nDetected SSH keys:")
		for i, k := range detectedKeys {
			fmt.Printf("  [%d] %s\n", i+1, k)
		}
		fmt.Printf("  [%d] Enter path manually\n", len(detectedKeys)+1)
		fmt.Println()

		defaultChoice := "1"
		choice, err := prompt.InputString(fmt.Sprintf("Select key [1-%d]:", len(detectedKeys)+1), defaultChoice)
		if err != nil {
			return "", "", err
		}

		idx := 0
		if _, err := fmt.Sscanf(choice, "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(detectedKeys) {
				keyPath = detectedKeys[idx-1]
			}
		}
		// else fall through to manual input
	}

	if keyPath == "" {
		// Manual path entry
		path, err := prompt.InputString("Enter path to SSH private key:", "~/.ssh/id_ed25519")
		if err != nil {
			return "", "", err
		}
		expanded, err := config.ExpandPath(path)
		if err != nil {
			return "", "", err
		}
		if _, err := os.Stat(expanded); err != nil {
			return "", "", fmt.Errorf("SSH key file not found: %s", expanded)
		}
		keyPath = expanded
	}

	// Offer to save globally
	save, err := prompt.Confirm("Save this SSH key path for future environments?", true)
	if err != nil {
		return "", "", err
	}
	if save {
		globalCfg.SSHKeyPath = keyPath
		globalCfg.GitHubToken = "" // clear token if switching to SSH
		if err := globalCfg.Save(); err != nil {
			fmt.Printf("%s Could not save to global config: %v\n", yellow("⚠"), err)
		} else {
			fmt.Printf("%s SSH key path saved globally\n", green("✓"))
		}
	}

	fmt.Printf("%s Make sure this key is added to GitHub: https://github.com/settings/keys\n\n", yellow("ℹ"))
	return "", keyPath, nil
}

func promptToken(globalCfg *config.GlobalConfig) (string, string, error) {
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println()
	fmt.Printf("%s To create a Personal Access Token:\n", cyan("ℹ"))
	fmt.Printf("  1. Visit: %s\n", cyan("https://github.com/settings/tokens/new"))
	fmt.Printf("  2. Set description: %s\n", cyan("Odoo Enterprise Access"))
	fmt.Printf("  3. Select scope: %s\n", cyan("repo (Full control of private repositories)"))
	fmt.Printf("  4. Click %s and copy the token\n\n", cyan("'Generate token'"))

	token, err := prompt.InputPassword("Enter GitHub Personal Access Token:")
	if err != nil {
		return "", "", err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", fmt.Errorf("token cannot be empty")
	}

	if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") {
		fmt.Printf("\n%s Token doesn't match expected format (should start with 'ghp_' or 'github_pat_')\n", yellow("⚠"))
		confirm, err := prompt.Confirm("Continue anyway?", false)
		if err != nil {
			return "", "", err
		}
		if !confirm {
			return "", "", fmt.Errorf("authentication cancelled")
		}
	}

	// Offer to save globally
	save, err := prompt.Confirm("Save this token for future environments?", true)
	if err != nil {
		return "", "", err
	}
	if save {
		globalCfg.GitHubToken = token
		globalCfg.SSHKeyPath = "" // clear SSH key if switching to token
		if err := globalCfg.Save(); err != nil {
			fmt.Printf("%s Could not save to global config: %v\n", yellow("⚠"), err)
		} else {
			fmt.Printf("%s Token saved globally\n", green("✓"))
		}
	}

	fmt.Printf("\n%s Token configured for enterprise access\n", green("✓"))
	return token, "", nil
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

	if state.Enterprise {
		authMethod := "SSH Agent"
		if state.EnterpriseGitHubToken != "" {
			authMethod = "GitHub Token"
		} else if state.EnterpriseSSHKeyPath != "" {
			authMethod = fmt.Sprintf("SSH Key (%s)", state.EnterpriseSSHKeyPath)
		}
		fmt.Printf("  Enterprise:  %s (%s)\n", green("✓"), authMethod)
	}

	if len(state.AddonsPaths) > 0 {
		fmt.Printf("  Addons:      %d custom path(s)\n", len(state.AddonsPaths))
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. %s  # Build image and initialize database\n", cyan("odooctl docker run -i"))
	fmt.Printf("  2. %s   # View container status\n", cyan("odooctl docker status"))
}
