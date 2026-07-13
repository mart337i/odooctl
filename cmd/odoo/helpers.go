package odoo

import (
	"fmt"
	"os"
	"strings"

	"github.com/mart337i/odooctl/internal/config"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
)

func loadState() (*config.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, fmt.Errorf("no Docker environment found. Run 'odooctl docker create' first")
	}
	return state, nil
}

func runOdooShellScript(state *config.State, script string, capture bool) (string, error) {
	cmd := dockerlib.ComposeCommand(state, "exec", "-T", "odoo", "odoo", "shell", "-d", state.DBName(), "--log-level=critical")
	cmd.Stdin = strings.NewReader(script)
	if capture {
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return "", cmd.Run()
}
