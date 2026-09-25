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
				ProjectName:         "test-project",
				OdooVersion:         version,
				OdooCommit:          strings.Repeat("a", 40),
				DockerSchemaVersion: 2,
				Branch:              strings.ReplaceAll(version, ".", ""),
				ProjectRoot:         home,
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
				"exec /opt/odoo-venv/bin/python3 /opt/odoo-src/odoo-bin \"$@\"",
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
				"name: test-project-" + strings.Replace(version, ".", "", 1),
				"ODOO_COMMIT: " + strings.Repeat("a", 40),
				"odoo-pydeps:",
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
				ProjectName:         "browser-project",
				OdooVersion:         version,
				OdooCommit:          strings.Repeat("b", 40),
				DockerSchemaVersion: 2,
				Branch:              strings.ReplaceAll(version, ".", ""),
				ProjectRoot:         home,
				BrowserEnabled:      true,
				BrowserProvider:     "playwright-chromium",
				Ports:               config.CalculatePorts(version),
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

func TestRenderLegacyGeventConstraint(t *testing.T) {
	for _, test := range []struct {
		version        string
		wantConstraint bool
	}{
		{version: "16.0", wantConstraint: true},
		{version: "17.0", wantConstraint: true},
		{version: "18.0", wantConstraint: false},
	} {
		t.Run(test.version, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			state := &config.State{
				ProjectName:         "gevent-project",
				OdooVersion:         test.version,
				OdooCommit:          strings.Repeat("c", 40),
				DockerSchemaVersion: 2,
				Branch:              strings.ReplaceAll(test.version, ".", ""),
				ProjectRoot:         home,
				Ports:               config.CalculatePorts(test.version),
			}
			if err := Render(state); err != nil {
				t.Fatal(err)
			}
			envDir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(envDir, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			containsLegacyBuild := strings.Contains(string(data), "pip install 'Cython<3'") && strings.Contains(string(data), "pip install --no-build-isolation")
			if containsLegacyBuild != test.wantConstraint {
				t.Fatalf("legacy gevent build setup = %v, want %v", containsLegacyBuild, test.wantConstraint)
			}
		})
	}
}

func TestRenderEnterpriseUsesLockedSourceAndBuildSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := &config.State{
		ProjectName:           "enterprise-project",
		OdooVersion:           "19.0",
		OdooCommit:            strings.Repeat("a", 40),
		EnterpriseCommit:      strings.Repeat("b", 40),
		DockerSchemaVersion:   2,
		Enterprise:            true,
		EnterpriseGitHubToken: "token-value",
		Branch:                "main",
		ProjectRoot:           home,
		Ports:                 config.CalculatePorts("19.0"),
	}
	if err := Render(state); err != nil {
		t.Fatal(err)
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
		"--mount=type=secret,id=github_token",
		"${ENTERPRISE_COMMIT}",
		"github.com/odoo/enterprise.git",
		"/opt/odoo-enterprise",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("Dockerfile missing Enterprise pattern %q", required)
		}
	}
	if strings.Contains(dockerfile, "token-value") {
		t.Fatal("Dockerfile contains the Enterprise token")
	}
	composeData, err := os.ReadFile(filepath.Join(envDir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(composeData)
	for _, required := range []string{"github_token:", "environment: GITHUB_TOKEN", "ENTERPRISE_COMMIT: " + strings.Repeat("b", 40)} {
		if !strings.Contains(compose, required) {
			t.Fatalf("docker-compose.yml missing Enterprise pattern %q", required)
		}
	}
}

func TestRenderScopesComposeResourcesPerEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	states := []*config.State{
		{ProjectName: "same-project", OdooVersion: "19.0", OdooCommit: strings.Repeat("a", 40), DockerSchemaVersion: 2, Branch: "main", ProjectRoot: home, Ports: config.CalculatePorts("19.0")},
		{ProjectName: "same-project", OdooVersion: "19.0", OdooCommit: strings.Repeat("a", 40), DockerSchemaVersion: 2, Branch: "feature", ProjectRoot: home, Ports: config.CalculatePorts("19.0")},
	}
	for _, state := range states {
		if err := Render(state); err != nil {
			t.Fatal(err)
		}
	}

	firstDir, _ := config.EnvironmentDir(states[0].ProjectName, states[0].Branch)
	secondDir, _ := config.EnvironmentDir(states[1].ProjectName, states[1].Branch)
	first, err := os.ReadFile(filepath.Join(firstDir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(secondDir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "name: same-project-feature-190") || strings.Contains(string(second), "name: same-project-main-190") {
		t.Fatal("Compose resources are not scoped to the environment")
	}
	if !strings.Contains(string(first), "name: same-project-main-190") || !strings.Contains(string(second), "name: same-project-feature-190") {
		t.Fatal("Compose project names do not include the branch")
	}
}

func TestRenderLegacyEnvironmentKeepsExistingVolumeNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := &config.State{
		ProjectName:         "legacy-project",
		OdooVersion:         "19.0",
		OdooCommit:          strings.Repeat("a", 40),
		DockerSchemaVersion: 1,
		Branch:              "main",
		ProjectRoot:         home,
		Ports:               config.CalculatePorts("19.0"),
	}
	if err := Render(state); err != nil {
		t.Fatal(err)
	}
	envDir, _ := config.EnvironmentDir(state.ProjectName, state.Branch)
	data, err := os.ReadFile(filepath.Join(envDir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(data)
	if strings.Contains(compose, "postgres-data:    name") {
		t.Fatal("legacy volume mapping was rendered on the same YAML line")
	}
	for _, required := range []string{
		"name: odoo-postgres-data-190",
		"name: odoo-filestore-190",
		"name: odoo-sessions-190",
		"name: odoo-pydeps-190",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("legacy Compose file missing volume mapping %q", required)
		}
	}
}
