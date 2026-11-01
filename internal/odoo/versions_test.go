package odoo

import (
	"slices"
	"testing"
)

func TestOdooVersions(t *testing.T) {
	want := []string{"20.0", "19.0", "18.0", "17.0", "16.0", "15.0", "14.0", "13.0", "12.0"}
	if !slices.Equal(OdooVersions, want) {
		t.Fatalf("OdooVersions = %v, want %v", OdooVersions, want)
	}
}

func TestDefaultOdooVersion(t *testing.T) {
	if DefaultOdooVersion != "19.0" {
		t.Fatalf("DefaultOdooVersion = %q, want 19.0", DefaultOdooVersion)
	}
}

func TestVersionsString(t *testing.T) {
	want := "20.0, 19.0, 18.0, 17.0, 16.0, 15.0, 14.0, 13.0, 12.0"
	if got := VersionsString(); got != want {
		t.Fatalf("VersionsString() = %q, want %q", got, want)
	}
}
