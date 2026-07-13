package docker

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagFollow    bool
	flagLogTail   int
	flagLogJSON   bool
	flagLogGrep   string
	flagLogErrors bool
	flagLogSince  string
)

type logsReport struct {
	Service string `json:"service"`
	Tail    int    `json:"tail"`
	Since   string `json:"since,omitempty"`
	Grep    string `json:"grep,omitempty"`
	Errors  bool   `json:"errors"`
	Text    string `json:"text"`
}

var logsCmd = &cobra.Command{
	Use:          "logs [service]",
	Short:        "View container logs",
	SilenceUsage: true,
	Long: `Shows logs from Docker containers. Defaults to the odoo service.

Examples:
  odooctl docker logs             # Last 100 lines of odoo logs
  odooctl docker logs -f          # Follow odoo logs
  odooctl docker logs --tail 50   # Last 50 lines
  odooctl docker logs --errors    # Tracebacks and common Odoo errors
  odooctl docker logs --grep Traceback --since 10m
  odooctl docker logs db          # View database logs`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVar(&flagLogTail, "tail", 100, "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolVar(&flagLogJSON, "json", false, "Print JSON output (not compatible with --follow)")
	logsCmd.Flags().StringVar(&flagLogGrep, "grep", "", "Filter log lines containing text (case-insensitive)")
	logsCmd.Flags().BoolVar(&flagLogErrors, "errors", false, "Filter common Odoo error and traceback lines")
	logsCmd.Flags().StringVar(&flagLogSince, "since", "", "Show logs since a duration or timestamp, passed to docker compose logs")
}

func runLogs(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	service := "odoo"
	if len(args) > 0 {
		service = args[0]
	}
	filtering := flagLogJSON || flagLogGrep != "" || flagLogErrors
	if flagFollow && filtering {
		return fmt.Errorf("--follow cannot be used with --json, --grep, or --errors")
	}

	logArgs := []string{"logs"}
	if flagFollow {
		logArgs = append(logArgs, "-f")
	}
	if flagLogTail > 0 {
		logArgs = append(logArgs, "--tail", fmt.Sprintf("%d", flagLogTail))
	}
	if flagLogSince != "" {
		logArgs = append(logArgs, "--since", flagLogSince)
	}
	logArgs = append(logArgs, service)
	if filtering {
		text, err := docker.ComposeOutput(state, logArgs...)
		if err != nil {
			return err
		}
		text = filterLogText(text, flagLogGrep, flagLogErrors)
		if flagLogJSON {
			return output.PrintJSON(logsReport{Service: service, Tail: flagLogTail, Since: flagLogSince, Grep: flagLogGrep, Errors: flagLogErrors, Text: text})
		}
		fmt.Print(text)
		if !strings.HasSuffix(text, "\n") && text != "" {
			fmt.Println()
		}
		return nil
	}

	return docker.Compose(state, logArgs...)
}

func filterLogText(text, grep string, errorsOnly bool) string {
	if grep == "" && !errorsOnly {
		return text
	}
	grep = strings.ToLower(grep)
	patterns := []string{"traceback", "error", "critical", "psycopg2", "odoo.tools.convert", "parseerror", "accesserror", "validationerror"}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		if grep != "" && !strings.Contains(lower, grep) {
			continue
		}
		if errorsOnly && !containsAny(lower, patterns) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func containsAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}
