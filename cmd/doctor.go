package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/diagnostics"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagDoctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the current odooctl environment",
	Long:  `Checks project state, Docker access, Compose services, environment files, and Python dependency state.`,
	RunE:  runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&flagDoctorJSON, "json", false, "Print JSON output")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	report := diagnostics.Collect(cwd)
	if flagDoctorJSON {
		return output.PrintJSON(report)
	}
	printDoctorReport(report)
	return nil
}

func printDoctorReport(report diagnostics.Report) {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	status := green("OK")
	if report.Status == diagnostics.StatusWarning {
		status = yellow("WARNING")
	} else if report.Status == diagnostics.StatusError {
		status = red("ERROR")
	}
	fmt.Printf("odooctl doctor: %s\n\n", status)

	if report.Project != nil {
		fmt.Printf("Project: %s\n", cyan(report.Project.Name))
		fmt.Printf("Odoo:    %s\n", cyan(report.Project.OdooVersion))
		fmt.Printf("Root:    %s\n", report.Project.Root)
		fmt.Printf("Branch:  %s\n\n", report.Project.Branch)
	}

	for _, check := range report.Checks {
		marker := green("✓")
		if check.Status == diagnostics.StatusWarning {
			marker = yellow("!")
		} else if check.Status == diagnostics.StatusError {
			marker = red("✗")
		}
		fmt.Printf("%s %-24s %s\n", marker, check.Name, check.Message)
		if check.Detail != "" {
			fmt.Printf("  %s\n", strings.ReplaceAll(check.Detail, "\n", "\n  "))
		}
	}

	if report.Docker.OdooURL != "" || report.Docker.MailHogURL != "" {
		fmt.Println("\nAccess:")
		if report.Docker.OdooURL != "" {
			fmt.Printf("  Odoo:    %s\n", cyan(report.Docker.OdooURL))
		}
		if report.Docker.MailHogURL != "" {
			fmt.Printf("  MailHog: %s\n", cyan(report.Docker.MailHogURL))
		}
	}

	if len(report.NextSteps) > 0 {
		fmt.Println("\nNext steps:")
		for _, step := range uniqueStrings(report.NextSteps) {
			fmt.Printf("  - %s\n", step)
		}
	}

	if len(report.SafeCommands) > 0 {
		fmt.Println("\nSafe commands for AI/automation:")
		for _, command := range uniqueStrings(report.SafeCommands) {
			fmt.Printf("  %s\n", cyan(command))
		}
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
