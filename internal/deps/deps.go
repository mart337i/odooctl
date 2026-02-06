package deps

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egeskov/odooctl/internal/module"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
)

// ParsePipPackages parses pip packages from comma-separated string or requirements.txt file
func ParsePipPackages(input string) []string {
	if input == "" {
		return nil
	}

	// Check if it's a file path
	if strings.HasSuffix(input, ".txt") || strings.Contains(input, "/") {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return parseCommaSeparated(input)
		}

		file, err := os.Open(absPath)
		if err != nil {
			return parseCommaSeparated(input)
		}
		defer file.Close()

		var packages []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			packages = append(packages, line)
		}

		if len(packages) > 0 {
			fmt.Printf("%s Loaded %d packages from %s\n", color.CyanString("📦"), len(packages), input)
			return packages
		}
	}

	return parseCommaSeparated(input)
}

func parseCommaSeparated(input string) []string {
	var packages []string
	for _, pkg := range strings.Split(input, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			packages = append(packages, pkg)
		}
	}
	return packages
}

// DiscoverPythonDeps scans manifests for external_dependencies.python
func DiscoverPythonDeps(dirs []string, existingPkgs []string) []string {
	existingSet := make(map[string]bool)
	for _, pkg := range existingPkgs {
		// Normalize package name (remove version specs)
		name := strings.Split(pkg, "==")[0]
		name = strings.Split(name, ">=")[0]
		name = strings.Split(name, "<=")[0]
		name = strings.Split(name, "[")[0]
		existingSet[strings.ToLower(name)] = true
	}

	discovered := make(map[string][]string) // package -> modules requiring it

	for _, dir := range dirs {
		modules, _ := module.FindModules(dir)
		for _, mod := range modules {
			manifestPath := filepath.Join(dir, mod, "__manifest__.py")
			deps := ParseManifestPythonDeps(manifestPath)
			for _, dep := range deps {
				depLower := strings.ToLower(dep)
				if !existingSet[depLower] {
					discovered[dep] = append(discovered[dep], mod)
				}
			}
		}
	}

	if len(discovered) == 0 {
		return nil
	}

	fmt.Printf("\n%s Python dependencies detected in manifests:\n", color.CyanString("🔍"))

	var selected []string
	for pkg, mods := range discovered {
		fmt.Printf("\n%s %s\n", color.YellowString("📦"), pkg)
		fmt.Printf("   Required by: %s\n", color.HiBlackString(strings.Join(mods, ", ")))

		confirmed, err := prompt.Confirm(fmt.Sprintf("   Include %s?", pkg), true)
		if err == nil && confirmed {
			selected = append(selected, pkg)
			fmt.Printf("   %s Will install %s\n", color.GreenString("✓"), pkg)
		} else {
			fmt.Printf("   %s Skipped - module(s) may fail without it\n", color.YellowString("⚠️"))
		}
	}

	if len(selected) > 0 {
		fmt.Printf("\n%s Added %d Python packages from manifests\n", color.GreenString("✓"), len(selected))
	}

	return selected
}

// ParseManifestPythonDeps extracts python deps from __manifest__.py
func ParseManifestPythonDeps(manifestPath string) []string {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}

	text := string(content)

	extIdx := strings.Index(text, "external_dependencies")
	if extIdx == -1 {
		return nil
	}

	braceStart := strings.Index(text[extIdx:], "{")
	if braceStart == -1 {
		return nil
	}

	braceCount := 1
	start := extIdx + braceStart + 1
	end := start
	for i := start; i < len(text) && braceCount > 0; i++ {
		switch text[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
		}
		end = i
	}

	extDeps := text[start:end]

	pythonIdx := strings.Index(extDeps, "'python'")
	if pythonIdx == -1 {
		pythonIdx = strings.Index(extDeps, "\"python\"")
	}
	if pythonIdx == -1 {
		return nil
	}

	listStart := strings.Index(extDeps[pythonIdx:], "[")
	if listStart == -1 {
		return nil
	}
	listEnd := strings.Index(extDeps[pythonIdx+listStart:], "]")
	if listEnd == -1 {
		return nil
	}

	listContent := extDeps[pythonIdx+listStart+1 : pythonIdx+listStart+listEnd]

	var packages []string
	for _, item := range strings.Split(listContent, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "'\"")
		if item != "" {
			packages = append(packages, item)
		}
	}

	return packages
}
