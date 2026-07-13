package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagConfigJSON bool

type globalConfigReport struct {
	SSHKeyPath  string `json:"ssh_key_path"`
	GitHubToken string `json:"github_token"`
}

type configValueReport struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type configMutationReport struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
	Set   bool   `json:"set"`
}

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
  odooctl config set github-token <token>
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
	configSetCmd.Flags().BoolVar(&flagConfigJSON, "json", false, "Print JSON output")
	configGetCmd.Flags().BoolVar(&flagConfigJSON, "json", false, "Print JSON output")
	configUnsetCmd.Flags().BoolVar(&flagConfigJSON, "json", false, "Print JSON output")
	configShowCmd.Flags().BoolVar(&flagConfigJSON, "json", false, "Print JSON output")
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
		expanded, err := config.ExpandPath(value)
		if err != nil {
			return err
		}
		if _, err := os.Stat(expanded); err != nil {
			return fmt.Errorf("SSH key file not found: %s", expanded)
		}
		cfg.SSHKeyPath = expanded
		if !flagConfigJSON {
			fmt.Printf("%s ssh-key-path set to: %s\n", color.GreenString("✓"), expanded)
		}

	case "github-token":
		token := strings.TrimSpace(value)
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") && !flagConfigJSON {
			fmt.Printf("%s Token doesn't match expected format (ghp_ or github_pat_), saving anyway\n", color.YellowString("⚠"))
		}
		cfg.GitHubToken = token
		if !flagConfigJSON {
			fmt.Printf("%s github-token saved\n", color.GreenString("✓"))
		}

	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: ssh-key-path, github-token", key)
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	if flagConfigJSON {
		return output.PrintJSON(configMutationReport{Key: key, Value: configValueForKey(cfg, key), Set: true})
	}
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	switch key {
	case "ssh-key-path":
		if flagConfigJSON {
			return output.PrintJSON(configValueReport{Key: key, Value: cfg.SSHKeyPath})
		}
		if cfg.SSHKeyPath == "" {
			fmt.Println("(not set)")
		} else {
			fmt.Println(cfg.SSHKeyPath)
		}
	case "github-token":
		if flagConfigJSON {
			return output.PrintJSON(configValueReport{Key: key, Value: configValueForKey(cfg, key)})
		}
		if cfg.GitHubToken == "" {
			fmt.Println("(not set)")
		} else {
			fmt.Println(config.MaskToken(cfg.GitHubToken))
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
	if flagConfigJSON {
		return output.PrintJSON(configMutationReport{Key: key, Set: false})
	}
	fmt.Printf("%s %s unset\n", color.GreenString("✓"), key)
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if flagConfigJSON {
		return output.PrintJSON(globalConfigReport{SSHKeyPath: cfg.SSHKeyPath, GitHubToken: configValueForKey(cfg, "github-token")})
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
		fmt.Printf("  github-token:  %s\n", cyan(config.MaskToken(cfg.GitHubToken)))
	}

	fmt.Println()
	return nil
}

func configValueForKey(cfg *config.GlobalConfig, key string) string {
	switch key {
	case "ssh-key-path":
		return cfg.SSHKeyPath
	case "github-token":
		if cfg.GitHubToken == "" {
			return ""
		}
		return config.MaskToken(cfg.GitHubToken)
	default:
		return ""
	}
}
