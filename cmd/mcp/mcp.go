package mcp

import (
	"os"
	"os/signal"
	"syscall"

	mcpsrv "github.com/mart337i/odooctl/internal/mcp"
	"github.com/spf13/cobra"
)

func NewCommand(version string) *cobra.Command {
	options := mcpsrv.Options{}
	cmd := &cobra.Command{Use: "mcp", Short: "Expose project-scoped inspection tools to AI clients"}
	serve := &cobra.Command{
		Use: "serve", Short: "Run a local MCP server over stdio", Args: cobra.NoArgs, SilenceUsage: true,
		Long: "Run inspection-only MCP tools. Runtime tools never start containers. Credentials are read from server environment variables, not tool arguments. See docs/ai/MCP.md.",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.BrowserLogin = os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN")
			options.BrowserPassword = os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD")
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return mcpsrv.Serve(ctx, options, version)
		},
	}
	serve.Flags().StringVar(&options.Project, "project", ".", "Project directory (fixed for the server lifetime)")
	serve.Flags().StringVar(&options.SQLRole, "sql-role", "", "Dedicated non-privileged PostgreSQL login role; enables SQL tools")
	serve.Flags().StringSliceVar(&options.SQLTables, "sql-tables", nil, "Allowlisted public tables, comma-separated; required with --sql-role")
	serve.Flags().IntVar(&options.OdooUserID, "odoo-user-id", 0, "Non-superuser Odoo ID; enables ORM tools with --models")
	serve.Flags().StringSliceVar(&options.Models, "models", nil, "Allowlisted Odoo model names, comma-separated")
	serve.Flags().IntSliceVar(&options.CompanyIDs, "company-ids", nil, "Selectable Odoo company context IDs, not row isolation (default: user's main company)")
	serve.Flags().BoolVar(&options.BrowserEnabled, "browser", false, "Enable authenticated browser observation using server environment credentials")
	serve.Flags().BoolVar(&options.ContainerSource, "container-source", false, "Allow code lookup in running container core/Enterprise source roots")
	cmd.AddCommand(serve)
	return cmd
}
