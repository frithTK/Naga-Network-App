package singbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	"naga.network/core/profile"
)

var ErrBinaryNotConfigured = errors.New("sing-box binary is not configured")

const (
	// connectivityProbeURL — цель Clash GET /proxies/{tag}/delay.
	// HTTP generate_204 без лишнего TLS через прокси. Latency — второй
	// успешный /delay (unified-delay на Clash API). При ошибке второго
	// замера сохраняется первый.
	connectivityProbeURL  = "http://www.gstatic.com/generate_204"
	maxFailureOutputBytes = 64 * 1024
	maxRuntimeLogLine     = 16 * 1024
)

type Adapter struct {
	BinaryPath   string
	ConfigDir    string
	Args         []string
	Env          []string
	Journal      *diagnostics.Logger
	Starter      processStarter
	ReadyTimeout time.Duration
}

func (Adapter) Type() engine.Type {
	return engine.SingBox
}

func (a Adapter) Validate(ctx context.Context, config []byte) error {
	normalized, err := NormalizeLinuxConfig(config)
	if err != nil {
		return err
	}
	if err := profile.ValidateSingBoxConfig(normalized); err != nil {
		return err
	}
	if a.BinaryPath == "" {
		return nil
	}
	return a.checkConfig(ctx, normalized)
}

func (a Adapter) Start(ctx context.Context, config []byte) (engine.Runtime, error) {
	if a.Journal != nil {
		a.Journal.Info("sing-box", "start_requested", "Запрошен запуск sing-box", nil)
	}
	normalized, err := NormalizeLinuxConfig(config)
	if err != nil {
		return nil, err
	}
	normalized, err = adaptTUNForPlatform(normalized)
	if err != nil {
		return nil, err
	}
	if err := profile.ValidateSingBoxConfig(normalized); err != nil {
		return nil, err
	}
	tunInterfaces, err := linuxTUNInterfaceNames(normalized)
	if err != nil {
		return nil, err
	}
	if a.BinaryPath == "" {
		return nil, ErrBinaryNotConfigured
	}

	configDir := a.ConfigDir
	removeConfigDir := false
	if configDir == "" {
		var err error
		configDir, err = os.MkdirTemp("", "naga-singbox-")
		if err != nil {
			return nil, err
		}
		removeConfigDir = true
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	args := a.Args
	api := (*runtimeAPI)(nil)
	if len(args) == 0 {
		normalized, api, err = prepareRuntimeConfig(normalized)
		if err != nil {
			if removeConfigDir {
				_ = os.RemoveAll(configDir)
			}
			return nil, err
		}
		api.journal = a.Journal
	}

	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, normalized, 0600); err != nil {
		if removeConfigDir {
			_ = os.RemoveAll(configDir)
		}
		return nil, err
	}
	if err := restrictConfigAccess(configDir, configPath); err != nil {
		if removeConfigDir {
			_ = os.RemoveAll(configDir)
		}
		return nil, err
	}

	if len(args) == 0 {
		if err := a.checkConfigFile(ctx, configPath, configDir); err != nil {
			if removeConfigDir {
				_ = os.RemoveAll(configDir)
			}
			return nil, err
		}
		args = []string{"run", "-c", configPath}
	}
	logPath := filepath.Join(configDir, "sing-box.log")
	stderr := newTailBuffer(maxFailureOutputBytes)
	logWriters := make([]*runtimeLogWriter, 0, 2)
	var stdout io.Writer = io.Discard
	var stderrWriter io.Writer = stderr
	if a.Journal != nil {
		stdoutLog := newRuntimeLogWriter(a.Journal, "stdout")
		stderrLog := newRuntimeLogWriter(a.Journal, "stderr")
		stdout = stdoutLog
		stderrWriter = io.MultiWriter(stderr, stderrLog)
		logWriters = append(logWriters, stdoutLog, stderrLog)
	}
	proc, err := a.launch(ctx, processRequest{
		Binary:     a.BinaryPath,
		Args:       args,
		Dir:        configDir,
		Env:        a.Env,
		ConfigPath: configPath,
		LogPath:    logPath,
		Stdout:     stdout,
		Stderr:     stderrWriter,
	})
	if err != nil {
		if removeConfigDir {
			_ = os.RemoveAll(configDir)
		}
		return nil, wrapStartError(err)
	}
	if a.Journal != nil {
		fields := map[string]any{}
		if pid := commandPID(proc); pid != 0 {
			fields["pid"] = pid
		}
		a.Journal.Info("sing-box", "process_started", "Процесс sing-box запущен", fields)
	}

	runtime := &processRuntime{
		proc:            proc,
		configDir:       configDir,
		removeConfigDir: removeConfigDir,
		api:             api,
		stderr:          stderr,
		logWriters:      logWriters,
		journal:         a.Journal,
		status:          engine.Starting,
		done:            make(chan struct{}),
	}
	go runtime.wait()
	if api != nil {
		if err := runtime.waitReady(ctx, a.readyTimeout()); err != nil {
			_ = runtime.Stop()
			return nil, err
		}
		if a.Journal != nil {
			a.Journal.Info("sing-box", "api_ready", "Локальный API sing-box готов", nil)
		}
		if err := configureLinuxTUNInterfaces(tunInterfaces, a.Journal); err != nil {
			_ = runtime.Stop()
			return nil, err
		}
		runtime.startTelemetry()
	} else {
		select {
		case <-runtime.done:
			// Custom Args are primarily used by embedders and tests. Preserve
			// the runtime object so callers can inspect an unexpected exit.
			return runtime, nil
		case <-time.After(50 * time.Millisecond):
		}
		if err := runtime.Failure(); err != nil {
			_ = runtime.Stop()
			return nil, err
		}
	}
	runtime.mu.Lock()
	if runtime.status == engine.Starting {
		runtime.status = engine.Connected
	}
	runtime.mu.Unlock()
	return runtime, nil
}

