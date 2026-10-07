//go:build android

package control

// DiscoverUserProcesses is empty on Android. The UI lists packages through
// PackageManager and writes the chosen ids back to routing policy.
func DiscoverUserProcesses() []DiscoveredApp {
	return []DiscoveredApp{}
}
