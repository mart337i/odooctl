package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit [file]",
	Short: "Edit docker configuration files",
	Long: `Edit docker configuration files in your preferred editor.

Available files:
  config      - odoo.conf (Odoo configuration)
  dockerfile  - Dockerfile (container build)
  compose     - docker-compose.yml (services definition)
  env         - .env (environment variables)
  dockerignore - .dockerignore (build exclusions)

Examples:
  odooctl docker edit config      # Edit odoo.conf
  odooctl docker edit dockerfile  # Edit Dockerfile
  odooctl docker edit compose     # Edit docker-compose.yml`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEdit,
}

var filesMap = map[string]string{
	"config":       "odoo.conf",
	"dockerfile":   "Dockerfile",
	"compose":      "docker-compose.yml",
	"env":          ".env",
	"dockerignore": ".dockerignore",
}

func runEdit(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Default to config
	fileKey := "config"
	if len(args) > 0 {
		fileKey = strings.ToLower(args[0])
	}

	fileName, ok := filesMap[fileKey]
	if !ok {
		validKeys := make([]string, 0, len(filesMap))
		for k := range filesMap {
			validKeys = append(validKeys, k)
		}
		return fmt.Errorf("invalid file. Choose from: %s", strings.Join(validKeys, ", "))
	}

	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, fileName)

	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Get editor
	editor := getEditor()

	fmt.Printf("%s Opening %s in %s...\n", cyan("📝"), fileName, editor)

	editorCmd := exec.Command(editor, filePath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("error opening editor: %w", err)
	}

	fmt.Printf("%s File saved. Remember to rebuild if you edited the Dockerfile:\n", green("✓"))
	fmt.Printf("   %s\n", cyan("odooctl docker run --build"))

	return nil
}

func getEditor() string {
	// Check VISUAL first, then EDITOR, then fallback
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	// Try to find a common editor
	editors := []string{"code", "vim", "nvim", "nano", "vi"}
	for _, editor := range editors {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	return "vi" // Last resort
}