func (a Adapter) checkConfig(ctx context.Context, config []byte) error {
	directory, err := os.MkdirTemp("", "naga-singbox-check-")
	if err != nil {
		return fmt.Errorf("create config check directory: %w", err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, config, 0600); err != nil {
		return fmt.Errorf("write config for check: %w", err)
	}
	return a.checkConfigFile(ctx, path, directory)
}

func (a Adapter) checkConfigFile(ctx context.Context, path, directory string) error {
	check := exec.CommandContext(ctx, a.BinaryPath, "check", "-c", path)
	check.Dir = directory
	if len(a.Env) > 0 {
		check.Env = append(os.Environ(), a.Env...)
	}
	check.Stdout = io.Discard
	var stderr bytes.Buffer
	check.Stderr = &stderr
	if err := check.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("sing-box config check failed: %w: %s", err, message)
		}
		return fmt.Errorf("sing-box config check failed: %w", err)
	}
	return nil
}

type processRuntime struct {
	mu              sync.RWMutex
	proc            startedProcess
	configDir       string
	removeConfigDir bool
	api             *runtimeAPI
	stderr          *tailBuffer
	logWriters      []*runtimeLogWriter
	journal         *diagnostics.Logger
	status          engine.Status
	failure         error
	stopRequested   bool
	done            chan struct{}
	stopOnce        sync.Once
}

func (r *processRuntime) wait() {
	err := error(nil)
	if r.proc != nil {
		err = r.proc.Wait()
	}
	for _, writer := range r.logWriters {
		writer.Flush()
	}
	r.stopTelemetry()
	r.mu.Lock()
	r.failure = err
	if r.stopRequested {
		r.status = engine.Stopped
	} else {
		r.status = engine.Failed
		if r.failure == nil {
			r.failure = errors.New("sing-box process exited unexpectedly")
		}
		if message := strings.TrimSpace(r.stderr.String()); message != "" {
			r.failure = fmt.Errorf("%v: %s", r.failure, message)
		}
	}
	failure := r.failure
	stopRequested := r.stopRequested
	r.mu.Unlock()
	if r.journal != nil {
		fields := map[string]any{"requested": stopRequested}
		if failure != nil && !stopRequested {
			r.journal.Error("sing-box", "process_exited", failure.Error(), fields)
		} else {
			r.journal.Info("sing-box", "process_exited", "Процесс sing-box остановлен", fields)
		}
	}
	if r.removeConfigDir {
		_ = os.RemoveAll(r.configDir)
	}
	close(r.done)
}

func (r *processRuntime) waitReady(ctx context.Context, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := r.Failure(); err != nil {
			return err
		}
		if r.api.ready() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.done:
			if err := r.Failure(); err != nil {
				return err
			}
			return errors.New("sing-box process stopped before becoming ready")
		case <-deadline.C:
			return annotateReadyTimeout(r, "sing-box did not become ready in time")
		case <-ticker.C:
		}
	}
}

