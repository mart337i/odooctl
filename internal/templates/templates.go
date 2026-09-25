package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/mart337i/odooctl/internal/config"
)

//go:embed files/*.tmpl
var templateFS embed.FS

// Data holds template rendering context
type Data struct {
	ProjectName            string
	EnvironmentName        string
	OdooVersion            string
	OdooCommit             string
	EnterpriseCommit       string
	BaseImage              string
	VersionSuffix          string
	DBName                 string
	ProjectRoot            string
	InitModules            string
	WithoutDemo            bool
	Enterprise             bool
	EnterpriseGitHubToken  string
	EnterpriseSSHKeyPath   string
	AddonsPaths            []string
	Ports                  config.Ports
	BrowserEnabled         bool
	BrowserProvider        string
	IncludeLxmlHTMLClean   bool
	LegacyGeventBuild      bool
	DebugpyVersion         string
	IPythonVersion         string
	InitDemoFlag           string
	LegacyResources        bool
	UseAptOdooDependencies bool
}

// NewData creates template data from state
func NewData(state *config.State) Data {
	versionSuffix := strings.Replace(state.OdooVersion, ".", "", 1)
	dbName := "odoo-" + versionSuffix
	environmentName := config.SanitizeName(strings.ToLower(fmt.Sprintf("%s-%s-%s", state.ProjectName, state.Branch, versionSuffix)))
	baseImage := "ubuntu:noble"
	debugpyVersion := "1.8.14"
	ipythonVersion := "8.27.0"
	if state.OdooVersion == "12.0" || state.OdooVersion == "13.0" {
		baseImage = "debian:buster-slim"
		debugpyVersion = "1.6.7"
		ipythonVersion = "7.34.0"
	} else if state.OdooVersion == "14.0" {
		baseImage = "ubuntu:focal"
		debugpyVersion = "1.8.1"
		ipythonVersion = "8.12.3"
	} else if state.OdooVersion == "15.0" || state.OdooVersion == "16.0" || state.OdooVersion == "17.0" {
		baseImage = "ubuntu:jammy"
		debugpyVersion = "1.8.1"
		ipythonVersion = "8.12.3"
	}
	initDemoFlag := ""
	if isVersion19OrHigher(state.OdooVersion) {
		if !state.WithoutDemo {
			initDemoFlag = "--with-demo"
		}
	} else if state.WithoutDemo {
		initDemoFlag = "--without-demo=all"
	}

	modules := []string{"base", "web"}
	modules = append(modules, state.Modules...)

	return Data{
		ProjectName:            state.ProjectName,
		EnvironmentName:        environmentName,
		OdooVersion:            state.OdooVersion,
		OdooCommit:             state.OdooCommit,
		EnterpriseCommit:       state.EnterpriseCommit,
		BaseImage:              baseImage,
		VersionSuffix:          versionSuffix,
		DBName:                 dbName,
		ProjectRoot:            state.ProjectRoot,
		InitModules:            strings.Join(modules, ","),
		WithoutDemo:            state.WithoutDemo,
		Enterprise:             state.Enterprise,
		EnterpriseGitHubToken:  state.EnterpriseGitHubToken,
		EnterpriseSSHKeyPath:   state.EnterpriseSSHKeyPath,
		AddonsPaths:            state.AddonsPaths,
		Ports:                  state.Ports,
		BrowserEnabled:         state.BrowserEnabled,
		BrowserProvider:        state.BrowserProvider,
		IncludeLxmlHTMLClean:   state.OdooVersion != "12.0" && state.OdooVersion != "13.0",
		LegacyGeventBuild:      state.OdooVersion == "15.0" || state.OdooVersion == "16.0" || state.OdooVersion == "17.0",
		DebugpyVersion:         debugpyVersion,
		IPythonVersion:         ipythonVersion,
		InitDemoFlag:           initDemoFlag,
		LegacyResources:        state.DockerSchemaVersion == 1,
		UseAptOdooDependencies: state.OdooVersion == "12.0" || state.OdooVersion == "13.0",
	}
}

// getTemplatePath returns the version-specific template path if it exists,
// otherwise returns the base template path. For v19+, it falls back to 19.0 templates
// to ensure proper demo data handling (inverted behavior in v19+).
func getTemplatePath(version, filename string) string {
	// Dockerfiles are generated from one source-build template. The Odoo
	// version only changes its rendered build arguments and base image.
	if filename == "Dockerfile.tmpl" {
		return "files/Dockerfile.tmpl"
	}
	// Check for exact version-specific template first
	versionPath := fmt.Sprintf("files/%s/%s", version, filename)
	if _, err := templateFS.ReadFile(versionPath); err == nil {
		return versionPath
	}

	// For v19+, fall back to 19.0 template if it exists (handles demo inversion)
	if isVersion19OrHigher(version) {
		v19Path := fmt.Sprintf("files/19.0/%s", filename)
		if _, err := templateFS.ReadFile(v19Path); err == nil {
			return v19Path
		}
	}

	// Fall back to base template
	return fmt.Sprintf("files/%s", filename)
}

// isVersion19OrHigher checks if the version is 19.0 or higher
func isVersion19OrHigher(version string) bool {
	// Extract major version (e.g., "19.0" -> 19, "20.0" -> 20)
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return major >= 19
}

// Render generates all Docker files to the environment directory
func Render(state *config.State) error {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if state.BrowserEnabled {
		if err := os.MkdirAll(filepath.Join(dir, "browser-artifacts"), 0755); err != nil {
			return err
		}
	}

	data := NewData(state)

	// Map of output filename to template filename
	templateFiles := []string{
		"docker-compose.yml.tmpl",
		"Dockerfile.tmpl",
		"odoo.conf.tmpl",
		"entrypoint.sh.tmpl",
		"wait-for-psql.py.tmpl",
		".env.tmpl",
		".dockerignore.tmpl",
	}

	for _, tmplFilename := range templateFiles {
		// Get version-specific or base template path
		tmplPath := getTemplatePath(state.OdooVersion, tmplFilename)
		// Output filename removes .tmpl suffix
		outputName := strings.TrimSuffix(tmplFilename, ".tmpl")
		if err := renderFile(dir, outputName, tmplPath, data); err != nil {
			return err
		}
	}

	return nil
}

func renderFile(dir, outputName, tmplPath string, data Data) error {
	content, err := templateFS.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	tmpl, err := template.New(outputName).Parse(string(content))
	if err != nil {
		return err
	}

	outputPath := filepath.Join(dir, outputName)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	// Make scripts executable
	if strings.HasSuffix(outputName, ".sh") || strings.HasSuffix(outputName, ".py") {
		if err := os.Chmod(outputPath, 0755); err != nil {
			return err
		}
	}

	return nil
}
