//go:build ios

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"path/filepath"
	"sync"

	"naga.network/core/control"
)

var (
	mu    sync.Mutex
	local *control.LocalServer
)

func main() {}

//export NagaStart
func NagaStart(dataDir *C.char) *C.char {
	dir := C.GoString(dataDir)
	mu.Lock()
	defer mu.Unlock()
	if local != nil {
		return C.CString("")
	}
	srv, err := control.StartLocal(control.LocalOptions{
		Listen:    "127.0.0.1:8765",
		DataDir:   dir,
		TokenPath: filepath.Join(dir, "control.token"),
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
