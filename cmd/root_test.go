package cmd

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestRootVersionFlagMatchesVersionCommand(t *testing.T) {
	flagOutput := runOdooctl(t, "--version")
	commandOutput := runOdooctl(t, "version")
	want := fmt.Sprintf("odooctl %s\n", version)

	if flagOutput != want {
		t.Fatalf("odooctl --version = %q, want %q", flagOutput, want)
	}
	if commandOutput != flagOutput {
		t.Fatalf("odooctl version = %q, want %q", commandOutput, flagOutput)
	}
}

func runOdooctl(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", ".."}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run .. %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}
