package subscription

import (
	"errors"
	"net/url"
	"strings"
)

// ResolveFetchURL accepts direct HTTPS subscriptions and supported import links.
// The returned value is always an HTTPS URL; the control-plane still applies
// host policy before connecting.
func ResolveFetchURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("subscription URL must use HTTPS")
	}
	scheme := strings.ToLower(parsed.Scheme)
	var inner string
	switch scheme {
	case "https":
		if err := forbiddenSubscriptionURL(parsed); err != nil {
			return "", err
		}
		return raw, nil
	case "nagavpn", "clash", "v2rayng", "sing-box":
		action := "install-config"
		if scheme == "v2rayng" {
			action = "install-sub"
		}
		if scheme == "sing-box" {
			action = "import-remote-profile"
		}
		if !hasImportAction(parsed, action) || parsed.User != nil {
			return "", errors.New("unsupported subscription URL")
		}
		inner = strings.TrimSpace(parsed.Query().Get("url"))
	case "hiddify":
		const prefix = "hiddify://import/"
		if !strings.HasPrefix(strings.ToLower(raw), prefix) {
			return "", errors.New("unsupported subscription URL")
		}
		inner, _, _ = strings.Cut(raw[len(prefix):], "#")
		if !strings.HasPrefix(strings.ToLower(inner), "https://") {
			inner, err = url.PathUnescape(inner)
			if err != nil {
				return "", errors.New("invalid subscription URL")
			}
		}
	default:
		return "", errors.New("subscription URL must use HTTPS")
	}
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
}

func hasImportAction(parsed *url.URL, action string) bool {
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	path := strings.Trim(strings.ToLower(parsed.EscapedPath()), "/")
	return host == action && path == "" ||
		host == "" && path == action ||
		action == "install-config" && host == action && path == action
}
