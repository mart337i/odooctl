package mcp

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mart337i/odooctl/internal/browserruntime"
	mcpsrv "github.com/mart337i/odooctl/internal/mcp"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

func newBrowserCommand(version string) *cobra.Command {
	opts := mcpsrv.PlaywrightOptions{Project: "."}
	cmd := &cobra.Command{
		Use: "browser", Short: "Run inspection-only official Playwright MCP over stdio", Args: cobra.NoArgs, SilenceUsage: true,
		Long: "Use a prepared private browser runtime for development observation. Login defaults to admin/admin; ODOOCTL_MCP_BROWSER_LOGIN and ODOOCTL_MCP_BROWSER_PASSWORD override it. Use --public to skip authentication. Tool-requested navigation stays on the configured Odoo site; page redirects and external assets may use approved origins through Squid. No click, typing, upload or code-execution tools are exposed. Page loads can still have side effects. See docs/ai/PLAYWRIGHT_MCP.md.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Login, opts.Password = browserCredentials(!opts.Public)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			err := mcpsrv.ServePlaywright(ctx, opts, version)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.PersistentFlags().StringVar(&opts.Project, "project", ".", "Project directory with an existing running Odoo environment")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Skip authentication for public website observation")
	var origins []string
	setup := &cobra.Command{
		Use: "setup", Short: "Build the pinned browser and prepare its private proxy network", Args: cobra.NoArgs, SilenceUsage: true,
		Long: "Explicitly build the pinned official Playwright MCP and Squid images, then start only the browser proxy. Never creates, initializes, rebuilds or starts Odoo or its database. Approved origins are exact HTTPS DNS origins on port 443; other external and private destinations are denied.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			defer cancel()
			runtime, err := browserruntime.Prepare(ctx, opts.Project, origins)
			if err != nil {
				return err
			}
			return output.PrintJSON(runtime)
		},
	}
	setup.Flags().StringArrayVar(&origins, "allow-origin", nil, "Approve an exact external HTTPS origin for assets/frames (repeatable; no wildcard or arbitrary ports)")
	shutdown := &cobra.Command{
		Use: "stop", Short: "Stop the managed browser proxy; retain artifacts and Odoo data", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			if err := browserruntime.Stop(ctx, opts.Project); err != nil {
				return err
			}
			return output.PrintJSON(map[string]any{"stopped": true, "artifacts_retained": true})
		},
	}
	cmd.AddCommand(setup, shutdown)
	return cmd
}
