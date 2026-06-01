package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egeskov/odooctl/internal/config"
)

func TestRenderDockerfileUsesVenvForPipPackages(t *testing.T) {
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
				"/opt/odoo-venv/bin/pip install --no-cache-dir",
				"requests==2.31.0",
				"pandas>=2.0",
				"exec /opt/odoo-venv/bin/python3 /usr/bin/odoo \"$@\"",
				"ENV PATH=\"/opt/odoo-venv/bin:${PATH}\"",
			} {
				if !strings.Contains(dockerfile, required) {
					t.Fatalf("Dockerfile missing required venv pattern %q", required)
				}
			}
		})
	}
}
