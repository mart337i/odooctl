package prompt

import (
	"github.com/AlecAivazis/survey/v2"
)

// SelectVersion prompts user to select an Odoo version
func SelectVersion() (string, error) {
	versions := []string{"19.0", "18.0", "17.0", "16.0"}
	var selected string

	prompt := &survey.Select{
		Message: "Select Odoo version:",
		Options: versions,
		Default: "18.0",
	}

	err := survey.AskOne(prompt, &selected)
	return selected, err
}

// InputString prompts for text input
func InputString(message, defaultVal string) (string, error) {
	var result string
	prompt := &survey.Input{
		Message: message,
		Default: defaultVal,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// Confirm prompts for yes/no
func Confirm(message string, defaultVal bool) (bool, error) {
	var result bool
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultVal,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}
