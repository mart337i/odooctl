package ai

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var flagPromptDebugModule string

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Generate ready-to-paste AI prompts",
}

var promptDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Generate a debugging prompt",
	RunE:  runPromptDebug,
}

func init() {
	promptDebugCmd.Flags().StringVarP(&flagPromptDebugModule, "module", "m", "", "Module to debug")
	promptCmd.AddCommand(promptDebugCmd)
}

func runPromptDebug(cmd *cobra.Command, args []string) error {
	report, err := buildContextReport(flagPromptDebugModule)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("You are helping debug an Odoo module using odooctl.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Prefer safe `odooctl` commands and structured JSON output.\n")
	b.WriteString("- Do not suggest deleting Docker volumes, filestores, databases, or config unless explicitly approved.\n")
	b.WriteString("- Diagnose from manifest dependencies, Docker status, logs, and exact Odoo errors before proposing code changes.\n")
	b.WriteString("- If a command may touch the database, say so clearly.\n\n")
	b.WriteString("Task:\n")
	if flagPromptDebugModule != "" {
		b.WriteString(fmt.Sprintf("Diagnose why module `%s` fails to install, upgrade, test, or run.\n\n", flagPromptDebugModule))
	} else {
		b.WriteString("Diagnose the current Odoo development environment.\n\n")
	}
	b.WriteString("Context:\n")
	writeContextMarkdown(&b, report)
	b.WriteString("\nSuggested first commands if more evidence is needed:\n")
	for _, command := range uniqueStrings(report.SafeCommands) {
		b.WriteString(fmt.Sprintf("- `%s`\n", command))
	}
	if flagPromptDebugModule != "" {
		b.WriteString(fmt.Sprintf("- `odooctl ai debug-report --module %s --include-logs`\n", flagPromptDebugModule))
	}
	fmt.Print(b.String())
	return nil
}
