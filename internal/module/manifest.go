package module

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ManifestInfo struct {
	Module         string
	Path           string
	Name           string
	Version        string
	Depends        []string
	ExternalPython []string
	Installable    bool
	Application    bool
}

func ParseManifest(moduleDir string) (ManifestInfo, error) {
	path := filepath.Join(moduleDir, "__manifest__.py")
	data, err := os.ReadFile(path)
	if err != nil {
		return ManifestInfo{}, err
	}
	text := string(data)
	info := ManifestInfo{
		Module:      filepath.Base(moduleDir),
		Path:        path,
		Name:        parseStringField(text, "name"),
		Version:     parseStringField(text, "version"),
		Depends:     parseListField(text, "depends"),
		Installable: true,
	}
	info.ExternalPython = parseExternalPython(text)
	if installable, ok := parseBoolField(text, "installable"); ok {
		info.Installable = installable
	}
	if application, ok := parseBoolField(text, "application"); ok {
		info.Application = application
	}
	return info, nil
}

func parseStringField(text, key string) string {
	re := regexp.MustCompile(`["']` + regexp.QuoteMeta(key) + `["']\s*:\s*["']([^"']*)["']`)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func parseBoolField(text, key string) (bool, bool) {
	re := regexp.MustCompile(`["']` + regexp.QuoteMeta(key) + `["']\s*:\s*(True|False|true|false)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return false, false
	}
	return strings.EqualFold(matches[1], "true"), true
}

func parseListField(text, key string) []string {
	re := regexp.MustCompile(`(?s)["']` + regexp.QuoteMeta(key) + `["']\s*:\s*\[(.*?)\]`)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return nil
	}
	return parsePythonStringList(matches[1])
}

func parseExternalPython(text string) []string {
	extIdx := strings.Index(text, "external_dependencies")
	if extIdx == -1 {
		return nil
	}
	return parseListField(text[extIdx:], "python")
}

func parsePythonStringList(listContent string) []string {
	re := regexp.MustCompile(`["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(listContent, -1)
	values := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}
