package control

import "testing"

func TestShouldSkipDiscoveredProcess(t *testing.T) {
	t.Parallel()
	skip := []string{"svchost.exe", "SVCHOST.EXE", "csrss.exe", "naga-control.exe", "naga_network.exe", "NagaNetwork.exe", ""}
	for _, name := range skip {
		if !shouldSkipDiscoveredProcess(name) {
			t.Fatalf("%q must be skipped", name)
		}
	}
	keep := []string{"Telegram.exe", "firefox", "chrome.exe"}
	for _, name := range keep {
		if shouldSkipDiscoveredProcess(name) {
			t.Fatalf("%q must stay visible", name)
		}
	}
}

func TestDiscoveredAppFromPath(t *testing.T) {
	t.Parallel()
	windowsApp := discoveredAppFromPath(`C:\Program Files\Telegram Desktop\Telegram.exe`)
	if windowsApp.Name != "Telegram" || windowsApp.Process != "Telegram.exe" {
		t.Fatalf("windows display = %#v", windowsApp)
	}
	if windowsApp.ProcessPath != `C:\Program Files\Telegram Desktop\Telegram.exe` {
		t.Fatalf("windows process_path = %q", windowsApp.ProcessPath)
	}

	linuxApp := discoveredAppFromPath("/usr/bin/firefox")
	if linuxApp.Name != "firefox" || linuxApp.Process != "firefox" || linuxApp.ProcessPath != "/usr/bin/firefox" {
		t.Fatalf("linux display = %#v", linuxApp)
	}

	bare := discoveredAppFromPath("Telegram.exe")
	if bare.Name != "Telegram" || bare.Process != "Telegram.exe" || bare.ProcessPath != "" {
		t.Fatalf("basename-only = %#v", bare)
	}
}
