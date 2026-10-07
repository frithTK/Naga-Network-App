//go:build android

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"os"
	"path/filepath"
	"sync"

	"naga.network/core/androidvpn"
	"naga.network/core/control"
)

var (
	mu    sync.Mutex
	local *control.LocalServer
)

func main() {}

func existing(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

//export NagaStart
func NagaStart(dataDir, nativeLibDir *C.char) *C.char {
	dir := C.GoString(dataDir)
	native := C.GoString(nativeLibDir)
	mu.Lock()
	defer mu.Unlock()
	if local != nil {
		return C.CString("")
	}
	srv, err := control.StartLocal(control.LocalOptions{
		Listen:       "127.0.0.1:8765",
		DataDir:      dir,
		TokenPath:    filepath.Join(dir, "control.token"),
		NativeLibDir: native,
		SingBoxPath:  existing(filepath.Join(native, "libsingbox.so")),
		HevPath:      existing(filepath.Join(native, "libhevfd.so")),
		OlcPath:      existing(filepath.Join(native, "libolcrtc.so")),
	})
	if err != nil {
		return C.CString(err.Error())
	}
	local = srv
	return C.CString("")
}

//export NagaStop
func NagaStop() {
	mu.Lock()
	defer mu.Unlock()
	if local != nil {
		local.Stop()
		local = nil
	}
	androidvpn.CloseTun()
}

//export NagaStopRuntime
func NagaStopRuntime() {
	mu.Lock()
	srv := local
	mu.Unlock()
	if srv != nil {
		srv.StopRuntime()
	}
}

//export NagaSetTunFd
func NagaSetTunFd(fd C.int) *C.char {
	if err := androidvpn.SetTunFD(int(fd)); err != nil {
		return C.CString(err.Error())
	}
	return C.CString("")
}

//export NagaCloseTun
func NagaCloseTun() {
	androidvpn.CloseTun()
}
