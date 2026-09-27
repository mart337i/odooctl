package docker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestClassifyDaemonError(t *testing.T) {
	platform := HostPlatform{OS: "linux", Arch: "amd64"}
	tests := []struct {
		name   string
		output string
		want   ErrorCode
	}{
		{name: "missing cli", output: `exec: "docker": executable file not found in $PATH`, want: ErrorCodeDockerCLIUnavailable},
		{name: "socket permission", output: "permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock", want: ErrorCodeDockerSocketPermission},
		{name: "context", output: "context desktop-linux does not exist", want: ErrorCodeDockerContextUnavailable},
		{name: "daemon", output: "Cannot connect to the Docker daemon", want: ErrorCodeDockerDaemonUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := classifyDaemonError(tt.output, errors.New("exit status 1"), platform)
			if diagnostic.Code != tt.want {
				t.Fatalf("code = %q, want %q", diagnostic.Code, tt.want)
			}
		})
	}
}

func TestWSLRemediation(t *testing.T) {
	diagnostic := classifyDaemonError("Cannot connect to the Docker daemon", errors.New("exit status 1"), HostPlatform{OS: "linux", Arch: "amd64", WSL2: true})
	if diagnostic.Code != ErrorCodeDockerDaemonUnavailable {
		t.Fatalf("code = %q, want daemon unavailable", diagnostic.Code)
	}
	message := diagnostic.Error()
	for _, want := range []string{"Docker Desktop", "WSL integration", "/mnt/c"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q missing %q", message, want)
		}
	}
}

func TestBindMountCheckDistinguishesRegistryFailure(t *testing.T) {
	diagnostic, ok := formatBindMountCheckError("/home/user/project", "failed to resolve source metadata: network is unreachable", errors.New("exit status 1")).(*DiagnosticError)
	if !ok {
		t.Fatal("expected DiagnosticError")
	}
	if diagnostic.Code != ErrorCodeDockerBindCheckUnavailable {
		t.Fatalf("code = %q, want bind check unavailable", diagnostic.Code)
	}
	if diagnostic.Category != "network" || !diagnostic.Retryable {
		t.Fatalf("unexpected network diagnostic: %#v", diagnostic)
	}
}

func TestHostPathPermissionError(t *testing.T) {
	diagnostic, ok := formatHostPathCheckError("/home/user/project", os.ErrPermission).(*DiagnosticError)
	if !ok {
		t.Fatal("expected DiagnosticError")
	}
	if diagnostic.Code != ErrorCodeHostPathNotWritable {
		t.Fatalf("code = %q, want host path not writable", diagnostic.Code)
	}
}

func TestFormatComposeConfigError(t *testing.T) {
	diagnostic, ok := formatComposeConfigError("services.odoo.volumes must be a list", errors.New("exit status 1")).(*DiagnosticError)
	if !ok {
		t.Fatal("expected DiagnosticError")
	}
	if diagnostic.Code != ErrorCodeDockerComposeConfigInvalid || diagnostic.Retryable {
		t.Fatalf("unexpected compose config diagnostic: %#v", diagnostic)
	}
}

func TestFormatCLIUnavailableError(t *testing.T) {
	diagnostic, ok := FormatCLIUnavailableError(errors.New("executable file not found")).(*DiagnosticError)
	if !ok {
		t.Fatal("expected DiagnosticError")
	}
	if diagnostic.Code != ErrorCodeDockerCLIUnavailable || diagnostic.Retryable {
		t.Fatalf("unexpected CLI diagnostic: %#v", diagnostic)
	}
}

func TestComposeInvocationPrefersDockerComposePlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "docker"), `#!/bin/sh
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "compose" ] && [ "$2" = "ls" ]; then
  exit 0
fi
exit 1
`)
	writeExecutable(t, filepath.Join(dir, "docker-compose"), `#!/bin/sh
printf 'Docker Compose version standalone\n'
exit 0
`)
	t.Setenv("PATH", dir)

	invocation, err := composeInvocation()
	if err != nil {
		t.Fatal(err)
	}
	if invocation.name != "docker" || len(invocation.prefix) != 1 || invocation.prefix[0] != "compose" {
		t.Fatalf("composeInvocation() = %#v, want docker compose plugin", invocation)
	}
}

func TestComposeInvocationFallsBackToStandalone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "docker"), `#!/bin/sh
exit 1
`)
	writeExecutable(t, filepath.Join(dir, "docker-compose"), `#!/bin/sh
if [ "$1" = "version" ]; then
  printf 'Docker Compose version standalone\n'
  exit 0
fi
if [ "$1" = "ls" ]; then
  exit 0
fi
exit 1
`)
	t.Setenv("PATH", dir)

	invocation, err := composeInvocation()
	if err != nil {
		t.Fatal(err)
	}
	if invocation.name != "docker-compose" || len(invocation.prefix) != 0 {
		t.Fatalf("composeInvocation() = %#v, want standalone docker-compose", invocation)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}
