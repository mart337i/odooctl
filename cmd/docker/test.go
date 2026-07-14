package docker

import (
	"fmt"

	"github.com/fatih/color"
	internalbrowser "github.com/mart337i/odooctl/internal/browser"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/pkg/prompt"
	"github.com/spf13/cobra"
)

var (
	flagTestModules  string
	flagTestTags     string
	flagTestLogLevel string
	flagTestWeb      bool
)

var testCmd = &cobra.Command{
	Use:          "test",
	Short:        "Run Odoo tests",
	SilenceUsage: true,
	Long: `Run Odoo tests with advanced filtering.

Examples:
  # Run only post_install tests
  odooctl docker test --modules your_module --test-tags post_install

  # Run specific test tags
  odooctl docker test --modules your_module --test-tags standard

  # Exclude certain tags
  odooctl docker test --modules your_module --test-tags 'standard,-slow'

  # Run tests from specific module only
  odooctl docker test --test-tags /stock_account

  # Combine tags (runs tests with ANY of these tags)
  odooctl docker test --test-tags 'nice,standard'

  # Run specific test class
  odooctl docker test --test-tags /your_module:YourTestClass

  # Run specific test method
  odooctl docker test --test-tags .test_specific_method_name

  # Full specification
  odooctl docker test --test-tags /account:TestAccountInvoice.test_supplier_invoice

  # Run Odoo browser/web tests after checking Chromium availability
  odooctl docker test --web --test-tags /web

  # Run with verbose output
  odooctl docker test --modules your_module --log-level=test:DEBUG`,
	RunE: runTest,
}

func init() {
	testCmd.Flags().StringVarP(&flagTestModules, "modules", "m", "", "Modules to test (comma-separated)")
	testCmd.Flags().StringVar(&flagTestTags, "test-tags", "", "Test filter tags: [-][tag][/module][:class][.method]")
	testCmd.Flags().StringVar(&flagTestLogLevel, "log-level", "", "Logging level (e.g., 'test:DEBUG', 'odoo.tests:DEBUG')")
	testCmd.Flags().BoolVar(&flagTestWeb, "web", false, "Run browser readiness check first and default tags to /web")
}

func runTest(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if err := ensureDockerProjectAccess(state); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	if flagTestWeb {
		check := internalbrowser.CheckRuntime(state)
		if !check.CanLaunch {
			return fmt.Errorf("browser runtime is not ready: %s", check.Error)
		}
		if flagTestTags == "" {
			flagTestTags = "/web"
		}
		fmt.Printf("%s Browser runtime ready (%s)\n", cyan("🌐"), check.PlaywrightVersion)
	}

	// Build odoo-bin command
	database := state.DBName()

	testArgs := []string{
		"run", "--rm", "odoo",
		"odoo", "-c", "/etc/odoo/odoo.conf",
		"-d", database,
		"--test-enable",
	}

	if flagTestTags != "" {
		testArgs = append(testArgs, "--test-tags", flagTestTags)
		fmt.Printf("%s Running tests with tags: %s\n", cyan("🧪"), flagTestTags)
	}

	if flagTestLogLevel != "" {
		testArgs = append(testArgs, "--log-level", flagTestLogLevel)
		fmt.Printf("%s Log level: %s\n", cyan("📝"), flagTestLogLevel)
	}

	if flagTestModules != "" {
		if flagTestTags == "" {
			// Without test-tags, we need to install modules
			testArgs = append(testArgs, "-i", flagTestModules)
			fmt.Printf("%s Testing modules: %s\n", cyan("📦"), flagTestModules)
		} else {
			fmt.Printf("%s Module context: %s\n", cyan("📦"), flagTestModules)
		}
	}

	// Warn if no modules or tags specified
	if flagTestModules == "" && flagTestTags == "" {
		fmt.Printf("%s No modules or test-tags specified. This will run ALL tests!\n", color.YellowString("⚠️"))
		confirmed, err := prompt.Confirm("Continue?", false)
		if err != nil || !confirmed {
			fmt.Println("Test cancelled.")
			return nil
		}
	}

	testArgs = append(testArgs, "--stop-after-init")

	fmt.Println()
	if err := docker.Compose(state, testArgs...); err != nil {
		fmt.Printf("\n%s Tests failed!\n", red("✗"))
		return fmt.Errorf("tests failed: %w", err)
	}

	fmt.Printf("\n%s Tests completed!\n", green("✓"))
	return nil
}
