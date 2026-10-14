//go:build unix

package androidvpn

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func installTestTun(t *testing.T) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tun")
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(f.Fd()))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = SetTunFD(fd); err != nil {
		syscall.Close(fd)
		t.Fatal(err)
	}
	t.Cleanup(CloseTun)
}
func TestHevExitReleasesTunAndPublishesFailure(t *testing.T) {
	installTestTun(t)
	script := filepath.Join(t.TempDir(), "bridge.sh")
	if err := os.WriteFile(script, []byte("exit 17\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := StartHev("/bin/sh", script); err != nil {
		t.Fatal(err)
	}
	b := CurrentBridge()
	select {
	case <-b.Done():
	case <-time.After(time.Second):
		t.Fatal("bridge did not finish")
	}
	if TunFile() != nil || b.Failure() == nil {
		t.Fatal("dead bridge left TUN open or lost failure")
	}
	StopHev()
	StopHev()
}
func TestReplacingBridgeCannotCloseNewTun(t *testing.T) {
	installTestTun(t)
	script := filepath.Join(t.TempDir(), "bridge.sh")
	if err := os.WriteFile(script, []byte("exec sleep 30\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := StartHev("/bin/sh", script); err != nil {
		t.Fatal(err)
	}
	old := CurrentBridge()
	installTestTun(t)
	if TunFile() == nil {
		t.Fatal("replacement TUN was closed")
	}
	select {
	case <-old.Done():
	case <-time.After(time.Second):
		t.Fatal("old child not reaped")
	}
	if old.Failure() != nil {
		t.Fatal("intentional stop reported failure")
	}
}