func annotateReadyTimeout(runtime *processRuntime, prefix string) error {
	extra := ""
	if runtime != nil && runtime.stderr != nil {
		extra = strings.TrimSpace(runtime.stderr.String())
	}
	if extra == "" && runtime != nil && runtime.configDir != "" {
		if raw, err := os.ReadFile(filepath.Join(runtime.configDir, "sing-box.log")); err == nil {
			extra = strings.TrimSpace(tailBytes(raw, 4096))
		}
	}
	if extra == "" {
		return errors.New(prefix)
	}
	if runtime != nil && runtime.journal != nil {
		runtime.journal.Error("sing-box", "process_log", extra, map[string]any{"stream": "stderr"})
	}
	return fmt.Errorf("%s: %s", prefix, extra)
}

func tailBytes(raw []byte, limit int) string {
	if len(raw) <= limit {
		return string(raw)
	}
	return string(raw[len(raw)-limit:])
}

func (r *processRuntime) Stop() error {
	var stopErr error
	r.stopOnce.Do(func() {
		if r.journal != nil {
			r.journal.Info("sing-box", "stop_requested", "Запрошена остановка sing-box", nil)
		}
		r.stopTelemetry()
		r.mu.Lock()
		if r.status == engine.Stopped || r.status == engine.Failed {
			r.mu.Unlock()
			return
		}
		r.status = engine.Stopping
		r.stopRequested = true
		if r.proc != nil {
			stopErr = r.proc.SignalStop()
			if errors.Is(stopErr, os.ErrProcessDone) {
				stopErr = nil
			}
		}
		r.mu.Unlock()
		select {
		case <-r.done:
		case <-time.After(2 * time.Second):
			if r.proc != nil {
				killErr := r.proc.Kill()
				if errors.Is(killErr, os.ErrProcessDone) {
					killErr = nil
				}
				if stopErr == nil {
					stopErr = killErr
				}
			}
			<-r.done
		}
	})
	return stopErr
}

func (r *processRuntime) Status() engine.Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *processRuntime) Stats() engine.TrafficStats {
	if r.api != nil {
		return r.api.Stats()
	}
	return engine.TrafficStats{}
}

func (r *processRuntime) Details() engine.RuntimeDetails {
	if r.api != nil {
		return r.api.Details()
	}
	return engine.RuntimeDetails{}
}

func (r *processRuntime) startTelemetry() {
	if r.api != nil {
		r.api.Start()
	}
}

func (r *processRuntime) stopTelemetry() {
	if r.api != nil {
		r.api.Close()
	}
}

func (r *processRuntime) Failure() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.status != engine.Failed {
		return nil
	}
	return r.failure
}

func (r *processRuntime) Select(ctx context.Context, outbound string) error {
	if r.api == nil {
		return errors.New("sing-box runtime API is unavailable")
	}
	if err := r.api.Select(ctx, outbound); err != nil {
		return err
	}
	r.api.refreshDetails()
	return nil
}

func (r *processRuntime) Probe(ctx context.Context, outbound string, timeout time.Duration) (int, error) {
	if r.api == nil {
		return 0, errors.New("sing-box runtime API is unavailable")
	}
	return r.api.Probe(ctx, outbound, timeout)
}

type runtimeAPI struct {
	baseURL        string
	secret         string
	client         http.Client
	modeTag        string
	nodeMetadata   map[string]profile.Node
	selectionPaths map[string][]profile.SelectorChoice
	journal        *diagnostics.Logger

	mu           sync.RWMutex
	stats        engine.TrafficStats
	details      engine.RuntimeDetails
	stopCh       chan struct{}
	closeOnce    sync.Once
	activeCancel context.CancelFunc
}

func (a *runtimeAPI) Stats() engine.TrafficStats {
	a.mu.RLock()
	stats := a.stats
	a.mu.RUnlock()
	if !stats.LastUpdated.IsZero() && time.Since(stats.LastUpdated) > 3*time.Second {
		stats.UploadRate = 0
		stats.DownloadRate = 0
		stats.Available = false
	}
	return stats
}

func (a *runtimeAPI) Details() engine.RuntimeDetails {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.details
}

