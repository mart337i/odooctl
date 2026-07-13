package odoo

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var evalCmd = &cobra.Command{
	Use:          "eval <python expression>",
	Short:        "Evaluate a Python expression in Odoo shell context",
	SilenceUsage: true,
	Long: `Evaluate a Python expression with Odoo shell variables available.

Examples:
  odooctl odoo eval "env['res.users'].search([]).mapped('login')"
  odooctl odoo eval "env['ir.module.module'].search([('name','=','sale')]).state"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEval,
}

func runEval(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	expr := strings.Join(args, " ")
	script := fmt.Sprintf("result = %s\nprint(result)\n", expr)
	_, err = runOdooShellScript(state, script, false)
	return err
}
