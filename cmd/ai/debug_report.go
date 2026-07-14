package ai

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	internalbrowser "github.com/mart337i/odooctl/internal/browser"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

type DebugReport struct {
	Context      ContextReport                 `json:"context"`
	Logs         []LogSection                  `json:"logs,omitempty"`
	LogError     string                        `json:"log_error,omitempty"`
	BrowserCheck *internalbrowser.RuntimeCheck `json:"browser_check,omitempty"`
	Commands     []string                      `json:"commands"`
}

type LogSection struct {
	Service string `json:"service"`
	Lines   int    `json:"lines"`
	Text    string `json:"text"`
}

var (
	flagDebugModule         string
	flagDebugIncludeLogs    bool
	flagDebugIncludeBrowser bool
	flagDebugLogLines       int
	flagDebugOutput         string
	flagDebugFormat         string
)

var debugReportCmd = &cobra.Command{
	Use:   "debug-report",
	Short: "Print a redacted AI-ready debugging report",
	Long:  `Prints environment diagnostics, module context, dependency state, and optional recent logs for AI-assisted debugging.`,
	RunE:  runDebugReport,
}

func init() {
	debugReportCmd.Flags().StringVarP(&flagDebugModule, "module", "m", "", "Focus report on one module")
	debugReportCmd.Flags().BoolVar(&flagDebugIncludeLogs, "include-logs", false, "Include recent Docker logs")
	debugReportCmd.Flags().BoolVar(&flagDebugIncludeBrowser, "include-browser", false, "Include Playwright Chromium runtime check")
	debugReportCmd.Flags().IntVar(&flagDebugLogLines, "log-lines", 200, "Log lines per service when --include-logs is set")
	debugReportCmd.Flags().StringVarP(&flagDebugOutput, "output", "o", "", "Write report to a file instead of stdout")
	debugReportCmd.Flags().StringVar(&flagDebugFormat, "format", "markdown", "Output format: markdown or json")
}

func runDebugReport(cmd *cobra.Command, args []string) error {
	report, err := buildDebugReport(flagDebugModule, flagDebugIncludeLogs, flagDebugIncludeBrowser, flagDebugLogLines)
	if err != nil {
		return err
	}

	var content string
	switch flagDebugFormat {
	case "json":
		if flagDebugOutput == "" {
			return output.PrintJSON(report)
		}
		data, err := marshalJSON(report)
		if err != nil {
			return err
		}
		content = string(data)
	case "markdown", "md", "":
		content = renderDebugReportMarkdown(report)
	default:
		return fmt.Errorf("unsupported --format %q (supported: markdown, json)", flagDebugFormat)
	}

	if flagDebugOutput != "" {
		return os.WriteFile(flagDebugOutput, []byte(content), 0644)
	}
	fmt.Print(content)
	return nil
}

func buildDebugReport(moduleName string, includeLogs, includeBrowser bool, logLines int) (DebugReport, error) {
	contextReport, err := buildContextReport(moduleName)
	if err != nil {
		return DebugReport{}, err
	}
	report := DebugReport{
		Context: contextReport,
		Commands: []string{
			"odooctl doctor --json",
			"odooctl docker status --json",
			"odooctl module list --json",
		},
	}
	if moduleName != "" {
		report.Commands = append(report.Commands,
			fmt.Sprintf("odooctl module manifest %s --json", moduleName),
			fmt.Sprintf("odooctl module deps %s --json", moduleName),
			fmt.Sprintf("odooctl docker install %s --list-only --json", moduleName),
		)
	}
	if !includeLogs || contextReport.Project == nil {
		if includeBrowser {
			attachBrowserCheck(&report)
		}
		return report, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return report, err
	}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return report, nil
	}
	logs, err := collectLogs(state, logLines)
	if err != nil {
		report.LogError = err.Error()
	}
	report.Logs = logs
	if includeBrowser {
		attachBrowserCheck(&report)
	}
	return report, nil
}

func attachBrowserCheck(report *DebugReport) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return
	}
	check := internalbrowser.CheckRuntime(state)
	report.BrowserCheck = &check
}

func collectLogs(state *config.State, lines int) ([]LogSection, error) {
	if lines <= 0 {
		lines = 200
	}
	var sections []LogSection
	var logErrors []string
	for _, service := range []string{"odoo", "db"} {
		text, err := docker.ComposeOutput(state, "logs", "--no-color", "--tail", strconv.Itoa(lines), service)
		if text != "" {
			sections = append(sections, LogSection{Service: service, Lines: lines, Text: Redact(text)})
		}
		if err != nil {
			logErrors = append(logErrors, fmt.Sprintf("%s: %v", service, err))
		}
	}
	if len(logErrors) > 0 {
		return sections, errors.New(strings.Join(logErrors, "; "))
	}
	return sections, nil
}

