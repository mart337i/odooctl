package module

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseManifestContent(t *testing.T) {
	data := []byte(`{'name': 'Sales', 'version': '18.0.1.0.0',
        'depends': ['base', 'sale', 'base'],
        'external_dependencies': {'python': ['requests', 'requests']},
        'installable': False, 'application': True}`)
	info := ParseManifestContent("sale_addon", data)
	if info.Module != "sale_addon" || info.Path != filepath.Join("sale_addon", "__manifest__.py") || info.Name != "Sales" || info.Version != "18.0.1.0.0" || info.Installable || !info.Application {
		t.Fatalf("unexpected metadata: %+v", info)
	}
	if !reflect.DeepEqual(info.Depends, []string{"base", "sale"}) || !reflect.DeepEqual(info.ExternalPython, []string{"requests"}) {
		t.Fatalf("unexpected dependencies: %+v", info)
	}
	defaults := ParseManifestContent("empty", nil)
	if !defaults.Installable || defaults.Application || defaults.Name != "" {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
}

func TestParseManifestContentDoesNotReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "__manifest__.py")
	data := []byte("{'name': 'Original', 'depends': ['base']}")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := ParseManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseManifestContent(dir, data); !reflect.DeepEqual(got, fromFile) {
		t.Fatalf("parser changed: %+v != %+v", got, fromFile)
	}
	if err := os.WriteFile(path, []byte("{'name': 'Replacement', 'depends': ['secret']}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ParseManifestContent(dir, data); !reflect.DeepEqual(got, fromFile) {
		t.Fatalf("content parser reopened path: %+v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := ParseManifestContent(dir, data); !reflect.DeepEqual(got, fromFile) {
		t.Fatalf("content parser needs file: %+v", got)
	}
}