func (a *runtimeAPI) Select(ctx context.Context, outbound string) error {
	outbound = strings.TrimSpace(outbound)
	a.mu.RLock()
	choices, ok := a.selectionPaths[outbound]
	if !ok || len(choices) == 0 {
		if tag, path, found := lookupSelectionPath(a.selectionPaths, a.nodeMetadata, outbound); found {
			outbound = tag
			choices = path
			ok = true
		}
	}
	a.mu.RUnlock()
	if !ok || len(choices) == 0 {
		return fmt.Errorf("outbound %q is not selectable", outbound)
	}
	for _, choice := range choices {
		payload := map[string]string{"name": choice.Outbound}
		if err := a.postJSON(ctx, "/proxies/"+url.PathEscape(choice.Selector), payload); err != nil {
			return err
		}
	}
	return nil
}

func lookupSelectionPath(
	paths map[string][]profile.SelectorChoice,
	metadata map[string]profile.Node,
	outbound string,
) (string, []profile.SelectorChoice, bool) {
	if path, ok := paths[outbound]; ok && len(path) > 0 {
		return outbound, path, true
	}
	matches := make([]string, 0, 1)
	seen := map[string]struct{}{}
	for tag, path := range paths {
		if len(path) == 0 {
			continue
		}
		matched := strings.HasSuffix(tag, "::"+outbound)
		if !matched {
			if node, ok := metadata[tag]; ok && node.Tag == outbound {
				matched = true
			}
		}
		if !matched {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		matches = append(matches, tag)
	}
	if len(matches) != 1 {
		return outbound, nil, false
	}
	return matches[0], paths[matches[0]], true
}

// Probe делает два Clash /delay. Первый — холодный, второй — приближение
// unified-delay. Ошибка первого сразу возвращается; ошибка второго
// сохраняет первый результат. Каждый замер получает свой timeout+500ms,
// оба шота не оборачиваются одним дедлайном.
func (a *runtimeAPI) Probe(ctx context.Context, outbound string, timeout time.Duration) (int, error) {
	first, err := a.probeOnce(ctx, outbound, timeout)
	if err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		return first, nil
	}
	second, err := a.probeOnce(ctx, outbound, timeout)
	if err != nil || second <= 0 {
		return first, nil
	}
	return second, nil
}

func (a *runtimeAPI) probeOnce(ctx context.Context, outbound string, timeout time.Duration) (int, error) {
	outbound = strings.TrimSpace(outbound)
	if outbound == "" {
		return 0, errors.New("outbound is empty")
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	probeContext, cancel := context.WithTimeout(ctx, timeout+500*time.Millisecond)
	defer cancel()

	endpoint, err := url.Parse(a.baseURL + "/proxies/" + url.PathEscape(outbound) + "/delay")
	if err != nil {
		return 0, err
	}
	query := endpoint.Query()
	query.Set("url", connectivityProbeURL)
	query.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(probeContext, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	client := http.Client{Transport: a.client.Transport}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("outbound probe returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode outbound probe: %w", err)
	}
	if payload.Delay <= 0 {
		return 0, errors.New("outbound probe returned no latency")
	}
	return payload.Delay, nil
}

func (a *runtimeAPI) Start() {
	a.mu.Lock()
	if a.stopCh != nil {
		a.mu.Unlock()
		return
	}
	a.stopCh = make(chan struct{})
	a.mu.Unlock()
	go a.trafficLoop()
	go a.detailsLoop()
}

func (a *runtimeAPI) Close() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		stopCh := a.stopCh
		cancel := a.activeCancel
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if stopCh != nil {
			close(stopCh)
		}
	})
}

