package module

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagMigrateFrom string
	flagMigrateTo   string
	flagMigrateJSON bool
)

type migratePlanReport struct {
	Module string   `json:"module"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
	Items  []string `json:"items"`
}

type migrateScaffoldReport struct {
	Module       string   `json:"module"`
	MigrationDir string   `json:"migration_dir"`
	Created      []string `json:"created"`
	Existing     []string `json:"existing"`
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Plan or scaffold module migrations",
}

var migratePlanCmd = &cobra.Command{
	Use:   "plan <module>",
	Short: "Print a migration checklist for a module",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigratePlan,
}

var migrateScaffoldCmd = &cobra.Command{
	Use:   "scaffold <module>",
	Short: "Create migration script skeletons for a module",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateScaffold,
}

func init() {
	migratePlanCmd.Flags().StringVar(&flagMigrateFrom, "from", "", "Source Odoo version")
	migratePlanCmd.Flags().StringVar(&flagMigrateTo, "to", "", "Target Odoo version")
	migratePlanCmd.Flags().BoolVar(&flagMigrateJSON, "json", false, "Print JSON output")
	migrateScaffoldCmd.Flags().StringVar(&flagMigrateTo, "to", "", "Target module migration version directory")
	migrateScaffoldCmd.Flags().BoolVar(&flagMigrateJSON, "json", false, "Print JSON output")
	migrateCmd.AddCommand(migratePlanCmd)
	migrateCmd.AddCommand(migrateScaffoldCmd)
}

func runMigratePlan(cmd *cobra.Command, args []string) error {
	moduleName := args[0]
	items := migrationChecklist()
	if flagMigrateJSON {
		return output.PrintJSON(migratePlanReport{Module: moduleName, From: flagMigrateFrom, To: flagMigrateTo, Items: items})
	}
	fmt.Printf("Migration plan for %s\n", moduleName)
	if flagMigrateFrom != "" || flagMigrateTo != "" {
		fmt.Printf("Version: %s -> %s\n", valueOrDash(flagMigrateFrom), valueOrDash(flagMigrateTo))
	}
	for _, item := range items {
		fmt.Printf("- %s\n", item)
	}
	return nil
}

func runMigrateScaffold(cmd *cobra.Command, args []string) error {
	if flagMigrateTo == "" {
		return fmt.Errorf("--to is required")
	}
	dirs, _, err := moduleScanDirs()
	if err != nil {
		return err
	}
	moduleDir, ok := findModuleDir(args[0], dirs)
	if !ok {
		return fmt.Errorf("module %q not found", args[0])
	}
	migrationDir := filepath.Join(moduleDir, "migrations", flagMigrateTo)
	if err := os.MkdirAll(migrationDir, 0755); err != nil {
		return err
	}
	report := migrateScaffoldReport{Module: args[0], MigrationDir: migrationDir}
	for _, name := range []string{"pre-migration.py", "post-migration.py"} {
		path := filepath.Join(migrationDir, name)
		if _, err := os.Stat(path); err == nil {
			report.Existing = append(report.Existing, path)
			continue
		}
		content := []byte("def migrate(env, version):\n    pass\n")
		if err := os.WriteFile(path, content, 0644); err != nil {
			return err
		}
		report.Created = append(report.Created, path)
	}
	if flagMigrateJSON {
		return output.PrintJSON(report)
	}
	fmt.Printf("Created migration skeletons in %s\n", migrationDir)
	return nil
}

func migrationChecklist() []string {
	return []string{
		"Review manifest depends and external_dependencies",
		"Run module deps to find missing module and Python dependencies",
		"Check XML views for version-specific tags such as tree/list",
		"Check model overrides, decorators, constraints, and removed ORM APIs",
		"Run module tests and docker install after each migration step",
		"Add pre/post migration scripts only when data changes require them",
	}
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
