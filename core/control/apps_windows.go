//go:build windows

package control

import (
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DiscoverUserProcesses lists unique executables from the current session.
// Matching uses the exe basename, and the full image path when the UI stores
// process_path. System services and the Naga binaries are omitted.
func DiscoverUserProcesses() []DiscoveredApp {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []DiscoveredApp{}
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return []DiscoveredApp{}
	}

	seen := make(map[string]struct{})
	result := make([]DiscoveredApp, 0)
	for {
		exeName := windows.UTF16ToString(entry.ExeFile[:])
		path := windowsProcessImagePath(entry.ProcessID)
		if path == "" {
			path = exeName
		}
		base := processBaseName(path)
		if !shouldSkipDiscoveredProcess(base) {
			key := strings.ToLower(path)
			if _, exists := seen[key]; !exists && strings.TrimSpace(path) != "" {
				seen[key] = struct{}{}
				result = append(result, discoveredAppFromPath(path))
			}
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i].Name)
		right := strings.ToLower(result[j].Name)
		if left == right {
			return strings.ToLower(result[i].ProcessPath) < strings.ToLower(result[j].ProcessPath)
		}
		return left < right
	})
	return result
}

func windowsProcessImagePath(pid uint32) string {
	if pid == 0 {
		return ""
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		handle, err = windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
		if err != nil {
			return ""
		}
	}
	defer windows.CloseHandle(handle)

	var size uint32 = 32768
	buf := make([]uint16, size)
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil || size == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}
