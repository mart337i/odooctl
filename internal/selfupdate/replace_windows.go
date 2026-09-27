//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func replaceExecutable(target, replacement string) error {
	return replaceWindows(target, replacement)
}

func replaceWindows(target, replacement string) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$pidToWait = %d
$target = %s
$replacement = %s
Wait-Process -Id $pidToWait -ErrorAction SilentlyContinue
Move-Item -LiteralPath $replacement -Destination $target -Force
`, os.Getpid(), powershellString(target), powershellString(replacement))

	tmp, err := os.CreateTemp("", "odooctl-update-*.ps1")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(script); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path).Start()
}

func powershellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
