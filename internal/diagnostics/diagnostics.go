package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	internalbrowser "github.com/mart337i/odooctl/internal/browser"
	"github.com/mart337i/odooctl/internal/config"
	pydeps "github.com/mart337i/odooctl/internal/deps"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
	modlib "github.com/mart337i/odooctl/internal/module"
)

type CheckStatus string

const (
	StatusOK      CheckStatus = "ok"
	StatusWarning CheckStatus = "warning"
	StatusError   CheckStatus = "error"
)

type Check struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message"`
	Detail  string      `json:"detail,omitempty"`
}

type ProjectInfo struct {
	Name        string `json:"name"`
	Root        string `json:"root"`
	Branch      string `json:"branch"`
	OdooVersion string `json:"odoo_version"`
	Database    string `json:"database"`
	IsGitRepo   bool   `json:"is_git_repo"`
}

type EnvironmentInfo struct {
	Dir            string       `json:"dir"`
	StateFile      string       `json:"state_file"`
	Ports          config.Ports `json:"ports"`
	FilesPresent   []string     `json:"files_present"`
	FilesMissing   []string     `json:"files_missing"`
	AddonsPaths    []string     `json:"addons_paths"`
	ConfiguredMods []string     `json:"configured_modules"`
}

type ServiceStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

type DockerInfo struct {
	CLIPath       string          `json:"cli_path,omitempty"`
	Context       string          `json:"context,omitempty"`
	DaemonOK      bool            `json:"daemon_ok"`
	BindMountOK   bool            `json:"bind_mount_ok"`
	Services      []ServiceStatus `json:"services,omitempty"`
	ServiceError  string          `json:"service_error,omitempty"`
	OdooURL       string          `json:"odoo_url,omitempty"`
	MailHogURL    string          `json:"mailhog_url,omitempty"`
	DebugEndpoint string          `json:"debug_endpoint,omitempty"`
}

type PythonDepsInfo struct {
	Configured []string            `json:"configured"`
	Discovered map[string][]string `json:"discovered"`
	Missing    []string            `json:"missing"`
	Synced     bool                `json:"synced"`
	SyncedAt   *time.Time          `json:"synced_at,omitempty"`
}

type Report struct {
	GeneratedAt  time.Time             `json:"generated_at"`
	OK           bool                  `json:"ok"`
	Status       CheckStatus           `json:"status"`
	Project      *ProjectInfo          `json:"project,omitempty"`
	Environment  *EnvironmentInfo      `json:"environment,omitempty"`
	Docker       DockerInfo            `json:"docker"`
	PythonDeps   *PythonDepsInfo       `json:"python_deps,omitempty"`
	Browser      *internalbrowser.Info `json:"browser,omitempty"`
	Checks       []Check               `json:"checks"`
	Problems     []string              `json:"problems"`
	NextSteps    []string              `json:"next_steps"`
	SafeCommands []string              `json:"safe_commands"`
}

