package docker

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// ErrorCode identifies a Docker or host failure in machine-readable output.
type ErrorCode string

const (
	ErrorCodeDockerCLIUnavailable       ErrorCode = "docker_cli_unavailable"
	ErrorCodeDockerDaemonUnavailable    ErrorCode = "docker_daemon_unavailable"
	ErrorCodeDockerSocketPermission     ErrorCode = "docker_socket_permission"
	ErrorCodeDockerContextUnavailable   ErrorCode = "docker_context_unavailable"
	ErrorCodeDockerComposeUnavailable   ErrorCode = "docker_compose_unavailable"
	ErrorCodeDockerComposeConfigInvalid ErrorCode = "docker_compose_config_invalid"
	ErrorCodeDockerBindMountDenied      ErrorCode = "docker_bind_mount_denied"
	ErrorCodeDockerBindCheckUnavailable ErrorCode = "docker_bind_check_unavailable"
	ErrorCodeHostPathUnavailable        ErrorCode = "host_path_unavailable"
	ErrorCodeHostPathNotWritable        ErrorCode = "host_path_not_writable"
)

// HostPlatform describes the local environment relevant to Docker access.
type HostPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	WSL2 bool   `json:"wsl2"`
}

// DiagnosticError preserves the cause while exposing remediation metadata.
type DiagnosticError struct {
	Code        ErrorCode
	Category    string
	Retryable   bool
	Summary     string
	Detail      string
	Remediation []string
	Cause       error
}

// IsRetryable reports whether a Docker operation may succeed after a short wait.
func IsRetryable(err error) bool {
	var diagnostic *DiagnosticError
	return errors.As(err, &diagnostic) && diagnostic.Retryable
}

func (e *DiagnosticError) Error() string {
	message := e.Summary
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	if len(e.Remediation) > 0 {
		message += "\n" + strings.Join(e.Remediation, "\n")
	}
	return message
}

func (e *DiagnosticError) Unwrap() error { return e.Cause }

func DetectHostPlatform() HostPlatform {
	platform := HostPlatform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if runtime.GOOS != "linux" {
		return platform
	}

	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		platform.WSL2 = true
		return platform
	}

	if data, err := os.ReadFile("/proc/version"); err == nil {
		version := strings.ToLower(string(data))
		platform.WSL2 = strings.Contains(version, "microsoft") || strings.Contains(version, "wsl")
	}
	return platform
}

func formatDaemonCheckError(output string, err error) error {
	if err == nil {
		return nil
	}
	return classifyDaemonError(output, err, DetectHostPlatform())
}

// FormatCLIUnavailableError creates a structured error for a missing Docker executable.
func FormatCLIUnavailableError(err error) error {
	return &DiagnosticError{
		Code:        ErrorCodeDockerCLIUnavailable,
		Category:    "cli",
		Retryable:   false,
		Summary:     "Docker CLI is not available",
		Detail:      err.Error(),
		Remediation: remediationFor(ErrorCodeDockerCLIUnavailable, DetectHostPlatform()),
		Cause:       err,
	}
}

func classifyDaemonError(output string, err error, platform HostPlatform) *DiagnosticError {
	detail := strings.TrimSpace(output)
	if detail == "" && err != nil {
		detail = err.Error()
	}
	lower := strings.ToLower(detail)

	code := ErrorCodeDockerDaemonUnavailable
	category := "daemon"
	summary := "Docker daemon is not available"
	retryable := true
	switch {
	case strings.Contains(lower, "executable file not found"), strings.Contains(lower, "docker: command not found"), strings.Contains(lower, "is not recognized as an internal or external command"):
		code = ErrorCodeDockerCLIUnavailable
		category = "cli"
		summary = "Docker CLI is not available"
		retryable = false
	case strings.Contains(lower, "permission denied") && (strings.Contains(lower, "docker.sock") || strings.Contains(lower, "docker daemon")):
		code = ErrorCodeDockerSocketPermission
		category = "permissions"
		summary = "Permission denied while accessing Docker"
		retryable = false
	case strings.Contains(lower, "context") && (strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") || strings.Contains(lower, "endpoint")):
		code = ErrorCodeDockerContextUnavailable
		category = "context"
		summary = "Docker context is not available"
		retryable = false
	}

	return &DiagnosticError{
		Code:        code,
		Category:    category,
		Retryable:   retryable,
		Summary:     summary,
		Detail:      detail,
		Remediation: remediationFor(code, platform),
		Cause:       err,
	}
}

func formatBindMountCheckError(hostDir, output string, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		detail = err.Error()
	}
	lower := strings.ToLower(detail)
	code := ErrorCodeDockerBindMountDenied
	category := "filesystem"
	summary := fmt.Sprintf("Docker cannot access files under %s", hostDir)
	retryable := false
	if strings.Contains(lower, "pull access denied") || strings.Contains(lower, "pulling from host") || strings.Contains(lower, "failed to resolve") || strings.Contains(lower, "failed to copy") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "temporary failure") || strings.Contains(lower, "tls handshake timeout") || strings.Contains(lower, "i/o timeout") {
		code = ErrorCodeDockerBindCheckUnavailable
		category = "network"
		summary = "Docker bind-mount check could not run"
		retryable = true
	}

	return &DiagnosticError{
		Code:        code,
		Category:    category,
		Retryable:   retryable,
		Summary:     summary,
		Detail:      detail,
		Remediation: remediationFor(code, DetectHostPlatform()),
		Cause:       err,
	}
}

