package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mart337i/odooctl/internal/config"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
)

const ProviderPlaywrightChromium = "playwright-chromium"
const ContainerArtifactsDir = "/browser-artifacts"
const ChromeBin = "/usr/local/bin/chromium"
const PlaywrightBrowsersPath = "/opt/ms-playwright"

type Info struct {
	Enabled                bool   `json:"enabled"`
	Supported              bool   `json:"supported"`
	Provider               string `json:"provider,omitempty"`
	ArtifactsDir           string `json:"artifacts_dir,omitempty"`
	ContainerArtifactsDir  string `json:"container_artifacts_dir,omitempty"`
	ChromeBin              string `json:"chrome_bin,omitempty"`
	PlaywrightBrowsersPath string `json:"playwright_browsers_path,omitempty"`
}

type RuntimeCheck struct {
	Info              Info   `json:"info"`
	PlaywrightVersion string `json:"playwright_version,omitempty"`
	ChromiumPath      string `json:"chromium_path,omitempty"`
	CanLaunch         bool   `json:"can_launch"`
	Error             string `json:"error,omitempty"`
}

func SupportsVersion(version string) bool {
	major, err := majorVersion(version)
	return err == nil && major >= 15
}

func StaticInfo(state *config.State) Info {
	info := Info{
		Enabled:                state.BrowserEnabled,
		Supported:              SupportsVersion(state.OdooVersion),
		Provider:               state.BrowserProvider,
		ContainerArtifactsDir:  ContainerArtifactsDir,
		ChromeBin:              ChromeBin,
		PlaywrightBrowsersPath: PlaywrightBrowsersPath,
	}
	if info.Provider == "" && state.BrowserEnabled {
		info.Provider = ProviderPlaywrightChromium
	}
	if dir, err := ArtifactsDir(state); err == nil {
		info.ArtifactsDir = dir
	}
	return info
}

func EnsureSupported(state *config.State) error {
	if !SupportsVersion(state.OdooVersion) {
		return fmt.Errorf("browser tooling is supported for Odoo 15.0+ environments; current version is %s", state.OdooVersion)
	}
	return nil
}

func ArtifactsDir(state *config.State) (string, error) {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "browser-artifacts"), nil
}

func EnsureArtifactsDir(state *config.State) (string, error) {
	dir, err := ArtifactsDir(state)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		return "", err
	}
	_ = os.Chmod(dir, 0777)
	return dir, nil
}

func EnsureContainerArtifactsDir(state *config.State) error {
	cmd := dockerlib.ComposeCommand(state, "exec", "-T", "--user", "root", "odoo", "sh", "-c", "mkdir -p /browser-artifacts && chmod 777 /browser-artifacts")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to prepare browser artifact directory: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func CheckRuntime(state *config.State) RuntimeCheck {
	check := RuntimeCheck{Info: StaticInfo(state)}
	if !state.BrowserEnabled {
		check.Error = "browser tooling is not enabled; run 'odooctl docker reconfigure --browser --rebuild'"
		return check
	}
	if err := EnsureSupported(state); err != nil {
		check.Error = err.Error()
		return check
	}
	output, err := RunPythonScriptOneOff(state, runtimeCheckScript)
	if err != nil {
		check.Error = strings.TrimSpace(output)
		if check.Error == "" {
			check.Error = err.Error()
		}
		return check
	}
	jsonOutput := ExtractJSONOutput(output)
	if err := json.Unmarshal([]byte(jsonOutput), &check); err != nil {
		check.Error = fmt.Sprintf("failed to parse browser runtime check: %v: %s", err, strings.TrimSpace(output))
		return check
	}
	check.Info = StaticInfo(state)
	return check
}

func RunPythonScript(state *config.State, script string) (string, error) {
	cmd := dockerlib.ComposeCommand(state, "exec", "-T", "odoo", "/opt/odoo-venv/bin/python3", "-")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func RunPythonScriptOneOff(state *config.State, script string) (string, error) {
	cmd := dockerlib.ComposeCommand(state, "run", "--rm", "--no-deps", "odoo", "/opt/odoo-venv/bin/python3", "-")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func ExtractJSONOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "{") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return trimmed
}

func majorVersion(version string) (int, error) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid Odoo version %q", version)
	}
	return strconv.Atoi(parts[0])
}

const runtimeCheckScript = `import asyncio
import importlib.metadata
import json

from playwright.async_api import async_playwright

async def main():
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True, args=["--no-sandbox", "--disable-dev-shm-usage"])
        await browser.close()
        print(json.dumps({
            "playwright_version": importlib.metadata.version("playwright"),
            "chromium_path": p.chromium.executable_path,
            "can_launch": True,
        }))

asyncio.run(main())
`
