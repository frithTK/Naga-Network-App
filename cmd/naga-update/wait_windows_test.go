//go:build windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWaitForPIDTimesOutThenObservesExit(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestWaitForPIDChild$")
	child.Env = append(os.Environ(), "NAGA_WAIT_TEST_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	if err := waitForPID(child.Process.Pid, time.Millisecond); err == nil || !strings.Contains(err.Error(), "не завершился") {
		t.Fatalf("running process did not return a timeout: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := waitForPID(child.Process.Pid, 5*time.Second); err != nil {
		t.Fatalf("terminated process: %v", err)
	}
}

func TestWaitForPIDChild(t *testing.T) {
	if os.Getenv("NAGA_WAIT_TEST_CHILD") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}
