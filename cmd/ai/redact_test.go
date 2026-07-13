package ai

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	githubToken := "github_" + "pat_" + "123456789"
	shortToken := "g" + "hp_abcdef"
	input := "password=supersecret token: " + githubToken + " " + shortToken + " secret=veryprivate"
	redacted := Redact(input)
	for _, leaked := range []string{"supersecret", githubToken, shortToken, "veryprivate"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, "<redacted>") {
		t.Fatalf("expected redaction marker in %q", redacted)
	}
}
