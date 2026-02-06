package docker

import (
	"fmt"

	"github.com/egeskov/odooctl/internal/docker"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagTestModules  string
	flagTestTags     string
	flagTestLogLevel string
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run Odoo tests",
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

  # Run with verbose output
  odooctl docker test --modules your_module --log-level=test:DEBUG`,
	RunE: runTest,
}

func init() {
	testCmd.Flags().StringVarP(&flagTestModules, "modules", "m", "", "Modules to test (comma-separated)")
	testCmd.Flags().StringVar(&flagTestTags, "test-tags", "", "Test filter tags: [-][tag][/module][:class][.method]")
	testCmd.Flags().StringVar(&flagTestLogLevel, "log-level", "", "Logging level (e.g., 'test:DEBUG', 'odoo.tests:DEBUG')")
}

func runTest(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

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
