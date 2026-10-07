package amneziawg

import (
	"regexp"
	"strings"
)

var secretPattern = regexp.MustCompile(`[A-Za-z0-9+/=_-]{40,}`)

// SafeError returns an error string that must not contain key material.
func SafeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	message = secretPattern.ReplaceAllString(message, "[redacted]")
	return strings.TrimSpace(message)
}
