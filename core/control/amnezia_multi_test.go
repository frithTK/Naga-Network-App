package control

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
	"naga.network/core/storage"
)

type synchronizedAWG struct {
	arrived chan<- struct{}
	release <-chan struct{}
}

type testPreparedAWG struct {
	*awgengine.LoopbackSidecar
	starts   int
	readyErr error
	probeErr error
}

func (s *testPreparedAWG) Prepare(ctx context.Context) error {
	if s.ListenAddr() != "" {
		return nil
	}
	return s.LoopbackSidecar.Start(ctx, awgcfg.Config{})
}

func (s *testPreparedAWG) Start(ctx context.Context, _ awgcfg.Config) error {
	s.starts++
	return s.Prepare(ctx)
}
func (s *testPreparedAWG) Ready(context.Context) error { return s.readyErr }
func (s *testPreparedAWG) Deactivate()                 {}
func (s *testPreparedAWG) ProbeHTTPS(context.Context, string) (int, error) {
	if s.probeErr != nil {
		return 0, s.probeErr
	}
	return 25, nil
}

func TestManualAWGOnlyStartsSelectedNodeAndActivatesStandbyOnSwitch(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("standby activation requires a protected runtime")
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{ID: "bundle", Engine: profile.EngineSingBox, Config: multiAWGEnvelope(t), SelectedMode: awgcfg.LeafTag}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	entries := profile.AmneziaWGProfiles([]profile.Profile{value})
	runner := &selectableRuntime{}
	controller := NewRuntimeController(&store, &recordingAdapter{runtime: runner})
	controller.HealthInterval = time.Hour
	var sidecars []*testPreparedAWG
	controller.AWGFactory = func() awgengine.Sidecar {
		sidecar := &testPreparedAWG{LoopbackSidecar: awgengine.NewLoopbackSidecar()}
		sidecars = append(sidecars, sidecar)
		return sidecar
	}
	snapshot, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	if snapshot.ActiveNode != entries[0].AWGTag || snapshot.AccessState != "verified" ||
		sidecars[0].starts != 1 || sidecars[1].starts != 0 || sidecars[2].starts != 0 || sidecars[3].starts != 0 {
		t.Fatalf("start = %+v, starts by node = %d/%d/%d/%d",
			snapshot, sidecars[0].starts, sidecars[1].starts, sidecars[2].starts, sidecars[3].starts)
	}
	sidecars[1].readyErr = awgengine.ErrHandshakeTimeout
	if _, err := controller.Select(value.ID, entries[1].AWGTag); !errors.Is(err, awgengine.ErrHandshakeTimeout) {
		t.Fatalf("failed selection error = %v", err)
	}
	if runner.selectedTag() != value.ID+"::"+entries[0].AWGTag {
		t.Fatal("failed switch displaced the active AWG")
	}
	if _, err := controller.Select(value.ID, entries[2].AWGTag); err != nil {
		t.Fatal(err)
	}
	if sidecars[2].starts != 1 || runner.selectedTag() != value.ID+"::"+entries[2].AWGTag {
		t.Fatal("standby was not activated before switching")
	}
}

func TestStrictAWGKeepsTunnelIfHTTPSCannotBeVerified(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{ID: "awg", Engine: profile.EngineAmneziaWG, Config: awgRuntimeTestdata(t, "awg31.valid.conf"), SelectedMode: awgcfg.LeafTag}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	sidecar := &testPreparedAWG{LoopbackSidecar: awgengine.NewLoopbackSidecar(), probeErr: awgengine.ErrHTTPSStatus}
	controller := NewRuntimeController(&store, &recordingAdapter{runtime: &selectableRuntime{}})
	controller.HealthInterval = time.Hour
	controller.AWGFactory = func() awgengine.Sidecar { return sidecar }
	snapshot, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	if snapshot.Status != "connected" || snapshot.AccessState != "unconfirmed" || snapshot.ActiveLatency != 0 {
		t.Fatalf("strict AWG misreported HTTPS status: %+v", snapshot)
	}
}

func (s *synchronizedAWG) Start(context.Context, awgcfg.Config) error { return nil }
func (s *synchronizedAWG) ListenAddr() string                         { return "127.0.0.1:1" }
func (s *synchronizedAWG) Stop() error                                { return nil }
func (s *synchronizedAWG) Ready(ctx context.Context) error {
	s.arrived <- struct{}{}
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestAWGStartupPreparesPeersConcurrently(t *testing.T) {
	arrived := make(chan struct{}, 4)
	release := make(chan struct{})
	controller := &RuntimeController{AWGFactory: func() awgengine.Sidecar {
		return &synchronizedAWG{arrived: arrived, release: release}
	}}
	values := profile.AmneziaWGProfiles([]profile.Profile{{ID: "bundle", Config: multiAWGEnvelope(t)}})
	done := make(chan []awgStartResult, 1)
	go func() { stage := ""; done <- controller.startAWGSidecars(&stage, values, nil) }()
	for range values {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			close(release)
			<-done
			t.Fatal("AWG peers did not start concurrently")
		}
	}
	close(release)
	results := <-done
	defer stopUnclaimedAWG(results)
	for _, result := range results {
		if result.err != nil || result.sidecar == nil {
			t.Fatalf("startup failed: %v", result.err)
		}
	}
}

