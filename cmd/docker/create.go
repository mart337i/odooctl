package docker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/project"
	"github.com/egeskov/odooctl/internal/templates"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagName        string
	flagOdooVersion string
	flagModules     string
	flagEnterprise  bool
	flagWithoutDemo bool
	flagPip         string
	flagForce       bool
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Docker development environment",
	Long:  `Generates Docker Compose, Dockerfile, and configuration files for Odoo development.`,
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&flagName, "name", "n", "", "Project name (default: directory name)")
	createCmd.Flags().StringVarP(&flagOdooVersion, "odoo-version", "v", "", "Odoo version (16.0, 17.0, 18.0, 19.0)")
	createCmd.Flags().StringVarP(&flagModules, "modules", "m", "", "Modules to install (comma-separated)")
	createCmd.Flags().BoolVarP(&flagEnterprise, "enterprise", "e", false, "Include Odoo Enterprise")
	createCmd.Flags().BoolVar(&flagWithoutDemo, "without-demo", false, "Initialize without demo data")
	createCmd.Flags().StringVarP(&flagPip, "pip", "p", "", "Extra pip packages (comma-separated)")
	createCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Overwrite existing configuration")
}

func runCreate(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Detect project context
	ctx := project.Detect(cwd)

	// Override with flags
	if flagName != "" {
		ctx.Name = flagName
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

	// Check if project already exists
	if !flagForce {
		dir, _ := config.ProjectDir(ctx.Name)
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("project %q already exists. Use --force to overwrite", ctx.Name)
		}
	}

	// Parse modules
	var modules []string
	if flagModules != "" {
		modules = strings.Split(flagModules, ",")
		for i := range modules {
			modules[i] = strings.TrimSpace(modules[i])
		}
	}

	// Parse pip packages
	var pipPkgs []string
	if flagPip != "" {
		pipPkgs = strings.Split(flagPip, ",")
		for i := range pipPkgs {
			pipPkgs[i] = strings.TrimSpace(pipPkgs[i])
		}
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
	fmt.Printf("  Odoo:        %s\n", cyan(state.OdooVersion))
	fmt.Printf("  Port:        %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)))
	fmt.Printf("  Mailhog:     %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)))

	dir, _ := config.ProjectDir(state.ProjectName)
	fmt.Printf("  Files:       %s\n", cyan(dir))

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. %s  # Initialize database and start containers\n", cyan("odooctl docker run"))
	fmt.Printf("  2. %s   # View container status\n", cyan("odooctl docker status"))
}
