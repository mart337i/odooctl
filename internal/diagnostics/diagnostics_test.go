package diagnostics

import (
	"path/filepath"
	"testing"
)

func TestCollectWithoutEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	report := Collect(filepath.Join(home, "project"))
	if report.OK {
		t.Fatal("expected report to be non-OK without an environment")
	}
	if report.Status != StatusError {
		t.Fatalf("status = %q, want %q", report.Status, StatusError)
	}
	if len(report.Checks) != 1 || report.Checks[0].ID != "environment" {
		t.Fatalf("unexpected checks: %#v", report.Checks)
	}
	if len(report.NextSteps) == 0 {
		t.Fatal("expected next steps")
	}
}
