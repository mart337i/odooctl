package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mart337i/odooctl/internal/config"
)

func TestBrowserScriptsUseIsolatedPython(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake Docker fixture")
	}
	t.Setenv("HOME", t.TempDir())
	state := &config.State{ProjectName: "browser", Branch: "main", OdooVersion: "19.0", BrowserEnabled: true}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
    "compose version"|"compose ls") exit 0 ;;
    "compose exec -T odoo /opt/odoo-browser-venv/bin/python3 -"|"compose run --rm --no-deps odoo /opt/odoo-browser-venv/bin/python3 -")
        /bin/cat >/dev/null
        printf '%s\n' '{"can_launch":true,"playwright_version":"1.49.1"}' ;;
    *) exit 11 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, run := range []func(*config.State, string) (string, error){RunPythonScript, RunPythonScriptOneOff} {
		if output, err := run(state, "print('test')"); err != nil {
			t.Fatalf("browser script failed: %v\n%s", err, output)
		}
	}
	check := CheckRuntime(state)
	if check.Error != "" || !check.CanLaunch || check.PlaywrightVersion != "1.49.1" {
		t.Fatalf("CheckRuntime() = %+v", check)
	}
}

func TestSupportsVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"14.0", false},
		{"15.0", true},
		{"19.0", true},
		{"bad", false},
	} {
		if got := SupportsVersion(tc.version); got != tc.want {
			t.Fatalf("SupportsVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestExtractJSONOutputSkipsComposeStatusLines(t *testing.T) {
	output := "Container odoo-app Running\n Container project-odoo-run Creating\n{\"can_launch\":true}\n"
	got := ExtractJSONOutput(output)
	if got != `{"can_launch":true}` {
		t.Fatalf("ExtractJSONOutput() = %q", got)
	}
}
