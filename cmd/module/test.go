package module

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var flagModuleTestTags string

var testCmd = &cobra.Command{
	Use:          "test <module...>",
	Short:        "Run Odoo tests for modules",
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runModuleTest,
}

func init() {
	testCmd.Flags().StringVar(&flagModuleTestTags, "test-tags", "", "Override Odoo test tags")
}

func runModuleTest(cmd *cobra.Command, args []string) error {
	state, err := loadModuleState()
	if err != nil {
		return err
	}
	if err := docker.CheckDaemon(); err != nil {
		return err
	}
	if err := docker.CheckBindMount(state.ProjectRoot); err != nil {
		return err
	}
	tags := flagModuleTestTags
	if tags == "" {
		prefixed := make([]string, len(args))
		for i, module := range args {
			prefixed[i] = "/" + module
		}
		tags = strings.Join(prefixed, ",")
	}
	runArgs := []string{
		"run", "--rm", "odoo",
		"odoo", "-c", "/etc/odoo/odoo.conf",
		"-d", state.DBName(),
		"--test-enable",
		"--test-tags", tags,
		"--stop-after-init",
	}
	fmt.Printf("Running tests with tags: %s\n", tags)
	return docker.Compose(state, runArgs...)
}
