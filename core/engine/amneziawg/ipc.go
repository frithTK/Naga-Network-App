package amneziawg

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	awgcfg "naga.network/core/profile/amneziawg"
)

func ipcRequest(cfg awgcfg.Config) (string, error) {
	privateKey, err := keyToHex(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("amneziawg private key is invalid")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", privateKey)
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&b, "listen_port=%d\n", cfg.ListenPort)
	}
	if cfg.Jc > 0 {
		fmt.Fprintf(&b, "jc=%d\n", cfg.Jc)
	}
	if cfg.Jmin > 0 {
		fmt.Fprintf(&b, "jmin=%d\n", cfg.Jmin)
	}
	if cfg.Jmax > 0 {
		fmt.Fprintf(&b, "jmax=%d\n", cfg.Jmax)
	}
	if cfg.S[0] > 0 {
		fmt.Fprintf(&b, "s1=%d\n", cfg.S[0])
	}
	if cfg.S[1] > 0 {
		fmt.Fprintf(&b, "s2=%d\n", cfg.S[1])
	}
	if cfg.S[2] > 0 {
		fmt.Fprintf(&b, "s3=%d\n", cfg.S[2])
	}
	if cfg.S[3] > 0 {
		fmt.Fprintf(&b, "s4=%d\n", cfg.S[3])
	}
	for i, header := range cfg.H {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		fmt.Fprintf(&b, "h%d=%s\n", i+1, header)
	}
	for i, payload := range cfg.I {
		if strings.TrimSpace(payload) != "" {
			fmt.Fprintf(&b, "i%d=%s\n", i+1, payload)
		}
	}
	if strings.TrimSpace(cfg.HeaderProtectionKey) != "" {
		hp, err := keyToHex(cfg.HeaderProtectionKey)
		if err != nil {
			return "", fmt.Errorf("amneziawg header protection key is invalid")
		}
		fmt.Fprintf(&b, "header_protection_key=%s\n", hp)
	}
	if pad := strings.TrimSpace(cfg.ContentPaddingAddition); pad != "" {
		fmt.Fprintf(&b, "content_padding_addition=%s\n", pad)
	}
	if value, ok := boolUAPI(cfg.RandomTrailers); ok {
		fmt.Fprintf(&b, "random_trailers=%s\n", value)
	}
	if value, ok := boolUAPI(cfg.DisableCookies); ok {
		fmt.Fprintf(&b, "disable_cookies=%s\n", value)
	}
	for _, peer := range cfg.Peers {
		publicKey, err := keyToHex(peer.PublicKey)
		if err != nil {
			return "", fmt.Errorf("amneziawg peer public key is invalid")
		}
		fmt.Fprintf(&b, "public_key=%s\n", publicKey)
		if strings.TrimSpace(peer.PresharedKey) != "" {
			psk, err := keyToHex(peer.PresharedKey)
			if err != nil {
				return "", fmt.Errorf("amneziawg peer preshared key is invalid")
			}
			fmt.Fprintf(&b, "preshared_key=%s\n", psk)
		}
		if peer.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", peer.Endpoint)
		}
		allowed := peer.AllowedIPs
		if len(allowed) == 0 {
			allowed = []string{"0.0.0.0/0", "::/0"}
		}
		for _, ip := range allowed {
			fmt.Fprintf(&b, "allowed_ip=%s\n", ip)
		}
		if peer.PersistentKeepalive > 0 {
			fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", peer.PersistentKeepalive)
		}
	}
	return b.String(), nil
}

func keyToHex(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return "", err
	}
	if len(decoded) != 32 {
		return "", fmt.Errorf("key length")
	}
	return hex.EncodeToString(decoded), nil
}

func boolUAPI(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "yes", "on":
		return "true", true
	case "0", "f", "false", "no", "off":
		return "false", true
	default:
		return "", false
	}
}
