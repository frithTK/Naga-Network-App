package winhelper

import (
	"encoding/xml"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	TaskName  = "NagaNetworkElevatedHelper"
	PipeName  = "NagaNetworkElevated"
	MutexName = "Local\\NagaNetworkElevatedHelper"
	ModeTun   = "tun-host"
	ModeOlc   = "olc-host"
)

var (
	ErrWindowsOnly       = errors.New("elevated helper is only supported on Windows")
	ErrHelperUnavailable = errors.New("elevated helper is not installed")
	ErrNotElevated       = errors.New("нужны права администратора: запусти установщик")
	ErrInvalidSpawn      = errors.New("invalid elevated helper request")
	pipeNameRe           = regexp.MustCompile(`^Naga(Tun|Olc)-[0-9a-f]{16}$`)
)

type SpawnRequest struct {
	Mode string `json:"mode"`
	Pipe string `json:"pipe"`
}

type SpawnReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func ValidMode(mode string) bool {
	return mode == ModeTun || mode == ModeOlc
}

func ValidPipeName(name string) bool {
	return pipeNameRe.MatchString(strings.TrimSpace(name))
}

func ValidateSpawn(mode, pipe string) error {
	if !ValidMode(mode) || !ValidPipeName(pipe) {
		return ErrInvalidSpawn
	}
	return nil
}

func AllowedBundledBinary(selfDir, got, name string) bool {
	if strings.TrimSpace(selfDir) == "" || strings.TrimSpace(got) == "" || strings.TrimSpace(name) == "" {
		return false
	}
	want := filepath.Join(selfDir, name)
	return strings.EqualFold(filepath.Clean(got), filepath.Clean(want))
}

func HostFlag(mode string) (string, bool) {
	switch mode {
	case ModeTun:
		return "--tun-host", true
	case ModeOlc:
		return "--olc-host", true
	default:
		return "", false
	}
}

func TaskDefinition(exe, sid string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Naga Network elevated TUN helper</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + xmlText(sid) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlText(sid) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>6</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlText(exe) + `</Command>
      <Arguments>--elevated-helper</Arguments>
      <WorkingDirectory>` + xmlText(filepath.Dir(exe)) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

func xmlText(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