func multiAWGEnvelope(t *testing.T) []byte {
	t.Helper()
	conf := string(awgRuntimeTestdata(t, "awg31.valid.conf"))
	entries := []any{}
	for _, country := range []string{"PL", "LV", "CA", "CZ"} {
		entries = append(entries, map[string]any{"country": country, "conf": conf + "\n# " + country})
	}
	raw, err := json.Marshal(map[string]any{
		"v": 1, "app": "naga-network",
		"singbox": json.RawMessage(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"}]}`),
		"amnezia": map[string]any{"conf": conf + "\n# PL", "confs": entries},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMultipleAWGNodesCanBeListedSelectedAndRestarted(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{ID: "bundle", Engine: profile.EngineSingBox, Config: multiAWGEnvelope(t), SelectedMode: awgcfg.LeafTag}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, &recordingAdapter{runtime: &selectableRuntime{}})
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.NewLoopbackSidecar() }
	s := Server{Storage: &store, Runtime: controller}
	nodes := s.profileNodes(value)
	awg := profile.AmneziaWGProfiles([]profile.Profile{value})
	if len(awg) != 4 {
		t.Fatalf("AWG count: %d", len(awg))
	}
	for _, entry := range awg {
		found := false
		for _, node := range nodes.Nodes {
			if node.Tag == entry.AWGTag && node.Country == entry.AWGCountry {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing country node: %s", entry.AWGTag)
		}
	}
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	for _, entry := range awg {
		if _, err := controller.Select(value.ID, entry.AWGTag); err != nil {
			t.Fatalf("select %s: %v", entry.AWGTag, err)
		}
		if controller.activeRuntimeTag != value.ID+"::"+entry.AWGTag {
			t.Fatalf("incorrect active node: %s", controller.activeRuntimeTag)
		}
	}
	saved, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedMode != awg[3].AWGTag {
		t.Fatalf("selection not persisted: %q", saved.SelectedMode)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatalf("restart selected AWG: %v", err)
	}
	if !strings.Contains(string(controller.Adapter.(*recordingAdapter).last), value.ID+"::"+awg[3].AWGTag) {
		t.Fatal("selected node not injected on restart")
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAWGCountryNeverSelectsAnother(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{ID: "bundle", Engine: profile.EngineSingBox, Config: multiAWGEnvelope(t)}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	controller := NewRuntimeController(&store, &recordingAdapter{runtime: runtime})
	count := 0
	controller.AWGFactory = func() awgengine.Sidecar {
		count++
		if count < 4 {
			return awgengine.FailingSidecar{}
		}
		return awgengine.NewLoopbackSidecar()
	}
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	entries := profile.AmneziaWGProfiles([]profile.Profile{value})
	if _, err := controller.Select(value.ID, entries[3].AWGTag); err != nil {
		t.Fatalf("working node: %v", err)
	}
	before := runtime.selectedTag()
	if _, err := controller.Select(value.ID, entries[1].AWGTag); err == nil {
		t.Fatal("unavailable country was accepted")
	}
	if _, err := controller.Select(value.ID, awgcfg.LeafTag); err == nil {
		t.Fatal("unavailable legacy country was accepted")
	}
	if runtime.selectedTag() != before {
		t.Fatal("unavailable selection switched to another country")
	}
}

func TestRefreshPreservesSelectedAWGCountry(t *testing.T) {
	raw := multiAWGEnvelope(t)
	entries := profile.AmneziaWGProfiles([]profile.Profile{{Config: raw}})
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	field := document["amnezia"].(map[string]any)
	confs := field["confs"].([]any)
	confs[0], confs[3] = confs[3], confs[0]
	field["conf"] = confs[0].(map[string]any)["conf"]
	reordered, _ := json.Marshal(document)
	selected, ok := remapAmneziaSelection(raw, reordered, awgcfg.LeafTag)
	if !ok || selected != entries[0].AWGTag {
		t.Fatalf("legacy choice changed: %q %v", selected, ok)
	}
	selected, ok = remapAmneziaSelection(raw, reordered, entries[1].AWGTag)
	if !ok || selected != entries[1].AWGTag {
		t.Fatalf("country choice changed: %q %v", selected, ok)
	}
	field["confs"] = []any{confs[0], confs[2], confs[3]}
	removed, _ := json.Marshal(document)
	if _, ok := remapAmneziaSelection(raw, removed, entries[1].AWGTag); ok {
		t.Fatal("removed country accepted")
	}
}
