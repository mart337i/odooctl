package odoo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagModuleStateJSON bool

type moduleState struct {
	Name             string `json:"name"`
	State            string `json:"state"`
	LatestVersion    string `json:"latest_version,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
}

var moduleStateCmd = &cobra.Command{
	Use:          "module-state <module...>",
	Short:        "Show installed/available state for Odoo modules",
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runModuleState,
}

func init() {
	moduleStateCmd.Flags().BoolVar(&flagModuleStateJSON, "json", false, "Print JSON output")
}

func runModuleState(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	namesJSON, err := json.Marshal(args)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`import json
names = %s
records = env['ir.module.module'].search([('name', 'in', names)])
by_name = {record.name: {
    'name': record.name,
    'state': record.state,
    'latest_version': record.latest_version or '',
    'installed_version': record.installed_version or '',
} for record in records}
print(json.dumps([by_name.get(name, {'name': name, 'state': 'not_found'}) for name in names]))
`, string(namesJSON))
	text, err := runOdooShellScript(state, script, true)
	if err != nil {
		return fmt.Errorf("failed to inspect module state: %w", err)
	}
	states, err := parseModuleStateOutput(text)
	if err != nil {
		return err
	}
	if flagModuleStateJSON {
		return output.PrintJSON(states)
	}
	for _, state := range states {
		fmt.Printf("%-32s %s\n", state.Name, state.State)
	}
	return nil
}

func parseModuleStateOutput(text string) ([]moduleState, error) {
	text = strings.TrimSpace(text)
	idx := strings.LastIndex(text, "[")
	if idx >= 0 {
		text = text[idx:]
	}
	var states []moduleState
	if err := json.Unmarshal([]byte(text), &states); err != nil {
		return nil, fmt.Errorf("failed to parse module-state output: %w\n%s", err, text)
	}
	return states, nil
}
