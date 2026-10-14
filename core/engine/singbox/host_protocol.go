package singbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type tunHostRequest struct {
	ConfigPath string   `json:"config_path"`
	BinaryPath string   `json:"binary_path"`
	Env        []string `json:"env,omitempty"`
}

type tunHostReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func formatTunHostExit(err error, stderr string) string {
	message := strings.TrimSpace(stderr)
	if message != "" {
		if err != nil {
			return fmt.Sprintf("%v: %s", err, message)
		}
		return message
	}
	if err != nil {
		return err.Error()
	}
	return "sing-box exited unexpectedly"
}

func staleWintunAdapterNameOK(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func staleWintunCleanupScript(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if !staleWintunAdapterNameOK(name) {
		return "", false
	}
	script := strings.Join([]string{
		"$ErrorActionPreference = 'SilentlyContinue'",
		fmt.Sprintf("$n = '%s'", name),
		"Get-NetAdapter -IncludeHidden | Where-Object { $_.Name -eq $n -or $_.InterfaceDescription -eq $n } | ForEach-Object { Disable-NetAdapter -Name $_.Name -Confirm:$false }",
		"Get-PnpDevice -Class Net | Where-Object { $_.FriendlyName -eq $n } | ForEach-Object { Disable-PnpDevice -InstanceId $_.InstanceId -Confirm:$false; Remove-PnpDevice -InstanceId $_.InstanceId -Confirm:$false }",
	}, "; ")
	return script, true
}

func tunHostReplyError(reply tunHostReply, decodeErr error) error {
	if decodeErr != nil {
		return decodeErr
	}
	message := strings.TrimSpace(reply.Error)
	if message == "" {
		return nil
	}
	return errors.New(message)
}

// Privileged peers accept only the finite start protocol; ordinary clients do
// not get to choose host output files or arbitrary process environment values.
func decodeTunHostRequest(reader io.Reader) (tunHostRequest, error) {
	var request tunHostRequest
	decoder := json.NewDecoder(io.LimitReader(reader, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, errors.New("invalid TUN host request")
	}
	if request.ConfigPath == "" || request.BinaryPath == "" {
		return request, errors.New("TUN host request is incomplete")
	}
	allowed := map[string]bool{
		"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true":      true,
		"ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM=true":  true,
		"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true": true,
	}
	if len(request.Env) > len(allowed) {
		return request, errors.New("unexpected TUN host environment")
	}
	seen := make(map[string]bool)
	for _, value := range request.Env {
		if !allowed[value] || seen[value] {
			return request, errors.New("unexpected TUN host environment")
		}
		seen[value] = true
	}
	return request, nil
}
