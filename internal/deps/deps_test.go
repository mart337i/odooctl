package deps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizePackageName(t *testing.T) {
	cases := map[string]string{
		"requests==2.31.0":                           "requests",
		"python_slugify>=8":                          "python-slugify",
		"Pandas[performance]~=2.0; python_version>3": "pandas",
		"  zeep <= 4.2 ":                             "zeep",
	}
	for input, want := range cases {
		if got := NormalizePackageName(input); got != want {
			t.Fatalf("NormalizePackageName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMergePackagesUsesNormalizedNames(t *testing.T) {
	merged, added := MergePackages([]string{"requests==2.31.0"}, []string{"requests>=2", "zeep"})
	if !reflect.DeepEqual(merged, []string{"requests==2.31.0", "zeep"}) {
		t.Fatalf("merged = %#v", merged)
	}
	if !reflect.DeepEqual(added, []string{"zeep"}) {
		t.Fatalf("added = %#v", added)
	}
}

func TestDiscoverPythonDepsForModules(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "module_a", `{
    'name': 'Module A',
    'external_dependencies': {'python': ['requests', 'zeep']},
}`)
	writeManifest(t, root, "module_b", `{
    'name': 'Module B',
    'external_dependencies': {'python': ['pandas']},
}`)

	discovered := DiscoverPythonDepsForModules([]string{root}, []string{"module_a"})
	if !reflect.DeepEqual(discovered, map[string][]string{"requests": {"module_a"}, "zeep": {"module_a"}}) {
		t.Fatalf("discovered = %#v", discovered)
	}
	missing := MissingPythonDeps(discovered, []string{"requests==2.31.0"})
	if !reflect.DeepEqual(missing, []string{"zeep"}) {
		t.Fatalf("missing = %#v", missing)
	}
}

func writeManifest(t *testing.T, root, moduleName, manifest string) {
	t.Helper()
	moduleDir := filepath.Join(root, moduleName)
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "__manifest__.py"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
}