func Collect(cwd string) Report {
	report := Report{GeneratedAt: time.Now(), Status: StatusOK}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		report.add(Check{ID: "environment", Name: "Environment", Status: StatusError, Message: "No odooctl environment found", Detail: err.Error()})
		report.NextSteps = append(report.NextSteps, "Run 'odooctl docker create' from the project root")
		report.SafeCommands = append(report.SafeCommands, "odooctl docker create")
		report.finalize()
		return report
	}

	report.Project = &ProjectInfo{
		Name:        state.ProjectName,
		Root:        state.ProjectRoot,
		Branch:      state.Branch,
		OdooVersion: state.OdooVersion,
		Database:    state.DBName(),
		IsGitRepo:   state.IsGitRepo,
	}
	report.add(Check{ID: "environment", Name: "Environment", Status: StatusOK, Message: "Environment state loaded"})

	envDir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		report.add(Check{ID: "environment_dir", Name: "Environment directory", Status: StatusError, Message: "Failed to resolve environment directory", Detail: err.Error()})
	} else {
		report.Environment = collectEnvironment(state, envDir)
		if len(report.Environment.FilesMissing) == 0 {
			report.add(Check{ID: "environment_files", Name: "Environment files", Status: StatusOK, Message: "Required environment files exist"})
		} else {
			report.add(Check{ID: "environment_files", Name: "Environment files", Status: StatusError, Message: "Required environment files are missing", Detail: strings.Join(report.Environment.FilesMissing, ", ")})
			report.NextSteps = append(report.NextSteps, "Run 'odooctl docker create' or inspect the environment directory")
		}
	}

	report.collectDocker(state)
	browserInfo := internalbrowser.StaticInfo(state)
	report.Browser = &browserInfo
	if state.BrowserEnabled && !browserInfo.Supported {
		report.add(Check{ID: "browser", Name: "Browser tooling", Status: StatusWarning, Message: "Browser tooling is enabled for an unsupported Odoo version", Detail: state.OdooVersion})
	} else if state.BrowserEnabled {
		report.add(Check{ID: "browser", Name: "Browser tooling", Status: StatusOK, Message: "Browser tooling is enabled"})
	} else {
		report.add(Check{ID: "browser", Name: "Browser tooling", Status: StatusOK, Message: "Browser tooling is disabled", Detail: "Optional. Run 'odooctl docker reconfigure --browser --rebuild' to enable Playwright Chromium"})
	}
	report.PythonDeps = collectPythonDeps(state)
	if report.PythonDeps != nil && len(report.PythonDeps.Missing) > 0 {
		report.add(Check{ID: "python_deps", Name: "Python dependencies", Status: StatusWarning, Message: "Manifest Python dependencies are not synced", Detail: strings.Join(report.PythonDeps.Missing, ", ")})
		report.NextSteps = append(report.NextSteps, "Run 'odooctl docker deps sync' or 'odooctl docker install <module>'")
	} else {
		report.add(Check{ID: "python_deps", Name: "Python dependencies", Status: StatusOK, Message: "No missing manifest Python dependencies found"})
	}

	report.SafeCommands = append(report.SafeCommands,
		"odooctl doctor --json",
		"odooctl ai context --format json",
		"odooctl module list --json",
		"odooctl docker status --json",
	)
	report.finalize()
	return report
}

func collectEnvironment(state *config.State, envDir string) *EnvironmentInfo {
	info := &EnvironmentInfo{
		Dir:            envDir,
		StateFile:      filepath.Join(envDir, config.StateFileName),
		Ports:          state.Ports,
		AddonsPaths:    append([]string{}, state.AddonsPaths...),
		ConfiguredMods: append([]string{}, state.Modules...),
	}
	for _, name := range []string{config.StateFileName, "docker-compose.yml", "Dockerfile", "odoo.conf"} {
		path := filepath.Join(envDir, name)
		if _, err := os.Stat(path); err == nil {
			info.FilesPresent = append(info.FilesPresent, name)
		} else {
			info.FilesMissing = append(info.FilesMissing, name)
		}
	}
	return info
}

