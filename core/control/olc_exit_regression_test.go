//go:build !windows

package control

import (
	"context"
	"naga.network/core/engine"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/profile"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOlcExitNeverRemainsConnected(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "olcrtc")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 17\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "olcrtc.version"), []byte(engineolc.PinnedCommit), 0600); err != nil {
		t.Fatal(err)
	}
	proc, err := engineolc.Start(context.Background(), bin, "unused")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(time.Second):
		t.Fatal("child did not exit")
	}
	c := &RuntimeController{olcProc: proc, activeEngine: profile.EngineOlcRTC, profileID: "fixture"}
	if c.Status().Status != string(engine.Failed) {
		t.Fatal("exited process reported connected")
	}
	c.mu.Lock()
	c.startOlcStatsLocked()
	c.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		clean := c.olcProc == nil
		c.mu.Unlock()
		if clean {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dead session was not cleaned")
		}
		time.Sleep(time.Millisecond)
	}
	if c.Status().Status != string(engine.Failed) || c.Status().Error == "" {
		t.Fatal("lost failure after cleanup")
	}
	if _, err = c.Stop(); err != nil {
		t.Fatal(err)
	}
	if c.Status().Status != string(engine.Stopped) {
		t.Fatal("explicit Stop did not clear failure")
	}
}
