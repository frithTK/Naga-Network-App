package olcrtc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir is the private directory for one profile's client.yaml.
func ConfigDir(root, profileID string) string {
	return filepath.Join(root, ".olcrtc", profileID)
}

const (
	DNS             = "1.1.1.1:53"
	SOCKSHost       = "127.0.0.1"
	SOCKSPort       = 10808
	MaxFlows        = 256
	SessionDuration = "6h"
	RetryDelay      = "2s"
	yamlName        = "client.yaml"
)

// allowedParams are the only URI k=v keys copied into net.
var allowedParams = map[string]struct{}{}

// Document is the client YAML passed as the only olcrtc argument.
type Document struct {
	Body string
}

// Build renders mode:cnc YAML. One profile is flat. Several profiles share
// crypto, DNS, SOCKS, UDP and lifecycle, with failover in envelope order.
func Build(cfg *Config) (Document, error) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return Document{}, fmt.Errorf("%w: profiles", ErrURI)
	}
	profiles := make([]Profile, len(cfg.Profiles))
	for i, profile := range cfg.Profiles {
		if !profile.RuntimeAllowed {
			return Document{}, ErrTransportOff
		}
		transport, err := runtimeTransport(profile)
		if err != nil {
			return Document{}, err
		}
		profile.Transport = transport
		profiles[i] = profile
	}
	if len(profiles) == 1 {
		return Document{Body: flatYAML(profiles[0])}, nil
	}
	return Document{Body: failoverYAML(profiles)}, nil
}

func runtimeTransport(profile Profile) (string, error) {
	transport := profile.Transport
	if profile.Provider == ProviderTelemost || profile.Provider == ProviderWBStream {
		if transport == TransportDataChannel {
			transport = TransportVP8Channel
		}
	}
	if transport != TransportDataChannel && transport != TransportVP8Channel {
		return "", ErrTransportOff
	}
	return transport, nil
}

func flatYAML(profile Profile) string {
	var b strings.Builder
	b.WriteString("mode: cnc\n")
	fmt.Fprintf(&b, "auth:\n  provider: %s\n", yamlScalar(profile.Provider))
	fmt.Fprintf(&b, "room:\n  id: %s\n", yamlQuote(profile.Room))
	fmt.Fprintf(&b, "crypto:\n  key: %s\n", yamlQuote(profile.Key))
	b.WriteString("net:\n")
	fmt.Fprintf(&b, "  transport: %s\n", yamlScalar(profile.Transport))
	writeParams(&b, "  ", profile.Params)
	fmt.Fprintf(&b, "  dns: %s\n", yamlQuote(DNS))
	fmt.Fprintf(&b, "socks:\n  host: %s\n  port: %d\n", yamlQuote(SOCKSHost), SOCKSPort)
	fmt.Fprintf(&b, "udp:\n  enabled: true\n  max_flows: %d\n", MaxFlows)
	fmt.Fprintf(&b, "lifecycle:\n  max_session_duration: %s\n", SessionDuration)
	return b.String()
}

func failoverYAML(profiles []Profile) string {
	var b strings.Builder
	b.WriteString("mode: cnc\n")
	fmt.Fprintf(&b, "crypto:\n  key: %s\n", yamlQuote(profiles[0].Key))
	fmt.Fprintf(&b, "net:\n  dns: %s\n", yamlQuote(DNS))
	fmt.Fprintf(&b, "socks:\n  host: %s\n  port: %d\n", yamlQuote(SOCKSHost), SOCKSPort)
	fmt.Fprintf(&b, "udp:\n  enabled: true\n  max_flows: %d\n", MaxFlows)
	fmt.Fprintf(&b, "lifecycle:\n  max_session_duration: %s\n", SessionDuration)
	b.WriteString("profiles:\n")
	for _, profile := range profiles {
		fmt.Fprintf(&b, "  - name: %s\n", yamlScalar(profile.Name))
		fmt.Fprintf(&b, "    auth:\n      provider: %s\n", yamlScalar(profile.Provider))
		fmt.Fprintf(&b, "    room:\n      id: %s\n", yamlQuote(profile.Room))
		b.WriteString("    net:\n")
		fmt.Fprintf(&b, "      transport: %s\n", yamlScalar(profile.Transport))
		writeParams(&b, "      ", profile.Params)
	}
	fmt.Fprintf(&b, "failover:\n  retry_delay: %s\n  max_cycles: 0\n", RetryDelay)
	return b.String()
}

func writeParams(b *strings.Builder, indent string, params map[string]string) {
	for key, value := range params {
		if _, ok := allowedParams[key]; !ok {
			continue
		}
		fmt.Fprintf(b, "%s%s: %s\n", indent, key, yamlQuote(value))
	}
}

func yamlScalar(value string) string {
	if value == "" || strings.ContainsAny(value, ":#{}[]&*!|>%@`\"', \n") {
		return yamlQuote(value)
	}
	return value
}

func yamlQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

// WriteFile stores the YAML for the profile directory. The file mode is 0600
// and the directory mode is 0700. Replacement is a rename in the same directory.
func WriteFile(dir string, doc Document) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(dir, yamlName)
	tmp, err := os.CreateTemp(dir, "client-*.yaml")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := tmp.WriteString(doc.Body); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, final); err != nil {
		return "", err
	}
	ok = true
	if err := os.Chmod(final, 0o600); err != nil {
		return "", err
	}
	return final, nil
}

// RemoveFile deletes the runtime YAML for a profile directory.
func RemoveFile(dir string) error {
	err := os.Remove(filepath.Join(dir, yamlName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
