//go:build !linux && !windows

package control

// DiscoverUserProcesses is implemented for Linux and Windows; other
// platforms return an empty list rather than failing the control-plane.
func DiscoverUserProcesses() []DiscoveredApp {
	return []DiscoveredApp{}
}
