package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/egeskov/odooctl/internal/config"
)

//go:embed files/* files/12.0/* files/13.0/* files/14.0/* files/15.0/* files/16.0/* files/17.0/*
var templateFS embed.FS

// Data holds template rendering context
type Data struct {
	ProjectName   string
	OdooVersion   string
	VersionSuffix string
	DBName        string
	ProjectRoot   string
	InitModules   string
	WithoutDemo   bool
	Enterprise    bool
	PipPackages   string
	AddonsPaths   []string
	Ports         config.Ports
}

// NewData creates template data from state
func NewData(state *config.State) Data {
	versionSuffix := strings.Replace(state.OdooVersion, ".", "", 1)
	dbName := "odoo-" + versionSuffix

	modules := []string{"base", "web"}
	modules = append(modules, state.Modules...)

	pipPkgs := ""
	if len(state.PipPackages) > 0 {
		pipPkgs = strings.Join(state.PipPackages, " \\\n    ")
	}

	return Data{
		ProjectName:   state.ProjectName,
		OdooVersion:   state.OdooVersion,
		VersionSuffix: versionSuffix,
		DBName:        dbName,
		ProjectRoot:   state.ProjectRoot,
		InitModules:   strings.Join(modules, ","),
		WithoutDemo:   state.WithoutDemo,
		Enterprise:    state.Enterprise,
		PipPackages:   pipPkgs,
		AddonsPaths:   state.AddonsPaths,
		Ports:         state.Ports,
	}
}

// getTemplatePath returns the version-specific template path if it exists,
// otherwise returns the base template path
func getTemplatePath(version, filename string) string {
	// Check for version-specific template first
	versionPath := fmt.Sprintf("files/%s/%s", version, filename)
	if _, err := templateFS.ReadFile(versionPath); err == nil {
		return versionPath
	}
	// Fall back to base template
	return fmt.Sprintf("files/%s", filename)
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
