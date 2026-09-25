package diagnostics

import (
	"path/filepath"
	"testing"

	"github.com/mart337i/odooctl/internal/config"
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

func TestExternalPortConflictsIgnoresPortsOwnedByRunningServices(t *testing.T) {
	ports := config.Ports{Odoo: 9930, Mailhog: 9955, SMTP: 2955, Debug: 7008}
	services := []ServiceStatus{
		{Name: "odoo", State: "running"},
		{Name: "mailhog", State: "running"},
	}
	if conflicts := externalPortConflicts(ports, services, []int{9930, 9955, 2955, 7008, 9999}); len(conflicts) != 1 || conflicts[0] != 9999 {
		t.Fatalf("external conflicts = %v, want [9999]", conflicts)
	}
}
