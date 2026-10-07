package subscription

import (
	"errors"
	"net/url"
	"strings"
)

// ResolveFetchURL accepts an HTTPS subscription URL or nagavpn://install-config?url=.
// The returned value is always an HTTPS URL; the control-plane still applies
// host policy before connecting.
func ResolveFetchURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("subscription URL must use HTTPS")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		if err := forbiddenSubscriptionURL(parsed); err != nil {
			return "", err
		}
		return raw, nil
	case "nagavpn":
		if !isInstallConfig(parsed) {
			return "", errors.New("unsupported nagavpn URL")
		}
		inner := strings.TrimSpace(parsed.Query().Get("url"))
		if inner == "" {
			return "", errors.New("install-config url is required")
		}
		nested, err := url.Parse(inner)
		if err != nil || !strings.EqualFold(nested.Scheme, "https") || nested.Host == "" {
			return "", errors.New("subscription URL must use HTTPS")
		}
		if err := forbiddenSubscriptionURL(nested); err != nil {
			return "", err
		}
		return inner, nil
	default:
		return "", errors.New("subscription URL must use HTTPS")
	}
}

func isInstallConfig(parsed *url.URL) bool {
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	path := strings.Trim(strings.ToLower(parsed.EscapedPath()), "/")
	if host == "install-config" && (path == "" || path == "install-config") {
		return true
	}
	return host == "" && path == "install-config"
}
