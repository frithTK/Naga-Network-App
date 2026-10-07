//go:build linux && !android

package control

import "os"

// DiscoverUserProcesses lists unique executables of the service user.
// /proc/<pid>/comm is truncated to 15 bytes, so matching uses the exe
// basename (and the full path when the UI stores process_path).
func DiscoverUserProcesses() []DiscoveredApp {
	return discoverProcesses("/proc", os.Getuid(), os.Readlink, os.ReadFile)
}
