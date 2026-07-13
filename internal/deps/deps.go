package deps

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/module"
	"github.com/mart337i/odooctl/pkg/prompt"
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

// NormalizePackageName returns the comparable package name from a pip spec.
func NormalizePackageName(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if idx := strings.Index(pkg, ";"); idx >= 0 {
		pkg = strings.TrimSpace(pkg[:idx])
	}
	if idx := strings.Index(pkg, "["); idx >= 0 {
		pkg = strings.TrimSpace(pkg[:idx])
	}
	for _, op := range []string{"===", "==", ">=", "<=", "~=", "!=", ">", "<"} {
		if idx := strings.Index(pkg, op); idx >= 0 {
			pkg = strings.TrimSpace(pkg[:idx])
		}
	}
	return strings.ToLower(strings.ReplaceAll(pkg, "_", "-"))
}

// MergePackages appends packages not already present by normalized package name.
func MergePackages(existing, additions []string) ([]string, []string) {
	seen := make(map[string]bool)
	for _, pkg := range existing {
		if name := NormalizePackageName(pkg); name != "" {
			seen[name] = true
		}
	}

	merged := append([]string{}, existing...)
	var added []string
	for _, pkg := range additions {
		pkg = strings.TrimSpace(pkg)
		name := NormalizePackageName(pkg)
		if pkg == "" || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, pkg)
		added = append(added, pkg)
	}
	return merged, added
}

// DiscoverPythonDepsForModules scans manifests and returns package -> modules requiring it.
// If targetModules is empty, all modules in dirs are scanned.
func DiscoverPythonDepsForModules(dirs []string, targetModules []string) map[string][]string {
	targets := make(map[string]bool)
	for _, mod := range targetModules {
		mod = strings.TrimSpace(mod)
		if mod != "" {
			targets[mod] = true
		}
	}

	discovered := make(map[string][]string)
	moduleSeenByPackage := make(map[string]map[string]bool)
	for _, dir := range dirs {
		modules, _ := module.FindModules(dir)
		for _, mod := range modules {
			if len(targets) > 0 && !targets[mod] {
				continue
			}
			manifestPath := filepath.Join(dir, mod, "__manifest__.py")
			for _, dep := range ParseManifestPythonDeps(manifestPath) {
				dep = strings.TrimSpace(dep)
				if dep == "" {
					continue
				}
				if moduleSeenByPackage[dep] == nil {
					moduleSeenByPackage[dep] = make(map[string]bool)
				}
				if moduleSeenByPackage[dep][mod] {
					continue
				}
				moduleSeenByPackage[dep][mod] = true
				discovered[dep] = append(discovered[dep], mod)
			}
		}
	}

	for pkg := range discovered {
		sort.Strings(discovered[pkg])
	}
	return discovered
}

func MissingPythonDeps(discovered map[string][]string, existingPkgs []string) []string {
	seen := make(map[string]bool)
	for _, pkg := range existingPkgs {
		if name := NormalizePackageName(pkg); name != "" {
			seen[name] = true
		}
	}

	var missing []string
	for pkg := range discovered {
		name := NormalizePackageName(pkg)
		if name != "" && !seen[name] {
			missing = append(missing, pkg)
		}
	}
	sort.Strings(missing)
	return missing
}

func SortedDiscoveredPackages(discovered map[string][]string) []string {
	packages := make([]string, 0, len(discovered))
	for pkg := range discovered {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return packages
}

// DiscoverPythonDeps scans manifests for external_dependencies.python
func DiscoverPythonDeps(dirs []string, existingPkgs []string) []string {
	discovered := DiscoverPythonDepsForModules(dirs, nil)
	missing := MissingPythonDeps(discovered, existingPkgs)

	if len(missing) == 0 {
		return nil
	}

	fmt.Printf("\n%s Python dependencies detected in manifests:\n", color.CyanString("🔍"))

	var selected []string
	for _, pkg := range missing {
		mods := discovered[pkg]
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
