package control

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DiscoveredApp is a running process that the user can add to a split-tunnel
// rule. Process is the executable basename; ProcessPath is the resolved
// image path (/proc/<pid>/exe on Linux, QueryFullProcessImageName on Windows).
type DiscoveredApp struct {
	Name        string `json:"name"`
	Process     string `json:"process"`
	ProcessPath string `json:"process_path,omitempty"`
}

var skippedDiscoveredProcesses = map[string]struct{}{
	"[system process]":    {},
	"conhost.exe":         {},
	"csrss.exe":           {},
	"dwm.exe":             {},
	"fontdrvhost.exe":     {},
	"idle":                {},
	"lsass.exe":           {},
	"lsm.exe":             {},
	"memory compression":  {},
	"naga-control.exe":    {},
	"naga_network.exe":    {},
	"naganetwork.exe":     {},
	"registry":            {},
	"secure system":       {},
	"services.exe":        {},
	"smss.exe":            {},
	"svchost.exe":         {},
	"system":              {},
	"system idle process": {},
	"wininit.exe":         {},
	"winlogon.exe":        {},
}

func shouldSkipDiscoveredProcess(name string) bool {
	base := strings.ToLower(strings.TrimSpace(processBaseName(name)))
	if base == "" || base == "." || base == "/" || base == `\` {
		return true
	}
	_, skip := skippedDiscoveredProcesses[base]
	return skip
}

func processBaseName(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	base := filepath.Base(normalized)
	if base == "." || base == "/" || base == `\` {
		return ""
	}
	return base
}

func displayNameFromProcess(base string) string {
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		return base[:len(base)-4]
	}
	if base == "" {
		return ""
	}
	return base
}

func discoveredAppFromPath(path string) DiscoveredApp {
	path = strings.TrimSpace(path)
	base := processBaseName(path)
	name := displayNameFromProcess(base)
	if name == "" {
		name = base
	}
	processPath := path
	if !strings.ContainsAny(path, `/\`) {
		processPath = ""
	}
	return DiscoveredApp{
		Name:        name,
		Process:     base,
		ProcessPath: processPath,
	}
}

func discoverProcesses(procRoot string, uid int, readlink func(string) (string, error), readFile func(string) ([]byte, error)) []DiscoveredApp {
	if strings.TrimSpace(procRoot) == "" || readlink == nil || readFile == nil {
		return []DiscoveredApp{}
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return []DiscoveredApp{}
	}
	seen := make(map[string]struct{})
	result := make([]DiscoveredApp, 0)
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		statusPath := filepath.Join(procRoot, entry.Name(), "status")
		status, err := readFile(statusPath)
		if err != nil {
			continue
		}
		if processUID(status) != uid {
			continue
		}
		exePath := filepath.Join(procRoot, entry.Name(), "exe")
		target, err := readlink(exePath)
		if err != nil {
			continue
		}
		target = strings.TrimSpace(strings.TrimSuffix(target, " (deleted)"))
		if target == "" {
			continue
		}
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}
		if shouldSkipDiscoveredProcess(filepath.Base(target)) {
			continue
		}
		result = append(result, discoveredAppFromPath(target))
	}
	return result
}

func processUID(status []byte) int {
	scanner := bufio.NewScanner(bytes.NewReader(status))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return -1
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil {
			return -1
		}
		return uid
	}
	return -1
}
