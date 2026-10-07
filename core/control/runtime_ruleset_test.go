package control

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"naga.network/core/engine"
	"naga.network/core/policy"
	"naga.network/core/profile"
	"naga.network/core/storage"
)

type capturingAdapter struct {
	mu      sync.Mutex
	configs [][]byte
	runtime *testRuntime
	failIf  func([]byte) error
}

func (*capturingAdapter) Type() engine.Type                      { return engine.SingBox }
func (*capturingAdapter) Validate(context.Context, []byte) error { return nil }

func (a *capturingAdapter) Start(_ context.Context, config []byte) (engine.Runtime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configs = append(a.configs, append([]byte(nil), config...))
	if a.failIf != nil {
		if err := a.failIf(config); err != nil {
			return nil, err
		}
	}
	if a.runtime == nil {
		a.runtime = &testRuntime{status: engine.Connected}
	}
	a.runtime.status = engine.Connected
	return a.runtime, nil
}

func (a *capturingAdapter) lastConfig() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.configs) == 0 {
		return nil
	}
	return a.configs[len(a.configs)-1]
}

type countingFetcher struct {
	mu    sync.Mutex
	calls int
	body  []byte
}

func (f *countingFetcher) FetchBytes(context.Context, string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]byte(nil), f.body...), nil
}

func (f *countingFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func providerRuleSetConfig() []byte {
	return []byte(`{"outbounds":[{"type":"direct","tag":"direct"},{"type":"vless","tag":"node"},{"type":"selector","tag":"Mode","outbounds":["node"]}],"route":{"final":"Mode","rule_set":[{"tag":"sub-ru","type":"remote","format":"source","url":"https://example.invalid/ru.json","download_detour":"direct"}],"rules":[{"rule_set":"sub-ru","outbound":"direct"}]}}`)
}

func TestStartMaterializesProviderRuleSets(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-ruleset",
		Engine: profile.EngineSingBox,
		Config: providerRuleSetConfig(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	routing := policy.DefaultRoutingPolicy()
	routing.ProviderRules = true
	if err := store.SaveRoutingPolicy(routing); err != nil {
		t.Fatal(err)
	}
	fetcher := &countingFetcher{body: []byte(`{"version":1,"rules":[{"domain_suffix":[".example"]}]}`)}
	adapter := &capturingAdapter{}
	controller := NewRuntimeController(&store, adapter)
	controller.RuleSets.Fetcher = fetcher
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	if fetcher.callCount() != 1 {
		t.Fatalf("fetch calls = %d", fetcher.callCount())
	}
	var document map[string]any
	if err := json.Unmarshal(adapter.lastConfig(), &document); err != nil {
		t.Fatal(err)
	}
	item := document["route"].(map[string]any)["rule_set"].([]any)[0].(map[string]any)
	if item["type"] != "local" {
		t.Fatalf("type = %#v", item["type"])
	}
	path, _ := item["path"].(string)
	if !filepath.IsAbs(path) {
		t.Fatalf("path = %q", path)
	}
	direct := false
	for _, raw := range document["route"].(map[string]any)["rules"].([]any) {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if rule["outbound"] != "direct" {
			continue
		}
		if _, ok := rule["rule_set"]; ok {
			direct = true
		}
	}
	if !direct {
		t.Fatalf("provider direct rule missing: %#v", document["route"])
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartSkipsRuleSetFetchWhenProviderRulesDisabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-ruleset-off",
		Engine: profile.EngineSingBox,
		Config: providerRuleSetConfig(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	routing := policy.DefaultRoutingPolicy()
	routing.ProviderRules = false
	routing.BuiltinRU = true
	if err := store.SaveRoutingPolicy(routing); err != nil {
		t.Fatal(err)
	}
	fetcher := &countingFetcher{body: []byte(`{"version":1,"rules":[]}`)}
	adapter := &capturingAdapter{}
	controller := NewRuntimeController(&store, adapter)
	controller.RuleSets.Fetcher = fetcher
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	if fetcher.callCount() != 0 {
		t.Fatalf("fetch calls = %d", fetcher.callCount())
	}
	var document map[string]any
	if err := json.Unmarshal(adapter.lastConfig(), &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("provider rule_set remained: %#v", route["rule_set"])
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartRetriesWithoutLocalRuleSets(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-ruleset-retry",
		Engine: profile.EngineSingBox,
		Config: providerRuleSetConfig(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	fetcher := &countingFetcher{body: []byte(`{"version":1,"rules":[{"domain_suffix":[".example"]}]}`)}
	adapter := &capturingAdapter{
		failIf: func(config []byte) error {
			if strings.Contains(string(config), `"type":"local"`) {
				return errors.New("invalid rule-set local path")
			}
			return nil
		},
	}
	controller := NewRuntimeController(&store, adapter)
	controller.RuleSets.Fetcher = fetcher
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	starts := len(adapter.configs)
	adapter.mu.Unlock()
	if starts != 2 {
		t.Fatalf("starts = %d, want retry", starts)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuleSetStartFailed(t *testing.T) {
	if !ruleSetStartFailed(errors.New("check: invalid rule-set")) {
		t.Fatal("expected rule-set failure")
	}
	if ruleSetStartFailed(errors.New("listen tcp :443")) {
		t.Fatal("unrelated error")
	}
}
