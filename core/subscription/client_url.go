package subscription

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var cgnatIPv4 = net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

func forbiddenSubscriptionURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme != "https" || strings.TrimSpace(parsed.Host) == "" {
		return errors.New("subscription URL must use HTTPS")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return errors.New("subscription URL host is not allowed")
	}
	if host == "localhost" || strings.HasPrefix(host, "localhost.") {
		return errors.New("subscription URL host is not allowed")
	}
	if label, _, _ := strings.Cut(host, "."); label == "panel" {
		return errors.New("subscription URL host is not allowed")
	}
	ip := net.ParseIP(host)
	if ip != nil && forbiddenSubscriptionIP(ip) {
		return errors.New("subscription URL host is not allowed")
	}
	return nil
}

func forbiddenSubscriptionIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && (v4[0] == 0 || cgnatIPv4.Contains(v4)) {
		return true
	}
	return false
}
