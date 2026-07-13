package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mart337i/odooctl/internal/config"
	modlib "github.com/mart337i/odooctl/internal/module"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagChangedJSON bool

type changedReport struct {
	New     []string `json:"new"`
	Changed []string `json:"changed"`
	Clean   bool     `json:"clean"`
}

var changedCmd = &cobra.Command{
	Use:   "changed",
	Short: "List local modules whose hashes changed since last install",
	RunE:  runChanged,
}

func init() {
	changedCmd.Flags().BoolVar(&flagChangedJSON, "json", false, "Print JSON output")
}

func runChanged(cmd *cobra.Command, args []string) error {
	state, err := loadModuleState()
	if err != nil {
		return err
	}
	modules, err := modlib.FindModules(state.ProjectRoot)
	if err != nil {
		return err
	}
	stored, _ := loadModuleHashes(state)
	var newModules, changedModules []string
	for _, name := range modules {
		hash, err := modlib.Hash(filepath.Join(state.ProjectRoot, name))
		if err != nil {
			return err
		}
		if stored[name] == "" {
			newModules = append(newModules, name)
		} else if stored[name] != hash {
			changedModules = append(changedModules, name)
		}
	}
	if flagChangedJSON {
		return output.PrintJSON(changedReport{New: newModules, Changed: changedModules, Clean: len(newModules) == 0 && len(changedModules) == 0})
	}
	if len(newModules) == 0 && len(changedModules) == 0 {
		fmt.Println("No local module changes detected")
		return nil
	}
	for _, name := range newModules {
		fmt.Printf("new     %s\n", name)
	}
	for _, name := range changedModules {
		fmt.Printf("changed %s\n", name)
	}
	return nil
}

func loadModuleHashes(state *config.State) (map[string]string, error) {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "module-hashes.json"))
	if err != nil {
		return map[string]string{}, err
	}
	var hashes map[string]string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, err
	}
	return hashes, nil
}
