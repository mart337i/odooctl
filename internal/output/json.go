package output

import (
	"encoding/json"
	"fmt"
)

// PrintJSON writes deterministic, indented JSON for CLI commands.
func PrintJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
