package control

import (
	"context"
	"naga.network/core/engine"
	"naga.network/core/profile"
	"naga.network/core/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cancelBlockingRuntime struct {
	entered, release chan struct{}
	once             sync.Once
	stopped          atomic.Bool
}

func (r *cancelBlockingRuntime) Stop() error { r.stopped.Store(true); return nil }
func (r *cancelBlockingRuntime) Status() engine.Status {
	if r.stopped.Load() {
		return engine.Stopped
	}
	return engine.Connected
}
func (r *cancelBlockingRuntime) Stats() engine.TrafficStats           { return engine.TrafficStats{} }
func (r *cancelBlockingRuntime) Select(context.Context, string) error { return nil }
func (r *cancelBlockingRuntime) Probe(ctx context.Context, _ string, _ time.Duration) (int, error) {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-r.release:
		return 1, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

type cancelBlockingAdapter struct{ r *cancelBlockingRuntime }

func (a cancelBlockingAdapter) Type() engine.Type                      { return engine.SingBox }
func (a cancelBlockingAdapter) Validate(context.Context, []byte) error { return nil }
func (a cancelBlockingAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	return a.r, nil
}

func TestHTTPStopCancelsStartingRuntime(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"outbounds":[{"type":"selector","tag":"proxy","outbounds":["CZ VLESS"],"default":"CZ VLESS"},{"type":"vless","tag":"CZ VLESS","server":"example.invalid","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000"}],"route":{"final":"proxy"}}`)
	if err = store.Save(profile.Profile{ID: "audit", Engine: profile.EngineSingBox, SelectedMode: "auto", Config: config}); err != nil {
		t.Fatal(err)
	}
	runtime := &cancelBlockingRuntime{entered: make(chan struct{}), release: make(chan struct{})}
	controller := NewRuntimeController(&store, cancelBlockingAdapter{runtime})
	controller.HealthInterval = time.Hour
	api := Server{Storage: &store, Runtime: controller, MutationMu: &sync.Mutex{}}
	started := make(chan struct{})
	startResponse := httptest.NewRecorder()
	go func() {
		defer close(started)
		api.ServeHTTP(startResponse, httptest.NewRequest(http.MethodPost, "/v1/runtime/start", strings.NewReader(`{"profile_id":"audit"}`)))
	}()
	select {
	case <-runtime.entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("startup never entered probe: response=%d %s", startResponse.Code, startResponse.Body.String())
	}
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		api.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/runtime/stop", nil))
	}()
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Error("HTTP stop is serialized behind startup instead of aborting it")
	}
	close(runtime.release)
	<-started
	<-stopDone
}

type contextBlockingAdapter struct{ entered chan struct{} }

func (a contextBlockingAdapter) Type() engine.Type                      { return engine.SingBox }
func (a contextBlockingAdapter) Validate(context.Context, []byte) error { return nil }
func (a contextBlockingAdapter) Start(ctx context.Context, _ []byte) (engine.Runtime, error) {
	close(a.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestHTTPStopCancelsEngineAcquisitionAndBlocksReplacement(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	adapter := contextBlockingAdapter{entered: make(chan struct{})}
	controller := NewRuntimeController(&store, adapter)
	api := Server{Storage: &store, Runtime: controller, Token: "test", MutationMu: &sync.Mutex{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/v1/runtime/start", strings.NewReader(`{"profile_id":"`+value.ID+`"}`))
		req.Header.Set("Authorization", "Bearer test")
		api.ServeHTTP(httptest.NewRecorder(), req)
	}()
	select {
	case <-adapter.entered:
	case <-time.After(time.Second):
		t.Fatal("start never reached engine")
	}
	unauthorized := httptest.NewRecorder()
	api.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/runtime/stop", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatal("unauthorized stop was accepted")
	}
	controller.operationMu.Lock()
	hasStart := controller.startCancel != nil
	controller.operationMu.Unlock()
	if !hasStart {
		t.Fatal("unauthorized request interrupted startup")
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		req := httptest.NewRequest(http.MethodPost, "/v1/runtime/stop", nil)
		req.Header.Set("Authorization", "Bearer test")
		api.ServeHTTP(httptest.NewRecorder(), req)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop cannot cancel blocked engine startup")
	}
	<-done
	if controller.Status().Status != string(engine.Stopped) {
		t.Fatal("runtime survived cancellation")
	}
	finish := controller.prepareStop()
	_, err := controller.Start(value.ID)
	finish()
	if err == nil {
		t.Fatal("replacement was allowed during pending Stop")
	}
}
