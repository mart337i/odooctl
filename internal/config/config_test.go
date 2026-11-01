package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectLinkLoadsStateWithoutRepoMarker(t *testing.T) {
	home := t.TempDir()
	projectRoot := filepath.Join(home, "repo")
	if err := os.MkdirAll(filepath.Join(projectRoot, "module"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	state := &State{
		ProjectName: "repo",
		OdooVersion: "19.0",
		Branch:      "main",
		ProjectRoot: projectRoot,
		Ports:       CalculatePorts("19.0"),
		CreatedAt:   time.Now(),
	}
	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := SaveProjectLink(state); err != nil {
		t.Fatalf("SaveProjectLink() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, legacyMarkerFileName)); !os.IsNotExist(err) {
		t.Fatalf("legacy marker exists or stat failed unexpectedly: %v", err)
	}

	loaded, err := LoadFromDir(filepath.Join(projectRoot, "module"))
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if loaded.ProjectRoot != projectRoot || loaded.ProjectName != "repo" {
		t.Fatalf("loaded state = %#v", loaded)
	}
}

func TestSaveProjectLinkRemovesLegacyMarker(t *testing.T) {
	home := t.TempDir()
	projectRoot := filepath.Join(home, "repo")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	state := &State{ProjectName: "repo", OdooVersion: "19.0", Branch: "main", ProjectRoot: projectRoot, CreatedAt: time.Now()}
	envDir, err := EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, legacyMarkerFileName), []byte(envDir), 0644); err != nil {
		t.Fatal(err)
	}

	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := SaveProjectLink(state); err != nil {
		t.Fatalf("SaveProjectLink() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, legacyMarkerFileName)); !os.IsNotExist(err) {
		t.Fatalf("legacy marker was not removed: %v", err)
	}
}

func TestRemoveProjectLink(t *testing.T) {
	home := t.TempDir()
	projectRoot := filepath.Join(home, "repo")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	state := &State{ProjectName: "repo", OdooVersion: "19.0", Branch: "main", ProjectRoot: projectRoot, CreatedAt: time.Now()}
	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := SaveProjectLink(state); err != nil {
		t.Fatalf("SaveProjectLink() error = %v", err)
	}
	linkPath, err := ProjectLinkPath(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("project link missing before remove: %v", err)
	}
	if err := RemoveProjectLink(projectRoot); err != nil {
		t.Fatalf("RemoveProjectLink() error = %v", err)
	}
	if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("project link was not removed: %v", err)
	}
}

func TestLookupFromDirDoesNotRepairOrRemoveMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	state := &State{ProjectName: "passive", Branch: "main", ProjectRoot: root, OdooVersion: "18.0"}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	envDir, _ := EnvironmentDir(state.ProjectName, state.Branch)
	marker := filepath.Join(root, legacyMarkerFileName)
	if err := os.WriteFile(marker, []byte(envDir), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LookupFromDir(root)
	if err != nil || loaded.ProjectRoot != root {
		t.Fatalf("lookup: %+v %v", loaded, err)
	}
	link, _ := ProjectLinkPath(root)
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatalf("passive lookup wrote link: %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != envDir {
		t.Fatalf("passive lookup changed marker: %s %v", data, err)
	}
}

func TestPassiveLookupRejectsAmbiguousEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	for _, branch := range []string{"main", "other"} {
		state := &State{ProjectName: "ambiguous", Branch: branch, ProjectRoot: root, OdooVersion: "18.0"}
		if err := state.Save(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LookupFromDir(root); err == nil {
		t.Fatal("passive lookup silently selected one of multiple environments")
	}
	selected := &State{ProjectName: "ambiguous", Branch: "other", ProjectRoot: root, OdooVersion: "18.0"}
	if err := SaveProjectLink(selected); err != nil {
		t.Fatal(err)
	}
	loaded, err := LookupFromDir(root)
	if err != nil || loaded.Branch != "other" {
		t.Fatalf("explicit project link was not respected: %+v %v", loaded, err)
	}
}