func formatHostPathCheckError(hostDir string, err error) error {
	if err == nil {
		return nil
	}
	code := ErrorCodeHostPathUnavailable
	if os.IsPermission(err) {
		code = ErrorCodeHostPathNotWritable
	}
	return &DiagnosticError{
		Code:        code,
		Category:    "filesystem",
		Retryable:   false,
		Summary:     fmt.Sprintf("Project path is not usable: %s", hostDir),
		Detail:      err.Error(),
		Remediation: remediationFor(code, DetectHostPlatform()),
		Cause:       err,
	}
}

func formatComposeCheckError(output string, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		detail = err.Error()
	}
	return &DiagnosticError{
		Code:        ErrorCodeDockerComposeUnavailable,
		Category:    "cli",
		Retryable:   false,
		Summary:     "Docker Compose is not available",
		Detail:      detail,
		Remediation: remediationFor(ErrorCodeDockerComposeUnavailable, DetectHostPlatform()),
		Cause:       err,
	}
}

func formatComposeConfigError(output string, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		detail = err.Error()
	}
	return &DiagnosticError{
		Code:        ErrorCodeDockerComposeConfigInvalid,
		Category:    "configuration",
		Retryable:   false,
		Summary:     "Docker Compose configuration is invalid",
		Detail:      detail,
		Remediation: []string{"Inspect the generated docker-compose.yml and rerun 'odooctl docker create' if it is stale"},
		Cause:       err,
	}
}

func remediationFor(code ErrorCode, platform HostPlatform) []string {
	wsl := []string{
		"Start Docker Desktop if using it, then enable Settings > Resources > WSL integration for this distribution",
		"If using native Docker Engine in WSL2, check it with 'systemctl status docker' and start it with 'sudo systemctl start docker'",
		"Prefer keeping the project inside the WSL filesystem instead of under /mnt/c or another Windows-mounted path",
	}
	linux := []string{
		"Check Docker Engine with 'systemctl status docker' and start it with 'sudo systemctl start docker'",
	}
	if platform.WSL2 {
		linux = wsl
	}

	switch code {
	case ErrorCodeDockerCLIUnavailable:
		if platform.WSL2 {
			return []string{"Install Docker Desktop and enable WSL2 integration, or install Docker Engine inside this WSL2 distribution"}
		}
		return []string{"Install Docker Engine and ensure the 'docker' command is on PATH"}
	case ErrorCodeDockerSocketPermission:
		if platform.WSL2 {
			return wsl
		}
		return []string{"Check access to /var/run/docker.sock or configure rootless Docker", "Add the user to the docker group only if that is appropriate for this host, then start a new login session"}
	case ErrorCodeDockerContextUnavailable:
		return []string{"Run 'docker context ls' and select a running context with 'docker context use <name>'"}
	case ErrorCodeDockerComposeUnavailable:
		return []string{"Install the Docker Compose v2 plugin or standalone docker-compose, then verify it with 'docker compose version' or 'docker-compose version'"}
	case ErrorCodeDockerComposeConfigInvalid:
		return []string{"Inspect the generated docker-compose.yml and rerun 'odooctl docker create' if it is stale"}
	case ErrorCodeDockerBindMountDenied:
		if platform.WSL2 {
			return wsl
		}
		return append(linux, "Ensure Docker can read the project path; check SELinux policy if it is enforcing")
	case ErrorCodeDockerBindCheckUnavailable:
		return []string{"Retry after Docker can reach the container registry; the bind mount itself could not be verified"}
	case ErrorCodeHostPathNotWritable:
		return []string{"Check ownership and write permissions for the project path"}
	case ErrorCodeHostPathUnavailable:
		return []string{"Check that the project path exists and is accessible from the current shell"}
	default:
		return linux
	}
}
