package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show docker environment location and status",
	Long:  `Display the location and status of the Docker environment files.`,
	RunE:  runPath,
}

func runPath(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	dir, err := config.ProjectDir(state.ProjectName)
	if err != nil {
		return err
	}

	fmt.Printf("%s Location: %s\n", cyan("📁"), dir)
	fmt.Printf("%s Version:  Odoo %s\n", cyan("🔢"), state.OdooVersion)
	fmt.Printf("%s Ports:    Odoo=%d, MailHog=%d, Debug=%d\n",
		cyan("🌐"), state.Ports.Odoo, state.Ports.Mailhog, state.Ports.Debug)

	if state.Enterprise {
		fmt.Printf("%s Edition:  Enterprise\n", cyan("🏢"))
	}

	// Check if docker files exist
	files := []string{"docker-compose.yml", "Dockerfile", "odoo.conf"}
	allExist := true
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(dir, file)); os.IsNotExist(err) {
			allExist = false
			break
		}
	}

	if allExist {
		entries, _ := os.ReadDir(dir)
		fmt.Printf("\n%s %d files ready\n", green("✓"), len(entries))
	} else {
		fmt.Printf("\n%s Not fully initialized - run 'odooctl docker create'\n", yellow("⚠️"))
	}

	// Show addons paths if configured
	if len(state.AddonsPaths) > 0 {
		fmt.Printf("\n%s Addons paths:\n", cyan("📦"))
		for i, path := range state.AddonsPaths {
			fmt.Printf("   %d. %s\n", i+1, path)
		}
	}

	return nil
}
