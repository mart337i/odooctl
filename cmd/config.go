package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global odooctl configuration",
	Long: `Manage global settings shared across all environments.

Available keys:
  ssh-key-path    Path to your SSH private key (e.g. ~/.ssh/id_ed25519)
  github-token    GitHub Personal Access Token for Odoo Enterprise access

Examples:
  odooctl config show                          # Show all saved settings
  odooctl config set ssh-key-path ~/.ssh/id_ed25519
  odooctl config set github-token ghp_xxxxx
  odooctl config get ssh-key-path
  odooctl config unset github-token`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigUnset,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show all configuration values",
	Args:  cobra.NoArgs,
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	switch key {
	case "ssh-key-path":
		// Expand ~ and validate the path exists
		expanded, err := expandPath(value)
		if err != nil {
			return err
		}
		if _, err := os.Stat(expanded); err != nil {
			return fmt.Errorf("SSH key file not found: %s", expanded)
		}
		cfg.SSHKeyPath = expanded
		fmt.Printf("%s ssh-key-path set to: %s\n", color.GreenString("✓"), expanded)

	case "github-token":
		token := strings.TrimSpace(value)
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") {
			fmt.Printf("%s Token doesn't match expected format (ghp_ or github_pat_), saving anyway\n", color.YellowString("⚠"))
		}
		cfg.GitHubToken = token
		fmt.Printf("%s github-token saved\n", color.GreenString("✓"))

	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: ssh-key-path, github-token", key)
	}

	return cfg.Save()
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	switch key {
	case "ssh-key-path":
		if cfg.SSHKeyPath == "" {
			fmt.Println("(not set)")
		} else {
			fmt.Println(cfg.SSHKeyPath)
		}
	case "github-token":
		if cfg.GitHubToken == "" {
			fmt.Println("(not set)")
		} else {
			fmt.Println(maskToken(cfg.GitHubToken))
		}
	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: ssh-key-path, github-token", key)
	}

	return nil
}

func runConfigUnset(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	switch key {
	case "ssh-key-path":
		cfg.SSHKeyPath = ""
	case "github-token":
		cfg.GitHubToken = ""
	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: ssh-key-path, github-token", key)
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("%s %s unset\n", color.GreenString("✓"), key)
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	configPath, _ := config.GlobalConfigPath()
	fmt.Printf("\n%s Global configuration (%s)\n\n", green("⚙"), configPath)

	if cfg.SSHKeyPath == "" {
		fmt.Printf("  ssh-key-path:  %s\n", yellow("(not set)"))
	} else {
		fmt.Printf("  ssh-key-path:  %s\n", cyan(cfg.SSHKeyPath))
	}

	if cfg.GitHubToken == "" {
		fmt.Printf("  github-token:  %s\n", yellow("(not set)"))
	} else {
		fmt.Printf("  github-token:  %s\n", cyan(maskToken(cfg.GitHubToken)))
	}

	fmt.Println()
	return nil
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

// maskToken shows only the prefix and last 4 chars
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	// Show prefix (ghp_ or github_pat_) + enough to identify, mask the rest
	prefixEnd := 4
	if strings.HasPrefix(token, "github_pat_") {
		prefixEnd = 11
	}
	visible := token[:prefixEnd]
	last4 := token[len(token)-4:]
	masked := len(token) - prefixEnd - 4
	return visible + strings.Repeat("*", masked) + last4
}
