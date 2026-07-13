package docker

import (
	"errors"
	"testing"
)

func TestShouldKeepConfigAfterDockerCleanupError(t *testing.T) {
	dockerErr := errors.New("docker unavailable")
	if !shouldKeepConfigAfterDockerCleanupError(dockerErr, true, true) {
		t.Fatal("expected config to be kept when volume cleanup fails")
	}
	if shouldKeepConfigAfterDockerCleanupError(dockerErr, false, true) {
		t.Fatal("config-only cleanup may continue after docker cleanup failure")
	}
	if shouldKeepConfigAfterDockerCleanupError(nil, true, true) {
		t.Fatal("successful docker cleanup should not block config removal")
	}
}

func TestShouldReturnDockerCleanupError(t *testing.T) {
	dockerErr := errors.New("docker unavailable")
	if !shouldReturnDockerCleanupError(dockerErr, false) {
		t.Fatal("expected docker-only cleanup failure to be returned")
	}
	if shouldReturnDockerCleanupError(dockerErr, true) {
		t.Fatal("config cleanup should succeed after files are removed")
	}
	if shouldReturnDockerCleanupError(nil, false) {
		t.Fatal("successful docker cleanup should not return an error")
	}
}
