package docker

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatDaemonCheckError(t *testing.T) {
	err := formatDaemonCheckError("Cannot connect to the Docker daemon", errors.New("exit status 1"))
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	for _, want := range []string{"Docker daemon is not available", "Cannot connect to the Docker daemon", "Start Docker Desktop"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q missing %q", message, want)
		}
	}
}

func TestFormatDaemonCheckErrorNil(t *testing.T) {
	if err := formatDaemonCheckError("24.0", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatBindMountCheckError(t *testing.T) {
	err := formatBindMountCheckError("/home/user/project", "", errors.New("exit status 1"))
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	for _, want := range []string{"Docker cannot access files", "/home/user/project", "WSL integration"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q missing %q", message, want)
		}
	}
}
