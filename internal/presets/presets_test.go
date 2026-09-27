package presets

import "testing"

func TestExpandQueueJobPreset(t *testing.T) {
	expansion, err := Expand([]string{"queue-job"}, "18.0")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expansion.Presets, []string{"queue-job"}; !sameStrings(got, want) {
		t.Fatalf("Presets = %v, want %v", got, want)
	}
	if got, want := expansion.Modules, []string{"queue_job"}; !sameStrings(got, want) {
		t.Fatalf("Modules = %v, want %v", got, want)
	}
	if got, want := expansion.ServerWideModules, []string{"queue_job"}; !sameStrings(got, want) {
		t.Fatalf("ServerWideModules = %v, want %v", got, want)
	}
	if got, want := expansion.OdooConfig["workers"], "2"; got != want {
		t.Fatalf("workers = %q, want %q", got, want)
	}
	if got, want := expansion.Environment["ODOO_QUEUE_JOB_CHANNELS"], "root:4"; got != want {
		t.Fatalf("ODOO_QUEUE_JOB_CHANNELS = %q, want %q", got, want)
	}
	if len(expansion.Repositories) != 1 {
		t.Fatalf("Repositories length = %d, want 1", len(expansion.Repositories))
	}
	repo := expansion.Repositories[0]
	if repo.Name != "oca-queue" || repo.Branch != "18.0" || !repo.AddonsPath {
		t.Fatalf("Repository = %#v, want oca-queue 18.0 addon repo", repo)
	}
}

func TestExpandMigrationPreset(t *testing.T) {
	expansion, err := Expand([]string{"migration"}, "17.0")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expansion.PipPackages, []string{"openupgradelib"}; !sameStrings(got, want) {
		t.Fatalf("PipPackages = %v, want %v", got, want)
	}
	if len(expansion.Repositories) != 2 {
		t.Fatalf("Repositories length = %d, want 2", len(expansion.Repositories))
	}
	if expansion.Repositories[0].Branch != "17.0" || !expansion.Repositories[0].AddonsPath {
		t.Fatalf("OpenUpgrade repository = %#v, want versioned addon repo", expansion.Repositories[0])
	}
	if expansion.Repositories[1].Branch != "master" || expansion.Repositories[1].AddonsPath {
		t.Fatalf("upgrade-util repository = %#v, want master tooling repo", expansion.Repositories[1])
	}
}

func TestCleanNamesSplitsAndDeduplicates(t *testing.T) {
	got := CleanNames([]string{"queue-job,migration", "queue-job", ""})
	want := []string{"queue-job", "migration"}
	if !sameStrings(got, want) {
		t.Fatalf("CleanNames() = %v, want %v", got, want)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
