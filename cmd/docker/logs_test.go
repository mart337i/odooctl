package docker

import (
	"strings"
	"testing"
)

func TestFilterLogTextGrep(t *testing.T) {
	text := "info line\nTraceback here\nother line"
	filtered := filterLogText(text, "traceback", false)
	if !strings.Contains(filtered, "Traceback here") || strings.Contains(filtered, "info line") {
		t.Fatalf("unexpected filtered output: %q", filtered)
	}
}

func TestFilterLogTextErrors(t *testing.T) {
	text := "INFO ok\nERROR bad\nValidationError invalid"
	filtered := filterLogText(text, "", true)
	for _, want := range []string{"ERROR bad", "ValidationError invalid"} {
		if !strings.Contains(filtered, want) {
			t.Fatalf("filtered output missing %q: %q", want, filtered)
		}
	}
	if strings.Contains(filtered, "INFO ok") {
		t.Fatalf("filtered output included info line: %q", filtered)
	}
}
