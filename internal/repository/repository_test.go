package repository

import "testing"

func TestShellQuote(t *testing.T) {
	if got, want := shellQuote("/tmp/my key"), "'/tmp/my key'"; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
	if got, want := shellQuote("/tmp/it's-key"), "'/tmp/it'\\''s-key'"; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}
