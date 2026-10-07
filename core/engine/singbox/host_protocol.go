package singbox

import (
	"errors"
	"fmt"
	"strings"
)

type tunHostRequest struct {
	ConfigPath string   `json:"config_path"`
	BinaryPath string   `json:"binary_path"`
	LogPath    string   `json:"log_path,omitempty"`
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
