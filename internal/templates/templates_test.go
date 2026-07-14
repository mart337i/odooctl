package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mart337i/odooctl/internal/config"
)

func TestRenderUsesRuntimeVolumeForPipPackages(t *testing.T) {
	versions := []string{"12.0", "13.0", "14.0", "15.0", "16.0", "17.0", "18.0", "19.0"}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			state := &config.State{
				ProjectName: "test-project",
				OdooVersion: version,
				Branch:      strings.ReplaceAll(version, ".", ""),
				ProjectRoot: home,
				PipPackages: []string{
					"requests==2.31.0",
					"pandas>=2.0",
				},
				Ports: config.CalculatePorts(version),
			}

			if err := Render(state); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			envDir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
			if err != nil {
				t.Fatalf("EnvironmentDir() error = %v", err)
			}

			content, err := os.ReadFile(filepath.Join(envDir, "Dockerfile"))
			if err != nil {
				t.Fatalf("ReadFile(Dockerfile) error = %v", err)
			}

			dockerfile := string(content)
			for _, forbidden := range []string{"--break-system-packages", "RUN pip3 install"} {
				if strings.Contains(dockerfile, forbidden) {
					t.Fatalf("Dockerfile contains forbidden system pip install pattern %q", forbidden)
				}
			}

			for _, required := range []string{
				"python3-venv",
				"python3 -m venv --system-site-packages /opt/odoo-venv",
				"--mount=type=cache,target=/root/.cache/pip",
				"/opt/odoo-venv/bin/pip install",
				"/opt/odoo-extra-python",
				"exec /opt/odoo-venv/bin/python3 /usr/bin/odoo \"$@\"",
				"ENV PATH=\"/opt/odoo-venv/bin:${PATH}\"",
			} {
				if !strings.Contains(dockerfile, required) {
					t.Fatalf("Dockerfile missing required venv pattern %q", required)
				}
			}
			for _, runtimeOnly := range []string{"requests==2.31.0", "pandas>=2.0"} {
				if strings.Contains(dockerfile, runtimeOnly) {
					t.Fatalf("Dockerfile contains runtime pip package %q", runtimeOnly)
				}
			}

			composeContent, err := os.ReadFile(filepath.Join(envDir, "docker-compose.yml"))
			if err != nil {
				t.Fatalf("ReadFile(docker-compose.yml) error = %v", err)
			}
			compose := string(composeContent)
			for _, required := range []string{
				"PYTHONPATH: /opt/odoo-extra-python",
				"odoo-pydeps-" + strings.Replace(version, ".", "", 1),
				":/opt/odoo-extra-python",
			} {
				if !strings.Contains(compose, required) {
					t.Fatalf("docker-compose.yml missing runtime dependency pattern %q", required)
				}
			}
		})
	}
}

func TestRenderBrowserEnabledIncludesPlaywrightChromium(t *testing.T) {
	for _, version := range []string{"15.0", "16.0", "17.0", "18.0", "19.0"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			state := &config.State{
				ProjectName:     "browser-project",
				OdooVersion:     version,
				Branch:          strings.ReplaceAll(version, ".", ""),
				ProjectRoot:     home,
				BrowserEnabled:  true,
				BrowserProvider: "playwright-chromium",
				Ports:           config.CalculatePorts(version),
			}
			if err := Render(state); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			envDir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
			if err != nil {
				t.Fatal(err)
			}
			dockerfileData, err := os.ReadFile(filepath.Join(envDir, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			dockerfile := string(dockerfileData)
			for _, required := range []string{
				"playwright==1.49.1",
				"PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright",
				"CHROME_BIN=/usr/local/bin/chromium",
				"python3 -m playwright install --with-deps chromium",
				"/usr/local/bin/google-chrome",
			} {
				if !strings.Contains(dockerfile, required) {
					t.Fatalf("Dockerfile missing browser pattern %q", required)
				}
			}
			composeData, err := os.ReadFile(filepath.Join(envDir, "docker-compose.yml"))
			if err != nil {
				t.Fatal(err)
			}
			compose := string(composeData)
			for _, required := range []string{
				"PLAYWRIGHT_BROWSERS_PATH: /opt/ms-playwright",
				"CHROME_BIN: /usr/local/bin/chromium",
				"./browser-artifacts:/browser-artifacts",
			} {
				if !strings.Contains(compose, required) {
					t.Fatalf("docker-compose.yml missing browser pattern %q", required)
				}
			}
		})
	}
}
