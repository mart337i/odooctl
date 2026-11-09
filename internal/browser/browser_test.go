package browser

import "testing"

func TestSupportsVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"14.0", false},
		{"15.0", true},
		{"19.0", true},
		{"bad", false},
	} {
		if got := SupportsVersion(tc.version); got != tc.want {
			t.Fatalf("SupportsVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestExtractJSONOutputSkipsComposeStatusLines(t *testing.T) {
	output := "Container odoo-app Running\n Container project-odoo-run Creating\n{\"can_launch\":true}\n"
	got := ExtractJSONOutput(output)
	if got != `{"can_launch":true}` {
		t.Fatalf("ExtractJSONOutput() = %q", got)
	}
}
