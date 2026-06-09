package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mart337i/odooctl/internal/config"
)

func TestRefreshStaleDockerfileRegeneratesSystemPipInstall(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	t.Setenv("HOME", home)

	state := &config.State{
		ProjectName: "test-project",
		OdooVersion: "19.0",
		Branch:      "main",
		ProjectRoot: projectRoot,
		Ports:       config.CalculatePorts("19.0"),
	}

	envDir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		t.Fatalf("EnvironmentDir() error = %v", err)
	}
	if err := os.MkdirAll(envDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	staleDockerfile := "RUN pip3 install --no-cache-dir --break-system-packages debugpy\n"
	if err := os.WriteFile(filepath.Join(envDir, "Dockerfile"), []byte(staleDockerfile), 0644); err != nil {
		t.Fatalf("WriteFile(Dockerfile) error = %v", err)
	}

	refreshed, err := refreshStaleDockerfile(state)
	if err != nil {
		t.Fatalf("refreshStaleDockerfile() error = %v", err)
	}
	if !refreshed {
		t.Fatal("refreshStaleDockerfile() refreshed = false, want true")
	}

	content, err := os.ReadFile(filepath.Join(envDir, "Dockerfile"))
	if err != nil {
		t.Fatalf("ReadFile(Dockerfile) error = %v", err)
	}

	dockerfile := string(content)
	for _, forbidden := range []string{"--break-system-packages", "RUN pip3 install"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Fatalf("Dockerfile still contains stale pattern %q", forbidden)
		}
	}
	if !strings.Contains(dockerfile, "/opt/odoo-venv/bin/pip install") {
		t.Fatal("Dockerfile missing venv pip install after refresh")
	}
}
