//go:build linux && !android

package control

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type fakeGSettings struct {
	values map[string]string
	fail   string
}

func (f *fakeGSettings) run(args ...string) (string, error) {
	key := args[1] + "/" + args[2]
	if args[0] == "get" {
		return f.values[key], nil
	}
	if key == f.fail {
		return "", errors.New("injected write failure")
	}
	f.values[key] = args[3]
	return "", nil
}
func corporateProxy() map[string]string {
	return map[string]string{
		"org.gnome.system.proxy/mode":       "'manual'",
		"org.gnome.system.proxy.http/host":  "'corp.example'",
		"org.gnome.system.proxy.http/port":  "uint32 2080",
		"org.gnome.system.proxy.https/host": "'secure.example'",
		"org.gnome.system.proxy.https/port": "uint32 12080",
		"org.gnome.system.proxy.socks/host": "'socks.example'",
		"org.gnome.system.proxy.socks/port": "uint32 10808",
	}
}
func TestGSettingsSystemProxyConfiguresAndRestores(t *testing.T) {
	f := &fakeGSettings{values: corporateProxy()}
	before := corporateProxy()
	restore, err := (&GSettingsSystemProxy{run: f.run}).Configure("127.0.0.1", 2080)
	if err != nil {
		t.Fatal(err)
	}
	if proxyValue(f.values["org.gnome.system.proxy.socks/host"]) != "127.0.0.1" {
		t.Fatal(f.values)
	}
	if err = restore(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.values, before) {
		t.Fatalf("lost original proxy on matching port: %#v", f.values)
	}
}
func TestGSettingsRestorePreservesUserReplacement(t *testing.T) {
	f := &fakeGSettings{values: corporateProxy()}
	restore, err := (&GSettingsSystemProxy{run: f.run}).Configure("127.0.0.1", 2080)
	if err != nil {
		t.Fatal(err)
	}
	f.values = corporateProxy()
	if err = restore(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.values, corporateProxy()) {
		t.Fatal("overwrote user's replacement")
	}
}
func TestGSettingsPartialFailureRestoresAppliedKeys(t *testing.T) {
	f := &fakeGSettings{values: corporateProxy(), fail: "org.gnome.system.proxy.https/host"}
	_, err := (&GSettingsSystemProxy{run: f.run}).Configure("127.0.0.1", 2080)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !reflect.DeepEqual(f.values, corporateProxy()) {
		t.Fatalf("partial update survived failure: %#v", f.values)
	}
}

func TestGSettingsNativeSerializationInPrivateBackend(t *testing.T) {
	if _, err := exec.LookPath("gsettings"); err != nil {
		t.Skip("gsettings unavailable")
	}
	env := append(os.Environ(), "GSETTINGS_BACKEND=keyfile", "XDG_CONFIG_HOME="+t.TempDir())
	run := func(args ...string) (string, error) {
		cmd := exec.Command("gsettings", args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	before := corporateProxy()
	for key, value := range before {
		parts := strings.Split(key, "/")
		if _, err := run("set", parts[0], parts[1], value); err != nil {
			t.Fatal(err)
		}
	}
	restore, err := (&GSettingsSystemProxy{run: run}).Configure("127.0.0.1", 2080)
	if err != nil {
		t.Fatal(err)
	}
	if err = restore(); err != nil {
		t.Fatal(err)
	}
	for key, want := range before {
		parts := strings.Split(key, "/")
		got, err := run("get", parts[0], parts[1])
		if err != nil || proxyValue(got) != proxyValue(want) {
			t.Fatalf("key=%s got=%q want=%q err=%v", key, got, want, err)
		}
	}
}
