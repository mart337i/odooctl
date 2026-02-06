package docker

import (
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var flagDatabase string

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Open PostgreSQL shell",
	Long:  `Opens an interactive PostgreSQL shell connected to the Odoo database.`,
	RunE:  runDB,
}

func init() {
	dbCmd.Flags().StringVarP(&flagDatabase, "database", "d", "", "Database name (auto-detected if omitted)")
}

func runDB(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	database := flagDatabase
	if database == "" {
		database = state.DBName()
	}

	return docker.Compose(state, "exec", "db", "psql", "-U", "odoo", "-d", database)
}