func (r *Report) collectDocker(state *config.State) {
	cliPath, err := exec.LookPath("docker")
	if err != nil {
		r.add(Check{ID: "docker_cli", Name: "Docker CLI", Status: StatusError, Message: "Docker CLI was not found", Detail: err.Error()})
		r.NextSteps = append(r.NextSteps, "Install Docker Desktop or Docker Engine")
		return
	}
	r.Docker.CLIPath = cliPath
	r.add(Check{ID: "docker_cli", Name: "Docker CLI", Status: StatusOK, Message: "Docker CLI found", Detail: cliPath})

	if context, err := commandOutput("docker", "context", "show"); err == nil {
		r.Docker.Context = context
	}

	if err := dockerlib.CheckDaemon(); err != nil {
		r.add(Check{ID: "docker_daemon", Name: "Docker daemon", Status: StatusError, Message: "Docker daemon is not reachable", Detail: err.Error()})
		r.NextSteps = append(r.NextSteps, "Start Docker Desktop or Docker Engine, then rerun 'odooctl doctor'")
		return
	}
	r.Docker.DaemonOK = true
	r.add(Check{ID: "docker_daemon", Name: "Docker daemon", Status: StatusOK, Message: "Docker daemon is reachable"})

	if err := dockerlib.CheckBindMount(state.ProjectRoot); err != nil {
		r.add(Check{ID: "docker_bind_mount", Name: "Docker bind mount", Status: StatusError, Message: "Docker cannot access project files", Detail: err.Error()})
		r.NextSteps = append(r.NextSteps, "Enable Docker Desktop WSL integration/file sharing for this distro")
		return
	}
	r.Docker.BindMountOK = true
	r.add(Check{ID: "docker_bind_mount", Name: "Docker bind mount", Status: StatusOK, Message: "Docker can access project files"})

	services, err := dockerlib.GetServicesStatus(state)
	if err != nil {
		r.Docker.ServiceError = err.Error()
		r.add(Check{ID: "docker_services", Name: "Docker services", Status: StatusWarning, Message: "Could not read Compose service status", Detail: err.Error()})
		return
	}
	for _, svc := range services {
		r.Docker.Services = append(r.Docker.Services, ServiceStatus{Name: svc.Name, State: svc.State, Status: svc.Status, Ports: svc.Ports})
		if svc.State == "running" && svc.Name == "odoo" {
			r.Docker.OdooURL = fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)
			r.Docker.DebugEndpoint = fmt.Sprintf("localhost:%d", state.Ports.Debug)
		}
		if svc.State == "running" && svc.Name == "mailhog" {
			r.Docker.MailHogURL = fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)
		}
	}
	if len(services) == 0 {
		r.add(Check{ID: "docker_services", Name: "Docker services", Status: StatusWarning, Message: "No Compose services found"})
		r.NextSteps = append(r.NextSteps, "Run 'odooctl docker run --build -i' to start and initialize the environment")
		return
	}
	r.add(Check{ID: "docker_services", Name: "Docker services", Status: StatusOK, Message: "Compose service status read"})
}

func collectPythonDeps(state *config.State) *PythonDepsInfo {
	dirs := []string{state.ProjectRoot}
	dirs = append(dirs, state.AddonsPaths...)
	discovered := pydeps.DiscoverPythonDepsForModules(dirs, nil)
	missing := pydeps.MissingPythonDeps(discovered, state.PipPackages)
	return &PythonDepsInfo{
		Configured: append([]string{}, state.PipPackages...),
		Discovered: discovered,
		Missing:    missing,
		Synced:     len(state.PipPackages) == 0 || state.PythonDepsHash == pythonDepsHash(state.PipPackages),
		SyncedAt:   state.PythonDepsSyncedAt,
	}
}

func (r *Report) add(check Check) {
	r.Checks = append(r.Checks, check)
	if check.Status == StatusError || check.Status == StatusWarning {
		problem := check.Message
		if check.Detail != "" {
			problem += ": " + check.Detail
		}
		r.Problems = append(r.Problems, problem)
	}
}

func (r *Report) finalize() {
	r.OK = true
	r.Status = StatusOK
	for _, check := range r.Checks {
		if check.Status == StatusError {
			r.OK = false
			r.Status = StatusError
			return
		}
		if check.Status == StatusWarning {
			r.Status = StatusWarning
		}
	}
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func pythonDepsHash(packages []string) string {
	packages = append([]string{}, packages...)
	for i, pkg := range packages {
		packages[i] = strings.ToLower(strings.TrimSpace(pkg))
	}
	sort.Strings(packages)
	hash := sha256.Sum256([]byte(strings.Join(packages, "\n")))
	return hex.EncodeToString(hash[:])
}

func FindModuleManifests(state *config.State, targets []string) ([]modlib.ManifestInfo, error) {
	dirs := []string{state.ProjectRoot}
	dirs = append(dirs, state.AddonsPaths...)
	targetSet := make(map[string]bool)
	for _, target := range targets {
		if target = strings.TrimSpace(target); target != "" {
			targetSet[target] = true
		}
	}

	manifests := []modlib.ManifestInfo{}
	seen := make(map[string]bool)
	for _, dir := range dirs {
		modules, err := modlib.FindModules(dir)
		if err != nil {
			continue
		}
		for _, name := range modules {
			if len(targetSet) > 0 && !targetSet[name] {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			manifest, err := modlib.ParseManifest(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, manifest)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Module < manifests[j].Module })
	return manifests, nil
}
