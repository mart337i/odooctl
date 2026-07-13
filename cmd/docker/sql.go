package docker

import (
	"fmt"
	"os"
	"strings"

	dockerlib "github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var (
	flagSQLDatabase string
	flagSQLFile     string
	flagSQLJSON     bool
)

var sqlCmd = &cobra.Command{
	Use:          "sql [query]",
	Short:        "Run a SQL query against the Odoo database",
	SilenceUsage: true,
	Long: `Run quick SQL without opening an interactive psql session.

Examples:
  odooctl docker sql "select id, login from res_users"
  odooctl docker sql --json "select id, name from ir_module_module"
  odooctl docker sql --file debug.sql`,
	Args: cobra.ArbitraryArgs,
	RunE: runSQL,
}

func init() {
	sqlCmd.Flags().StringVarP(&flagSQLDatabase, "database", "d", "", "Database name (auto-detected if omitted)")
	sqlCmd.Flags().StringVarP(&flagSQLFile, "file", "f", "", "Read SQL from a file")
	sqlCmd.Flags().BoolVar(&flagSQLJSON, "json", false, "Wrap a SELECT query and print JSON rows")
}

func runSQL(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	database := flagSQLDatabase
	if database == "" {
		database = state.DBName()
	}
	query, err := sqlQuery(args, flagSQLFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("SQL query is required")
	}
	if flagSQLJSON {
		wrapped := fmt.Sprintf("SELECT COALESCE(json_agg(row_to_json(q)), '[]'::json) FROM (%s) q", strings.TrimRight(strings.TrimSpace(query), ";"))
		text, err := dockerlib.ComposeOutput(state, "exec", "-T", "db", "psql", "-U", "odoo", "-d", database, "-t", "-A", "-c", wrapped)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(text))
		return nil
	}
	return dockerlib.Compose(state, "exec", "-T", "db", "psql", "-U", "odoo", "-d", database, "-c", query)
}

func sqlQuery(args []string, file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return strings.Join(args, " "), nil
}