func (a *runtimeAPI) trafficLoop() {
	for {
		_ = a.readTrafficStream()
		select {
		case <-a.stopChannel():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (a *runtimeAPI) readTrafficStream() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.activeCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.activeCancel = nil
		a.mu.Unlock()
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/traffic", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	streamClient := http.Client{Transport: a.client.Transport}
	response, err := streamClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("traffic API returned HTTP %d", response.StatusCode)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for scanner.Scan() {
		var payload struct {
			Upload   int64 `json:"up"`
			Download int64 `json:"down"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
			continue
		}
		if payload.Upload < 0 {
			payload.Upload = 0
		}
		if payload.Download < 0 {
			payload.Download = 0
		}
		now := time.Now().UTC()
		a.mu.Lock()
		if a.stats.SessionStarted.IsZero() {
			a.stats.SessionStarted = now
		}
		a.stats.Upload += payload.Upload
		a.stats.Download += payload.Download
		a.stats.UploadRate = payload.Upload
		a.stats.DownloadRate = payload.Download
		a.stats.LastUpdated = now
		a.stats.Available = true
		a.mu.Unlock()
	}
	return scanner.Err()
}

func (a *runtimeAPI) detailsLoop() {
	a.refreshDetails()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopChannel():
			return
		case <-ticker.C:
			a.refreshDetails()
		}
	}
}

type clashProxyState struct {
	Type    string `json:"type"`
	Now     string `json:"now"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history"`
}

// clashLeafSelection walks Mode → selector → URLTest and returns the leaf
// outbound. Group history is ignored: URLTest delay is an interval sample, not
// the last HTTP probe of the selected node.
func clashLeafSelection(proxies map[string]clashProxyState, modeTag string) (leaf string, latency int) {
	current := modeTag
	visited := make(map[string]bool)
	for step := 0; step < 16 && current != "" && !visited[current]; step++ {
		visited[current] = true
		proxy, ok := proxies[current]
		if !ok {
			return current, 0
		}
		if proxy.Now == "" || proxy.Now == current {
			if n := len(proxy.History); n > 0 {
				if last := proxy.History[n-1].Delay; last > 0 {
					return current, last
				}
			}
			return current, 0
		}
		current = proxy.Now
	}
	return current, 0
}

func (a *runtimeAPI) refreshDetails() {
	var payload struct {
		Proxies map[string]clashProxyState `json:"proxies"`
	}
	if err := a.getJSON("/proxies", &payload); err != nil {
		a.mu.Lock()
		previousStatus := a.details.Status
		if a.details.Node == "" {
			a.details.Status = "unavailable"
		}
		a.mu.Unlock()
		if previousStatus != "unavailable" && a.journal != nil {
			a.journal.Warn("sing-box", "telemetry_unavailable", "Не удалось прочитать состояние выбранного узла", nil)
		}
		return
	}

	current, latency := clashLeafSelection(payload.Proxies, a.modeTag)

	a.mu.Lock()
	previous := a.details
	details := engine.RuntimeDetails{Node: current, Latency: latency}
	if metadata, ok := a.nodeMetadata[current]; ok {
		details.Node = metadata.Tag
		details.Country = metadata.Country
		details.Protocol = metadata.Protocol
		details.Status = "active"
	} else if current == a.modeTag {
		details.Status = "checking"
	} else if proxy, ok := payload.Proxies[current]; ok {
		details.Protocol = proxy.Type
		details.Status = "active"
	} else {
		details.Status = "unavailable"
	}
	a.details = details
	a.mu.Unlock()
	if a.journal != nil && (previous.Node != details.Node || previous.Status != details.Status || previous.Protocol != details.Protocol) {
		a.journal.Info("sing-box", "active_node_changed", "Изменилось состояние активного VPN-узла", map[string]any{
			"node":       details.Node,
			"country":    details.Country,
			"protocol":   details.Protocol,
			"latency_ms": details.Latency,
			"status":     details.Status,
		})
	}
}

func (a *runtimeAPI) getJSON(path string, target any) error {
	request, err := http.NewRequest(http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("sing-box API returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (a *runtimeAPI) postJSON(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, a.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("sing-box API returned HTTP %d while selecting outbound", response.StatusCode)
	}
	return nil
}

func (a *runtimeAPI) stopChannel() <-chan struct{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stopCh
}

func (a *runtimeAPI) ready() bool {
	request, err := http.NewRequest(http.MethodGet, a.baseURL+"/", nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	client := a.client
	client.Timeout = 300 * time.Millisecond
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return true
}

func prepareRuntimeConfig(config []byte) ([]byte, *runtimeAPI, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, nil, fmt.Errorf("decode runtime config: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("allocate sing-box API port: %w", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()

	secretBytes := make([]byte, 16)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, nil, fmt.Errorf("generate sing-box API secret: %w", err)
	}
	secret := hex.EncodeToString(secretBytes)
	// The loopback mixed inbound used for subscription refresh is not a DNS
	// listener. systemd-resolved still needs dns-in on 127.0.0.1:53 whenever
	// a TUN interface is the system default route.
	if shouldPrepareSystemResolver() && (hasTUNInbound(document) || !hasSystemProxyInbound(document)) {
		prepareSystemResolverInbound(document)
	}

	experimental, _ := document["experimental"].(map[string]any)
	if experimental == nil {
		experimental = map[string]any{}
		document["experimental"] = experimental
	}
	experimental["clash_api"] = map[string]any{
		"external_controller":                  address,
		"secret":                               secret,
		"access_control_allow_origin":          []any{},
		"access_control_allow_private_network": false,
	}

	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("encode runtime config: %w", err)
	}
	options, _ := profile.InspectMode(result)
	metadata, _ := profile.InspectOutboundMetadata(result)
	selectionPaths := make(map[string][]profile.SelectorChoice)
	for tag := range metadata {
		if path, err := profile.SelectionPath(result, tag); err == nil && len(path) > 0 {
			selectionPaths[tag] = path
		}
	}
	return result, &runtimeAPI{
		baseURL: "http://" + address,
		secret:  secret,
		client: http.Client{
			Transport: &http.Transport{
				Proxy:               nil,
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 16,
				MaxConnsPerHost:     16,
			},
			Timeout: 800 * time.Millisecond,
		},
		modeTag:        options.Tag,
		nodeMetadata:   metadata,
		selectionPaths: selectionPaths,
		details:        engine.RuntimeDetails{Status: "checking"},
	}, nil
}

func hasTUNInbound(document map[string]any) bool {
	for _, rawInbound := range inbounds(document) {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		if inboundType, _ := inbound["type"].(string); inboundType == "tun" {
			return true
		}
	}
	return false
}

func hasSystemProxyInbound(document map[string]any) bool {
	for _, rawInbound := range inbounds(document) {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		if tag, _ := inbound["tag"].(string); tag == "naga-system-proxy" {
			return true
		}
	}
	return false
}

// systemd-resolved can only forward DNS to an IP address on port 53. Keep the
// profile's source config untouched, but expose the runtime DNS inbound on a
// loopback address and the standard resolver port.
func prepareSystemResolverInbound(document map[string]any) {
	rawInbounds, ok := document["inbounds"].([]any)
	if !ok {
		rawInbounds = []any{}
	}
	haveDNSInbound := false
	for _, rawInbound := range rawInbounds {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := inbound["tag"].(string)
		if tag != "dns-in" {
			continue
		}
		haveDNSInbound = true
		inbound["listen"] = "127.0.0.1"
		inbound["listen_port"] = float64(53)
	}
	if haveDNSInbound {
		return
	}
	document["inbounds"] = append(rawInbounds, map[string]any{
		"type":             "direct",
		"tag":              "dns-in",
		"listen":           "127.0.0.1",
		"listen_port":      float64(53),
		"override_address": "8.8.8.8",
		"override_port":    float64(53),
	})
}

var _ engine.FailureReporter = (*processRuntime)(nil)
var _ engine.SelectorSwitcher = (*processRuntime)(nil)
var _ engine.OutboundProber = (*processRuntime)(nil)

var _ engine.Adapter = Adapter{}

type tailBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, value...)
	if len(b.data) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}
	return len(value), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

type runtimeLogWriter struct {
	mu      sync.Mutex
	journal *diagnostics.Logger
	stream  string
	pending []byte
}

func newRuntimeLogWriter(journal *diagnostics.Logger, stream string) *runtimeLogWriter {
	return &runtimeLogWriter{journal: journal, stream: stream}
}

func (w *runtimeLogWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, value...)
	for {
		separator := bytes.IndexByte(w.pending, '\n')
		if separator < 0 {
			break
		}
		w.logLineLocked(string(w.pending[:separator]))
		w.pending = w.pending[separator+1:]
	}
	if len(w.pending) > maxRuntimeLogLine {
		w.logLineLocked(string(w.pending[:maxRuntimeLogLine]))
		w.pending = w.pending[maxRuntimeLogLine:]
	}
	return len(value), nil
}

func (w *runtimeLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	w.logLineLocked(string(w.pending))
	w.pending = nil
}

func (w *runtimeLogWriter) logLineLocked(line string) {
	line = strings.TrimSpace(line)
	if line == "" || w.journal == nil {
		return
	}
	fields := map[string]any{"stream": w.stream}
	upper := strings.ToUpper(line)
	switch {
	case strings.HasPrefix(upper, "FATAL[") || strings.HasPrefix(upper, "ERROR["):
		w.journal.Error("sing-box", "process_log", line, fields)
	case strings.HasPrefix(upper, "WARN[") || strings.HasPrefix(upper, "WARNING["):
		w.journal.Warn("sing-box", "process_log", line, fields)
	default:
		w.journal.Info("sing-box", "process_log", line, fields)
	}
}
