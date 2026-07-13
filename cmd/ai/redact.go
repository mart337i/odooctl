package ai

import (
	"regexp"
	"strings"
)

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(github_pat_[A-Za-z0-9_]+)`),
	regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9_]+)`),
	regexp.MustCompile(`(?i)(password\s*[=:]\s*)[^\s]+`),
	regexp.MustCompile(`(?i)(token\s*[=:]\s*)[^\s]+`),
	regexp.MustCompile(`(?i)(secret\s*[=:]\s*)[^\s]+`),
}

func Redact(text string) string {
	redacted := text
	for _, pattern := range redactionPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			lower := strings.ToLower(match)
			for _, prefix := range []string{"password", "token", "secret"} {
				if strings.HasPrefix(lower, prefix) {
					parts := strings.SplitN(match, "=", 2)
					if len(parts) == 2 {
						return parts[0] + "=<redacted>"
					}
					parts = strings.SplitN(match, ":", 2)
					if len(parts) == 2 {
						return parts[0] + ":<redacted>"
					}
				}
			}
			return "<redacted>"
		})
	}
	return redacted
}