func renderDebugReportMarkdown(report DebugReport) string {
	var b strings.Builder
	b.WriteString("# odooctl AI Debug Report\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", report.Context.GeneratedAt.Format(time.RFC3339)))
	writeContextMarkdown(&b, report.Context)
	if len(report.Logs) > 0 {
		b.WriteString("\n## Logs\n")
		for _, section := range report.Logs {
			b.WriteString(fmt.Sprintf("\n### %s logs\n", section.Service))
			b.WriteString("```text\n")
			b.WriteString(section.Text)
			if !strings.HasSuffix(section.Text, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("```\n")
		}
	}
	if report.LogError != "" {
		b.WriteString("\n## Log Collection Error\n")
		b.WriteString("```text\n")
		b.WriteString(report.LogError)
		b.WriteString("\n```\n")
	}
	if report.BrowserCheck != nil {
		b.WriteString("\n## Browser Runtime\n")
		b.WriteString(fmt.Sprintf("- Enabled: `%t`\n", report.BrowserCheck.Info.Enabled))
		b.WriteString(fmt.Sprintf("- Can launch: `%t`\n", report.BrowserCheck.CanLaunch))
		if report.BrowserCheck.PlaywrightVersion != "" {
			b.WriteString(fmt.Sprintf("- Playwright: `%s`\n", report.BrowserCheck.PlaywrightVersion))
		}
		if report.BrowserCheck.ChromiumPath != "" {
			b.WriteString(fmt.Sprintf("- Chromium: `%s`\n", report.BrowserCheck.ChromiumPath))
		}
		if report.BrowserCheck.Error != "" {
			b.WriteString(fmt.Sprintf("- Error: `%s`\n", report.BrowserCheck.Error))
		}
	}
	if len(report.Commands) > 0 {
		b.WriteString("\n## Useful Commands\n")
		for _, command := range report.Commands {
			b.WriteString(fmt.Sprintf("- `%s`\n", command))
		}
	}
	return b.String()
}

func writeContextMarkdown(b *strings.Builder, report ContextReport) {
	if report.Project == nil {
		b.WriteString("## Project\nNo odooctl environment was found for the current directory.\n")
		return
	}
	b.WriteString("## Project\n")
	b.WriteString(fmt.Sprintf("- Name: `%s`\n", report.Project.Name))
	b.WriteString(fmt.Sprintf("- Odoo version: `%s`\n", report.Project.OdooVersion))
	b.WriteString(fmt.Sprintf("- Branch/environment: `%s`\n", report.Project.Branch))
	b.WriteString(fmt.Sprintf("- Root: `%s`\n", report.Project.Root))
	if report.Environment != nil {
		b.WriteString(fmt.Sprintf("- Env dir: `%s`\n", report.Environment.Dir))
	}
	if report.Module != nil {
		b.WriteString("\n## Module\n")
		b.WriteString(fmt.Sprintf("- Module: `%s`\n", report.Module.Module))
		b.WriteString(fmt.Sprintf("- Name: `%s`\n", valueOrDash(report.Module.Name)))
		b.WriteString(fmt.Sprintf("- Path: `%s`\n", report.Module.Path))
		b.WriteString(fmt.Sprintf("- Depends: `%s`\n", joinOrDash(report.Module.Depends)))
		b.WriteString(fmt.Sprintf("- Python deps: `%s`\n", joinOrDash(report.Module.ExternalPython)))
	}
	if report.PythonDeps != nil {
		b.WriteString("\n## Python Dependencies\n")
		b.WriteString(fmt.Sprintf("- Configured: `%s`\n", joinOrDash(report.PythonDeps.Configured)))
		b.WriteString(fmt.Sprintf("- Missing: `%s`\n", joinOrDash(report.PythonDeps.Missing)))
	}
	if len(report.Problems) > 0 {
		b.WriteString("\n## Problems\n")
		for _, problem := range uniqueStrings(report.Problems) {
			b.WriteString(fmt.Sprintf("- `%s`\n", problem))
		}
	}
	if len(report.NextSteps) > 0 {
		b.WriteString("\n## Next Steps\n")
		for _, step := range uniqueStrings(report.NextSteps) {
			b.WriteString(fmt.Sprintf("- `%s`\n", step))
		}
	}
}
