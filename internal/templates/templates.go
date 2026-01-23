package templates

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/egeskov/odooctl/internal/config"
)

//go:embed files/*
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

// Render generates all Docker files to the project directory
func Render(state *config.State) error {
	dir, err := config.ProjectDir(state.ProjectName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data := NewData(state)

	files := map[string]string{
		"docker-compose.yml": "files/docker-compose.yml.tmpl",
		"Dockerfile":         "files/Dockerfile.tmpl",
		"odoo.conf":          "files/odoo.conf.tmpl",
		"entrypoint.sh":      "files/entrypoint.sh.tmpl",
		"wait-for-psql.py":   "files/wait-for-psql.py.tmpl",
		".env":               "files/.env.tmpl",
		".dockerignore":      "files/.dockerignore.tmpl",
	}

	for outputName, tmplPath := range files {
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
