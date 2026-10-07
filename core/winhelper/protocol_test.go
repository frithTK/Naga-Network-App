package winhelper

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidPipeName(t *testing.T) {
	if !ValidPipeName("NagaTun-0123456789abcdef") {
		t.Fatal("expected tun pipe")
	}
	if !ValidPipeName("NagaOlc-fedcba9876543210") {
		t.Fatal("expected olc pipe")
	}
	for _, name := range []string{
		"",
		"NagaTun-xyz",
		"NagaTun-0123456789ABCDEF",
		"NagaTun-0123456789abcdef0",
		"NagaTun-0123456789abcde",
		`--tun-host foo`,
		`NagaTun-0123456789abcdef & calc`,
		`../NagaTun-0123456789abcdef`,
	} {
		if ValidPipeName(name) {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestValidateSpawn(t *testing.T) {
	if err := ValidateSpawn(ModeTun, "NagaTun-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSpawn("rm", "NagaTun-0123456789abcdef"); err != ErrInvalidSpawn {
		t.Fatalf("mode = %v", err)
	}
	if err := ValidateSpawn(ModeOlc, "NagaTun-0123456789abcdef extra"); err != ErrInvalidSpawn {
		t.Fatalf("pipe = %v", err)
	}
}

func TestAllowedBundledBinary(t *testing.T) {
	dir := t.TempDir()
	if !AllowedBundledBinary(dir, filepath.Join(dir, "sing-box.exe"), "sing-box.exe") {
		t.Fatal("expected bundled sing-box")
	}
	if AllowedBundledBinary(dir, filepath.Join(dir, "cmd.exe"), "sing-box.exe") {
		t.Fatal("accepted foreign binary")
	}
	if AllowedBundledBinary(dir, filepath.Join(dir, "sing-box.exe", "..", "cmd.exe"), "sing-box.exe") {
		t.Fatal("accepted path escape")
	}
}

func TestHostFlag(t *testing.T) {
	flag, ok := HostFlag(ModeTun)
	if !ok || flag != "--tun-host" {
		t.Fatalf("tun = %q %v", flag, ok)
	}
	flag, ok = HostFlag(ModeOlc)
	if !ok || flag != "--olc-host" {
		t.Fatalf("olc = %q %v", flag, ok)
	}
	if _, ok := HostFlag("--tun-host"); ok {
		t.Fatal("raw flag must be rejected")
	}
}

func TestTaskDefinitionEscapesXML(t *testing.T) {
	xml := TaskDefinition(`C:\Program Files\Naga Network\naga-control.exe`, "S-1-5-21-1-2-3-1001")
	for _, want := range []string{
		"<Command>C:\\Program Files\\Naga Network\\naga-control.exe</Command>",
		"<Arguments>--elevated-helper</Arguments>",
		"<RunLevel>HighestAvailable</RunLevel>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<UserId>S-1-5-21-1-2-3-1001</UserId>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("missing %q in %s", want, xml)
		}
	}
	xml = TaskDefinition(`C:\App\naga&control.exe`, `S-1-5-21-<x>`)
	if !strings.Contains(xml, "naga&amp;control.exe") {
		t.Fatalf("unescaped exe: %s", xml)
	}
	if !strings.Contains(xml, "S-1-5-21-&lt;x&gt;") {
		t.Fatalf("unescaped sid: %s", xml)
	}
	if strings.Contains(xml, "naga&control.exe") {
		t.Fatal("raw ampersand in XML")
	}
}
