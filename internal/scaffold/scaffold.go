package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed files/*
var templateFS embed.FS

// ModuleConfig holds configuration for module generation
type ModuleConfig struct {
	Name        string
	Author      string
	Version     string
	Depends     []string
	Description string
	WithModel   bool
}

// TemplateData is passed to templates
type TemplateData struct {
	ModuleName  string
	ModelName   string
	ClassName   string
	Author      string
	Version     string
	Depends     string
	Description string
	HasModels   bool
	UseListTag  bool // true for Odoo 18+
}

// CreateModule creates a new Odoo module directory with files
func CreateModule(dir string, config ModuleConfig) error {
	// Create directory structure
	dirs := []string{
		dir,
		filepath.Join(dir, "static"),
		filepath.Join(dir, "data"),
		filepath.Join(dir, "security"),
	}

	if config.WithModel {
		dirs = append(dirs, filepath.Join(dir, "models"))
		dirs = append(dirs, filepath.Join(dir, "views"))
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	// Create .gitkeep files
	for _, d := range []string{"static", "data"} {
		path := filepath.Join(dir, d, ".gitkeep")
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			return err
		}
	}

	// Prepare template data
	data := TemplateData{
		ModuleName:  config.Name,
		ModelName:   strings.ReplaceAll(config.Name, "_", "."),
		ClassName:   toPascal(config.Name),
		Author:      config.Author,
		Version:     config.Version,
		Depends:     formatDepends(config.Depends),
		Description: config.Description,
		HasModels:   config.WithModel,
		UseListTag:  isVersion18OrHigher(config.Version),
	}

	// Generate files
	files := map[string]string{
		"__manifest__.py": "files/manifest.py.tmpl",
		"__init__.py":     "files/init.py.tmpl",
	}

	if config.WithModel {
		files["models/__init__.py"] = "files/models_init.py.tmpl"
		files["models/"+config.Name+".py"] = "files/model.py.tmpl"
		files["views/"+config.Name+"_views.xml"] = "files/views.xml.tmpl"
		files["security/ir.model.access.csv"] = "files/security.csv.tmpl"
	}

	for outFile, tmplPath := range files {
		if err := renderFile(dir, outFile, tmplPath, data); err != nil {
			return fmt.Errorf("failed to render %s: %w", outFile, err)
		}
	}

	return nil
}

func renderFile(dir, outFile, tmplPath string, data TemplateData) error {
	content, err := templateFS.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	tmpl, err := template.New(outFile).Parse(string(content))
	if err != nil {
		return err
	}

	outPath := filepath.Join(dir, outFile)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func toPascal(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, "")
}

func formatDepends(deps []string) string {
	quoted := make([]string, len(deps))
	for i, d := range deps {
		quoted[i] = fmt.Sprintf("'%s'", d)
	}
	return strings.Join(quoted, ", ")
}

func isVersion18OrHigher(version string) bool {
	if version == "" {
		return true // default to modern
	}
	var major int
	fmt.Sscanf(version, "%d", &major)
	return major >= 18
}
