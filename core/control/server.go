// Package control exposes the small local API used by the Flutter shell.
//
// List, preview and runtime responses return only safe metadata. Raw profile
// JSON never leaves the API. The subscription URL is returned only from
// GET /v1/profiles/{id}/share, for explicit copy/QR in the local UI.
package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	"naga.network/core/policy"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
	"naga.network/core/profile/olcrtc"
	"naga.network/core/storage"
	"naga.network/core/subscription"
	"naga.network/core/version"
)

type Server struct {
	Subscriptions subscription.Client
	Storage       *storage.Store
	Runtime       *RuntimeController
	Validator     engine.Adapter
	Token         string
	AllowedOrigin string
	Now           func() time.Time
	NetworkClass  func() policy.NetworkClass
	MutationMu    *sync.Mutex
	Journal       *diagnostics.Logger
	RuntimeReady  bool
	OlcRTCReady   bool
	Version       string
}

type healthResponse struct {
	Service      string `json:"service"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	RuntimeReady bool   `json:"runtime_ready"`
	OlcRTCReady  bool   `json:"olcrtc_ready"`
}

func (s Server) healthVersion() string {
	if trimmed := strings.TrimSpace(s.Version); trimmed != "" {
		return trimmed
	}
	return version.Version
}

var defaultMutationMu sync.Mutex

const maxImportBodyBytes = (16 << 20) + 64*1024

type previewRequest struct {
	URL                   string `json:"url"`
	Config                string `json:"config"`
	Name                  string `json:"name,omitempty"`
	ProfileTitle          string `json:"profile_title,omitempty"`
	SubscriptionUserinfo  string `json:"subscription_userinfo,omitempty"`
	ProfileUpdateInterval string `json:"profile_update_interval,omitempty"`
	ProviderName          string `json:"provider_name,omitempty"`
}

type runtimeStartRequest struct {
	ProfileID string `json:"profile_id"`
}

type runtimeSelectRequest struct {
	ProfileID string `json:"profile_id,omitempty"`
	Outbound  string `json:"outbound,omitempty"`
	Auto      bool   `json:"auto,omitempty"`
}

type modeRequest struct {
	Mode string `json:"mode"`
}

type Preview struct {
	ProfileTitle          string `json:"profile_title"`
	ProviderName          string `json:"provider_name,omitempty"`
	Engine                string `json:"engine"`
	UploadBytes           int64  `json:"upload_bytes"`
	DownloadBytes         int64  `json:"download_bytes"`
	TotalBytes            int64  `json:"total_bytes"`
	Unlimited             bool   `json:"unlimited"`
	ExpireUTC             string `json:"expire_utc,omitempty"`
	UpdateIntervalSeconds int64  `json:"update_interval_seconds"`
	CanConnect            bool   `json:"can_connect"`
	OlcRTCAvailable       bool   `json:"olcrtc_available"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type importResponse struct {
	ProfileID string  `json:"profile_id"`
	Preview   Preview `json:"preview"`
}

type shareResponse struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type ProfileSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ProviderName    string `json:"provider_name,omitempty"`
	Engine          string `json:"engine"`
	ImportedAt      string `json:"imported_at"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	UploadBytes     int64  `json:"upload_bytes"`
	DownloadBytes   int64  `json:"download_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Unlimited       bool   `json:"unlimited"`
	UpdateInterval  int64  `json:"update_interval_seconds"`
	ExpireUTC       string `json:"expire_utc,omitempty"`
	CanConnect      bool   `json:"can_connect"`
	HasSourceURL    bool   `json:"has_source_url"`
	OlcRTCAvailable bool   `json:"olcrtc_available,omitempty"`
}

type NodeSummary struct {
	Tag      string        `json:"tag"`
	Type     string        `json:"type"`
	Country  string        `json:"country,omitempty"`
	Protocol string        `json:"protocol,omitempty"`
	Selected bool          `json:"selected"`
	Children []NodeSummary `json:"children,omitempty"`
}

type ProfileNodes struct {
	ProfileID string        `json:"profile_id"`
	Mode      string        `json:"mode"`
	Nodes     []NodeSummary `json:"nodes"`
}

type UnifiedNodeSummary struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profile_id"`
	Tag         string `json:"tag"`
	RuntimeTag  string `json:"runtime_tag"`
	Country     string `json:"country,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	LatencyMS   int    `json:"latency_ms,omitempty"`
	ProbeStatus string `json:"probe_status,omitempty"`
}

type diagnosticsLogResponse struct {
	Entries []diagnostics.Entry `json:"entries"`
}

type responseStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseStatusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseStatusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Journal != nil && shouldLogMutation(r) {
		recorder := &responseStatusRecorder{ResponseWriter: w}
		w = recorder
		startedAt := time.Now()
		defer func() {
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			fields := map[string]any{
				"method":      r.Method,
				"route":       safeAPIRoute(r.URL.Path),
				"status":      status,
				"duration_ms": time.Since(startedAt).Milliseconds(),
			}
			if status >= http.StatusBadRequest {
				s.Journal.Warn("api", "mutation_failed", "Изменяющий API-запрос завершился ошибкой", fields)
				return
			}
			s.Journal.Info("api", "mutation_completed", "Изменяющий API-запрос выполнен", fields)
		}()
	}
	if s.AllowedOrigin != "" && r.Header.Get("Origin") == s.AllowedOrigin {
		w.Header().Set("Access-Control-Allow-Origin", s.AllowedOrigin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "DELETE, GET, POST, PUT, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.Token != "" && r.Header.Get("Authorization") != "Bearer "+s.Token {
		writeError(w, http.StatusUnauthorized, "control-plane authorization required")
		return
	}
	if r.Method != http.MethodGet &&
		!(r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/preview") &&
		!(r.Method == http.MethodPost && r.URL.Path == "/v1/nodes/probe") {
		unlock := s.lockMutation()
		defer unlock()
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
		writeJSON(w, http.StatusOK, healthResponse{
			Service:      "naga-control",
			Status:       "ok",
			Version:      s.healthVersion(),
			RuntimeReady: s.RuntimeReady,
			OlcRTCReady:  s.OlcRTCReady,
		})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/diagnostics/logs":
		s.diagnosticsLogs(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/diagnostics/client":
		s.recordClientEvent(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/preview":
		s.preview(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/profiles/import":
		s.importProfile(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/profiles":
		s.listProfiles(w)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/nodes":
		s.listUnifiedNodes(w)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/nodes/probe":
		s.probeUnifiedNodes(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/policy":
		s.getConnectionPolicy(w)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/policy":
		s.updateConnectionPolicy(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/routing":
		s.getRoutingPolicy(w)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/routing":
		s.updateRoutingPolicy(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/network":
		s.getNetworkClass(w)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/apps/discovered":
		s.listDiscoveredApps(w)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/apps":
		s.listApps(w)
	case r.Method == http.MethodPut:
		if appID, ok := appIDPath(r.URL.Path); ok {
			s.saveApp(w, r, appID)
			break
		}
		writeError(w, http.StatusNotFound, "endpoint not found")
	case r.Method == http.MethodDelete:
		if appID, ok := appIDPath(r.URL.Path); ok {
			s.deleteApp(w, appID)
			break
		}
		if profileID, ok := profileIDPath(r.URL.Path); ok {
			s.deleteProfile(w, profileID)
			break
		}
		writeError(w, http.StatusNotFound, "endpoint not found")
	case r.Method == http.MethodGet && r.URL.Path == "/v1/runtime":
		s.runtimeStatus(w)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/runtime/inspect":
		s.runtimeInspect(w)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/start":
		s.runtimeStart(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/select":
		s.runtimeSelect(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/stop":
		s.runtimeStop(w)
	default:
		if profileID, action, ok := profileAction(r.URL.Path); ok {
			switch {
			case r.Method == http.MethodGet && action == "nodes":
				s.listNodes(w, profileID)
			case r.Method == http.MethodGet && action == "share":
				s.shareProfile(w, profileID)
			case r.Method == http.MethodPost && action == "mode":
				s.selectMode(w, r, profileID)
			case r.Method == http.MethodPost && action == "refresh":
				s.refreshProfile(w, r, profileID)
			default:
				writeError(w, http.StatusNotFound, "endpoint not found")
			}
			return
		}
		writeError(w, http.StatusNotFound, "endpoint not found")
	}
}

func shouldLogMutation(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodOptions {
		return false
	}
	return !(r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/preview")
}

func safeAPIRoute(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "v1" && (parts[1] == "profiles" || parts[1] == "apps") {
		parts[2] = ":id"
	}
	if len(parts) == 0 || parts[0] == "" {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}

func (s Server) diagnosticsLogs(w http.ResponseWriter, r *http.Request) {
	if s.Journal == nil {
		writeJSON(w, http.StatusOK, diagnosticsLogResponse{Entries: []diagnostics.Entry{}})
		return
	}
	limit := 200
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(w, http.StatusBadRequest, "diagnostics limit must be between 1 and 1000")
			return
		}
		limit = parsed
	}
	entries, err := s.Journal.Recent(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "diagnostics log could not be read")
		return
	}
	writeJSON(w, http.StatusOK, diagnosticsLogResponse{Entries: entries})
}

type clientEventRequest struct {
	Level   string `json:"level"`
	Event   string `json:"event"`
	Message string `json:"message"`
}

// recordClientEvent lets the Flutter UI persist failures that happen before any
// other API call reaches the control-plane (subscription download timeouts,
// cancelled file dialogs), so the diagnostics log finally shows why an import
// never started.
func (s Server) recordClientEvent(w http.ResponseWriter, r *http.Request) {
	if s.Journal == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var request clientEventRequest
	if err := decodeBody(w, r, &request, 8192); err != nil {
		writeError(w, http.StatusBadRequest, "client event must be valid JSON")
		return
	}
	event := strings.TrimSpace(request.Event)
	if event == "" {
		event = "client_event"
	}
	message := strings.TrimSpace(request.Message)
	switch strings.ToLower(strings.TrimSpace(request.Level)) {
	case "error":
		s.Journal.Error("client", event, message, nil)
	case "warn", "warning":
		s.Journal.Warn("client", event, message, nil)
	default:
		s.Journal.Info("client", event, message, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) lockMutation() func() {
	mu := s.MutationMu
	if mu == nil {
		mu = &defaultMutationMu
	}
	mu.Lock()
	return mu.Unlock
}

func appIDPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] != "apps" || parts[2] == "" {
		return "", false
	}
	return parts[2], true
}

func (s Server) getConnectionPolicy(w http.ResponseWriter) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "policy storage is not configured")
		return
	}
	value, err := s.Storage.LoadConnectionPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "connection policy could not be loaded")
		return
	}
	if s.NetworkClass != nil {
		value.NetworkClass = s.NetworkClass()
	}
	writeJSON(w, http.StatusOK, value)
}

func (s Server) updateConnectionPolicy(w http.ResponseWriter, r *http.Request) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "policy storage is not configured")
		return
	}
	var value policy.ConnectionPolicy
	if err := decodeBody(w, r, &value, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "connection policy must be valid JSON")
		return
	}
	previous, err := s.Storage.LoadConnectionPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "connection policy could not be loaded")
		return
	}
	runtimeProfileID, runtimeWasRunning := s.runtimeForRollback()
	if err := s.Storage.PatchConnectionPolicy(func(current *policy.ConnectionPolicy) error {
		class := current.NetworkClass
		next := value
		next.NetworkClass = class
		*current = next
		return nil
	}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.Runtime != nil {
		s.Runtime.CacheConnectionMode(value.Mode)
	}
	if s.Runtime != nil && s.Runtime.IsRunning() {
		var err error
		if previous.TrafficMode != value.TrafficMode {
			_, err = s.Runtime.Reconnect()
		} else {
			_, err = s.Runtime.SelectByPolicy()
			if errors.Is(err, ErrRuntimeNotConnected) {
				err = nil
			}
		}
		if err != nil {
			rollbackErr := s.restoreAfterReconnectFailure(runtimeProfileID, runtimeWasRunning, func() error {
				return s.Storage.SaveConnectionPolicy(previous)
			})
			if rollbackErr != nil {
				writeError(w, http.StatusConflict, safeRuntimeError(errors.Join(err, rollbackErr)))
				return
			}
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	}
	saved, err := s.Storage.LoadConnectionPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "connection policy could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s Server) getRoutingPolicy(w http.ResponseWriter) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "routing storage is not configured")
		return
	}
	value, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing policy could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s Server) updateRoutingPolicy(w http.ResponseWriter, r *http.Request) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "routing storage is not configured")
		return
	}
	var value policy.RoutingPolicy
	if err := decodeBody(w, r, &value, 32768); err != nil {
		writeError(w, http.StatusBadRequest, "routing policy must be valid JSON")
		return
	}
	previous, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing policy could not be loaded")
		return
	}
	runtimeProfileID, runtimeWasRunning := s.runtimeForRollback()
	if err := s.Storage.SaveRoutingPolicy(value); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.Runtime != nil && s.Runtime.IsRunning() {
		if _, err := s.Runtime.Reconnect(); err != nil {
			rollbackErr := s.restoreAfterReconnectFailure(runtimeProfileID, runtimeWasRunning, func() error {
				return s.Storage.SaveRoutingPolicy(previous)
			})
			if rollbackErr != nil {
				writeError(w, http.StatusConflict, safeRuntimeError(errors.Join(err, rollbackErr)))
				return
			}
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	}
	saved, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing policy could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s Server) getNetworkClass(w http.ResponseWriter) {
	network := policy.NetworkUnknown
	if s.NetworkClass != nil {
		network = s.NetworkClass()
	}
	writeJSON(w, http.StatusOK, map[string]string{"network_class": string(network)})
}

func (s Server) listApps(w http.ResponseWriter) {
	if s.Storage == nil {
		writeJSON(w, http.StatusOK, map[string]any{"apps": []policy.AppRoute{}})
		return
	}
	value, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "applications could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": value.Apps})
}

func (s Server) saveApp(w http.ResponseWriter, r *http.Request, appID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "application storage is not configured")
		return
	}
	var app policy.AppRoute
	if err := decodeBody(w, r, &app, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "application route must be valid JSON")
		return
	}
	app.ID = appID
	routing, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "applications could not be loaded")
		return
	}
	previous := routing
	previous.Apps = append([]policy.AppRoute(nil), routing.Apps...)
	runtimeProfileID, runtimeWasRunning := s.runtimeForRollback()
	found := false
	for index := range routing.Apps {
		if routing.Apps[index].ID == appID {
			routing.Apps[index] = app
			found = true
			break
		}
	}
	if !found {
		routing.Apps = append(routing.Apps, app)
	}
	routing.FallbackIfNoEnabledApps()
	if err := s.Storage.SaveRoutingPolicy(routing); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.Runtime != nil && s.Runtime.IsRunning() {
		if _, err := s.Runtime.Reconnect(); err != nil {
			rollbackErr := s.restoreAfterReconnectFailure(runtimeProfileID, runtimeWasRunning, func() error {
				return s.Storage.SaveRoutingPolicy(previous)
			})
			if rollbackErr != nil {
				writeError(w, http.StatusConflict, safeRuntimeError(errors.Join(err, rollbackErr)))
				return
			}
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	}
	writeJSON(w, http.StatusOK, app)
}

func (s Server) deleteApp(w http.ResponseWriter, appID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "application storage is not configured")
		return
	}
	routing, err := s.Storage.LoadRoutingPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "applications could not be loaded")
		return
	}
	previous := routing
	previous.Apps = append([]policy.AppRoute(nil), routing.Apps...)
	runtimeProfileID, runtimeWasRunning := s.runtimeForRollback()
	filtered := make([]policy.AppRoute, 0, len(routing.Apps))
	for _, app := range routing.Apps {
		if app.ID != appID {
			filtered = append(filtered, app)
		}
	}
	routing.Apps = filtered
	routing.FallbackIfNoEnabledApps()
	if err := s.Storage.SaveRoutingPolicy(routing); err != nil {
		writeError(w, http.StatusInternalServerError, "application could not be deleted")
		return
	}
	if s.Runtime != nil && runtimeWasRunning {
		if _, err := s.Runtime.Reconnect(); err != nil {
			rollbackErr := s.restoreAfterReconnectFailure(runtimeProfileID, runtimeWasRunning, func() error {
				return s.Storage.SaveRoutingPolicy(previous)
			})
			if rollbackErr != nil {
				writeError(w, http.StatusConflict, safeRuntimeError(errors.Join(err, rollbackErr)))
				return
			}
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) runtimeForRollback() (string, bool) {
	if s.Runtime == nil || !s.Runtime.IsRunning() {
		return "", false
	}
	snapshot := s.Runtime.Status()
	return snapshot.ProfileID, snapshot.ProfileID != ""
}

func (s Server) restoreAfterReconnectFailure(profileID string, wasRunning bool, restore func() error) error {
	if err := restore(); err != nil {
		return fmt.Errorf("restore persisted state: %w", err)
	}
	if !wasRunning || s.Runtime == nil || profileID == "" {
		return nil
	}
	if _, err := s.Runtime.Start(profileID); err != nil {
		return fmt.Errorf("restore previous runtime: %w", err)
	}
	return nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func profileAction(path string) (profileID, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "profiles" || parts[2] == "" {
		return "", "", false
	}
	switch parts[3] {
	case "nodes", "mode", "refresh", "share":
		return parts[2], parts[3], true
	default:
		return "", "", false
	}
}

func profileIDPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] != "profiles" || parts[2] == "" {
		return "", false
	}
	return parts[2], true
}

func (s Server) shareProfile(w http.ResponseWriter, profileID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	value, err := s.Storage.Load(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile could not be loaded")
		return
	}
	url := strings.TrimSpace(value.SourceURL)
	if url == "" {
		url = olcrtcShareURI(value.Config)
	}
	if url == "" {
		writeError(w, http.StatusUnprocessableEntity, "у профиля нет ссылки для копирования")
		return
	}
	writeJSON(w, http.StatusOK, shareResponse{
		URL:  url,
		Name: value.Name,
	})
}

func (s Server) deleteProfile(w http.ResponseWriter, profileID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	if s.Runtime != nil && s.Runtime.IsActive(profileID) {
		if _, err := s.Runtime.Stop(); err != nil {
			writeError(w, http.StatusConflict, "active runtime could not be stopped")
			return
		}
	}
	if _, err := s.Storage.Load(profileID); err != nil {
		writeError(w, http.StatusNotFound, "profile could not be loaded")
		return
	}
	if err := s.Storage.Delete(profileID); err != nil {
		writeError(w, http.StatusInternalServerError, "profile could not be deleted")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) refreshProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	value, err := s.Storage.Load(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile could not be loaded")
		return
	}
	if strings.TrimSpace(value.SourceURL) == "" {
		writeError(w, http.StatusUnprocessableEntity, "у профиля нет ссылки подписки")
		return
	}
	request, ok := decodeOptionalPreview(w, r)
	if !ok {
		return
	}
	var snapshot subscription.Snapshot
	if strings.TrimSpace(request.Config) != "" {
		var err error
		request.URL = value.SourceURL
		snapshot, err = s.snapshotFromSupplied(request)
		if err != nil {
			writeImportSnapshotError(w, err)
			return
		}
		snapshot, err = s.validateSnapshot(r, snapshot)
		if err != nil {
			writeImportSnapshotError(w, err)
			return
		}
	} else {
		var err error
		snapshot, err = s.fetchSnapshot(r, value.SourceURL)
		if err != nil {
			s.logFetchError(err)
			writeError(w, http.StatusUnprocessableEntity, safeFetchError(err))
			return
		}
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	value.Name = snapshot.ProfileTitle
	value.ProviderName = snapshot.ProviderName
	value.Engine = snapshotEngine(snapshot)
	value.Config = snapshot.Config
	value.Subscription = snapshot.Subscription
	value.UpdateInterval = snapshot.UpdateInterval
	value.ImportedAt = now().UTC()
	if value.SelectedMode != "" && !profile.IsAmneziaWG(value) {
		available, inspectErr := profile.ContainsNode(profile.RuntimeSingBoxConfig(value.Config), value.SelectedMode)
		if inspectErr != nil || !available {
			if !envelopeAmneziaSelected(value.Config, value.SelectedMode) && !olcMatches(value.Config, value.SelectedMode) {
				value.SelectedMode = ""
			}
		}
	}
	if profile.IsAmneziaWG(value) && value.SelectedMode != "" && value.SelectedMode != awgcfg.LeafTag {
		value.SelectedMode = awgcfg.LeafTag
	}
	if value.SelectedMode == olcrtc.NodeTag && !olcrtcAvailable(value.Config) {
		value.SelectedMode = ""
	}
	if s.Runtime != nil {
		if _, err := s.Runtime.ReplaceProfile(value); err != nil {
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	} else if err := s.Storage.SaveVersioned(value); err != nil {
		writeError(w, http.StatusInternalServerError, "profile could not be updated")
		return
	}
	writeJSON(w, http.StatusOK, importResponse{
		ProfileID: value.ID,
		Preview:   s.previewFor(snapshot),
	})
}

func (s Server) listNodes(w http.ResponseWriter, profileID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	value, err := s.Storage.Load(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, s.profileNodes(value))
}

func (s Server) selectMode(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	var request modeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Mode) == "" {
		writeError(w, http.StatusBadRequest, "mode is required")
		return
	}
	value, err := s.Storage.Load(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile could not be loaded")
		return
	}
	mode := strings.TrimSpace(request.Mode)
	enteringOlc := olcMatches(value.Config, mode)
	onOlc := olcMatches(value.Config, value.SelectedMode) || value.Engine == profile.EngineOlcRTC
	if enteringOlc || onOlc {
		if enteringOlc && !olcrtcAvailable(value.Config) {
			writeError(w, http.StatusUnprocessableEntity, "selected node is not available")
			return
		}
		if s.Runtime != nil && s.Runtime.IsActive(profileID) {
			if _, err := s.Runtime.SwitchEngine(profileID, mode); err != nil {
				writeError(w, http.StatusConflict, safeRuntimeError(err))
				return
			}
		}
		value.SelectedMode = mode
		if err := s.Storage.Save(value); err != nil {
			writeError(w, http.StatusInternalServerError, "profile could not be updated")
			return
		}
		s.pinNodeSelection(value.Config, mode)
		writeJSON(w, http.StatusOK, s.profileNodes(value))
		return
	}
	if profile.IsAmneziaWG(value) || envelopeAmneziaSelected(value.Config, mode) {
		if mode != awgcfg.LeafTag && mode != awgcfg.SelectorTag && !envelopeAmneziaSelected(value.Config, mode) {
			writeError(w, http.StatusUnprocessableEntity, "selected node is not available")
			return
		}
		if mode == awgcfg.SelectorTag {
			mode = awgcfg.LeafTag
		}
	} else {
		available, err := profile.ContainsNode(profile.RuntimeSingBoxConfig(value.Config), mode)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "profile has no selectable nodes")
			return
		}
		if !available {
			writeError(w, http.StatusUnprocessableEntity, "selected node is not available")
			return
		}
	}
	if s.Runtime != nil && s.Runtime.IsActive(profileID) {
		if _, err := s.Runtime.Select(profileID, mode); err != nil {
			writeError(w, http.StatusConflict, safeRuntimeError(err))
			return
		}
	}
	value.SelectedMode = mode
	if err := s.Storage.Save(value); err != nil {
		writeError(w, http.StatusInternalServerError, "profile could not be updated")
		return
	}
	s.pinNodeSelection(value.Config, mode)
	writeJSON(w, http.StatusOK, s.profileNodes(value))
}

func (s Server) pinNodeSelection(raw []byte, mode string) {
	if s.Storage == nil || !concreteProfileSelection(mode) {
		return
	}
	country, protocol := selectionLabels(raw, mode)
	if s.Runtime != nil {
		s.Runtime.PinManualSelection(mode, country, protocol)
		return
	}
	_ = s.Storage.PatchConnectionPolicy(func(current *policy.ConnectionPolicy) error {
		current.Mode = policy.ConnectionManualStrict
		if country != "" {
			current.PreferredCountry = country
		}
		if protocol != "" {
			current.PreferredProtocol = protocol
		}
		return nil
	})
}

func (s Server) profileNodes(value profile.Profile) ProfileNodes {
	if value.Engine == profile.EngineOlcRTC {
		nodes := olcNodeSummaries(value.Config, value.SelectedMode)
		if len(nodes) == 0 {
			nodes = []NodeSummary{{
				Tag:      olcrtc.NodeTag,
				Type:     "olcrtc",
				Protocol: "olcRTC",
				Selected: true,
			}}
		}
		return ProfileNodes{ProfileID: value.ID, Nodes: nodes}
	}
	if profile.IsAmneziaWG(value) {
		selected := value.SelectedMode
		if selected == "" {
			selected = awgcfg.LeafTag
		}
		return ProfileNodes{
			ProfileID: value.ID,
			Mode:      awgcfg.SelectorTag,
			Nodes: []NodeSummary{{
				Tag:      awgcfg.LeafTag,
				Type:     "amneziawg",
				Protocol: profile.ProtocolFromType("amneziawg"),
				Selected: selected == awgcfg.LeafTag,
			}},
		}
	}
	options, err := profile.InspectMode(profile.RuntimeSingBoxConfig(value.Config))
	if err != nil {
		if len(profile.RuntimeAmneziaConf(value.Config)) == 0 {
			return ProfileNodes{ProfileID: value.ID, Nodes: []NodeSummary{}}
		}
		options = profile.ModeOptions{}
	}
	selected := value.SelectedMode
	if selected == "" {
		selected = options.Default
	}
	nodes := make([]NodeSummary, 0, len(options.Nodes)+1)
	for _, node := range options.Nodes {
		nodes = append(nodes, nodeSummary(node, selected))
	}
	nodes = append(nodes, olcNodeSummaries(value.Config, selected)...)
	if len(profile.RuntimeAmneziaConf(value.Config)) > 0 {
		if selected == "" {
			selected = awgcfg.LeafTag
		}
		nodes = append(nodes, NodeSummary{
			Tag:      awgcfg.LeafTag,
			Type:     "amneziawg",
			Protocol: profile.ProtocolFromType("amneziawg"),
			Selected: selected == awgcfg.LeafTag,
		})
	}
	return ProfileNodes{ProfileID: value.ID, Mode: options.Tag, Nodes: nodes}
}

func nodeSummary(node profile.Node, selected string) NodeSummary {
	children := make([]NodeSummary, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, nodeSummary(child, selected))
	}
	selectedHere := node.Tag == selected
	for _, child := range children {
		if child.Selected {
			selectedHere = true
			break
		}
	}
	return NodeSummary{
		Tag:      node.Tag,
		Type:     node.Type,
		Country:  node.Country,
		Protocol: node.Protocol,
		Selected: selectedHere,
		Children: children,
	}
}

func (s Server) preview(w http.ResponseWriter, r *http.Request) {
	request, ok := decodePreviewRequest(w, r)
	if !ok {
		return
	}
	snapshot, _, err := s.resolveSnapshot(r, request)
	if err != nil {
		s.logFetchError(err)
		writeImportSnapshotError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.previewFor(snapshot))
}

func (s Server) importProfile(w http.ResponseWriter, r *http.Request) {
	if s.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "profile storage is not configured")
		return
	}
	request, ok := decodePreviewRequest(w, r)
	if !ok {
		return
	}
	snapshot, sourceURL, err := s.resolveSnapshot(r, request)
	if err != nil {
		s.logFetchError(err)
		writeImportSnapshotError(w, err)
		return
	}
	if s.Runtime != nil && s.Runtime.IsRunning() {
		if _, err := s.Runtime.Stop(); err != nil {
			writeError(w, http.StatusConflict, "disconnect before importing a profile")
			return
		}
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	id := profile.IDFromConfig(snapshot.Config)
	if sourceURL != "" {
		id = profile.IDFromSourceURL(sourceURL)
	}
	value := profile.Profile{
		ID:             id,
		Name:           snapshot.ProfileTitle,
		ProviderName:   snapshot.ProviderName,
		SourceURL:      sourceURL,
		Engine:         snapshotEngine(snapshot),
		Config:         snapshot.Config,
		SelectedMode:   defaultOlcRTCMode(snapshot),
		Subscription:   snapshot.Subscription,
		UpdateInterval: snapshot.UpdateInterval,
		ImportedAt:     now().UTC(),
	}
	if err := s.Storage.Save(value); err != nil {
		writeError(w, http.StatusInternalServerError, "profile could not be saved")
		return
	}
	s.prefetchProviderRuleSets(value.Config)
	writeJSON(w, http.StatusOK, importResponse{
		ProfileID: value.ID,
		Preview:   s.previewFor(snapshot),
	})
}

func (s Server) prefetchProviderRuleSets(config []byte) {
	if s.Runtime == nil || s.Runtime.RuleSets == nil || len(config) == 0 {
		return
	}
	if s.Storage != nil {
		routing, err := s.Storage.LoadRoutingPolicy()
		if err != nil || !routing.ProviderRules {
			return
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _, _ = s.Runtime.RuleSets.Materialize(ctx, config)
	}()
}

func (s Server) listProfiles(w http.ResponseWriter) {
	if s.Storage == nil {
		writeJSON(w, http.StatusOK, []ProfileSummary{})
		return
	}
	values, err := s.Storage.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profiles could not be loaded")
		return
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	summaries := make([]ProfileSummary, 0, len(values))
	for _, value := range values {
		summary := ProfileSummary{
			ID:              value.ID,
			Name:            value.Name,
			ProviderName:    value.ProviderName,
			Engine:          string(value.Engine),
			ImportedAt:      value.ImportedAt.UTC().Format(time.RFC3339),
			UpdatedAt:       profileUpdatedAt(value),
			UploadBytes:     value.Subscription.Upload,
			DownloadBytes:   value.Subscription.Download,
			TotalBytes:      value.Subscription.Total,
			Unlimited:       value.Subscription.Unlimited(),
			UpdateInterval:  updateIntervalSeconds(value.UpdateInterval),
			CanConnect:      !value.Subscription.Expired(now()),
			HasSourceURL:    strings.TrimSpace(value.SourceURL) != "",
			OlcRTCAvailable: olcrtcAvailable(value.Config),
		}
		if !value.Subscription.Expire.IsZero() {
			summary.ExpireUTC = value.Subscription.Expire.UTC().Format(time.RFC3339)
		}
		summaries = append(summaries, summary)
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (s Server) listUnifiedNodes(w http.ResponseWriter) {
	if s.Storage == nil {
		writeJSON(w, http.StatusOK, []UnifiedNodeSummary{})
		return
	}
	values, err := s.Storage.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profiles could not be loaded")
		return
	}
	active := make([]profile.Profile, 0, len(values))
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	for _, value := range values {
		if value.Subscription.Expired(now()) {
			continue
		}
		active = append(active, value)
	}
	if len(active) == 0 {
		writeJSON(w, http.StatusOK, []UnifiedNodeSummary{})
		return
	}
	singBox := profile.SingBoxProfiles(active)
	awg := profile.AmneziaWGProfiles(active)
	if len(singBox) == 0 {
		candidates := amneziaUnifiedCandidates(awg, false)
		candidates = append(candidates, olcRTCUnifiedCandidates(active)...)
		s.writeUnifiedNodes(w, candidates)
		return
	}
	network := policy.NetworkUnknown
	if s.NetworkClass != nil {
		network = s.NetworkClass()
	}
	merged, err := profile.MergeProfilesForNetwork(singBox, string(network))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "profiles could not be combined")
		return
	}
	candidates := merged.Candidates
	if len(awg) > 0 {
		candidates = append(candidates, amneziaUnifiedCandidates(awg, true)...)
	}
	candidates = append(candidates, olcRTCUnifiedCandidates(active)...)
	s.writeUnifiedNodes(w, candidates)
}

func amneziaUnifiedCandidates(values []profile.Profile, namespaced bool) []policy.Candidate {
	candidates := make([]policy.Candidate, 0, len(values))
	for _, value := range values {
		if namespaced {
			candidates = append(candidates, amneziaUnifiedCandidate(value))
			continue
		}
		candidates = append(candidates, amneziaCandidate(value))
	}
	return candidates
}

func olcRTCUnifiedCandidates(values []profile.Profile) []policy.Candidate {
	candidates := make([]policy.Candidate, 0)
	for _, value := range values {
		candidates = append(candidates, olcNodeCandidates(value)...)
	}
	return candidates
}

func olcNodeCandidates(value profile.Profile) []policy.Candidate {
	profiles := olcProfiles(value.Config)
	tags := olcrtc.Tags(profiles)
	candidates := make([]policy.Candidate, 0, len(tags))
	for i, tag := range tags {
		if !profiles[i].RuntimeAllowed {
			continue
		}
		meta := profile.NodeMetadata(tag, "olcrtc")
		candidates = append(candidates, policy.Candidate{
			ID:         value.ID + "::" + tag,
			ProfileID:  value.ID,
			SourceTag:  tag,
			RuntimeTag: tag,
			Country:    meta.Country,
			Protocol:   "olcRTC",
			Type:       "olcrtc",
		})
	}
	return candidates
}

func olcNodeSummaries(raw []byte, selected string) []NodeSummary {
	profiles := olcProfiles(raw)
	tags := olcrtc.Tags(profiles)
	nodes := make([]NodeSummary, 0, len(tags))
	selected = strings.TrimSpace(selected)
	for i, tag := range tags {
		if !profiles[i].RuntimeAllowed {
			continue
		}
		meta := profile.NodeMetadata(tag, "olcrtc")
		nodes = append(nodes, NodeSummary{
			Tag:      tag,
			Type:     "olcrtc",
			Protocol: "olcRTC",
			Country:  meta.Country,
			Selected: selected == tag || (selected == olcrtc.NodeTag && len(tags) == 1),
		})
	}
	return nodes
}

func (s Server) writeUnifiedNodes(w http.ResponseWriter, candidates []policy.Candidate) {
	nodes := make([]UnifiedNodeSummary, 0, len(candidates))
	cache := map[string]ProbeCacheEntry{}
	if s.Runtime != nil {
		cache = s.Runtime.ProbeSnapshot()
	}
	for _, candidate := range candidates {
		node := UnifiedNodeSummary{
			ID:          candidate.ID,
			ProfileID:   candidate.ProfileID,
			Tag:         candidate.SourceTag,
			RuntimeTag:  candidate.RuntimeTag,
			Country:     candidate.Country,
			Protocol:    candidate.Protocol,
			ProbeStatus: string(ProbeUnknown),
		}
		if entry, ok := probeCacheForCandidate(cache, candidate); ok {
			node.ProbeStatus = string(entry.Status)
			node.LatencyMS = entry.LatencyMS
		}
		nodes = append(nodes, node)
	}
	writeJSON(w, http.StatusOK, nodes)
}

func profileUpdatedAt(value profile.Profile) string {
	moment := value.UpdatedAt
	if moment.IsZero() {
		moment = value.ImportedAt
	}
	if moment.IsZero() {
		return ""
	}
	return moment.UTC().Format(time.RFC3339)
}

func updateIntervalSeconds(interval time.Duration) int64 {
	if interval <= 0 {
		return int64((24 * time.Hour) / time.Second)
	}
	return int64(interval / time.Second)
}

func (s Server) runtimeStatus(w http.ResponseWriter) {
	if s.Runtime == nil {
		writeJSON(w, http.StatusOK, RuntimeSnapshot{Status: "stopped"})
		return
	}
	writeJSON(w, http.StatusOK, s.Runtime.Status())
}

func (s Server) runtimeInspect(w http.ResponseWriter) {
	if s.Runtime == nil {
		writeJSON(w, http.StatusOK, RuntimeInspect{Status: "stopped"})
		return
	}
	writeJSON(w, http.StatusOK, s.Runtime.Inspect())
}

func (s Server) runtimeStart(w http.ResponseWriter, r *http.Request) {
	if s.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime is not configured")
		return
	}
	var request runtimeStartRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "request must be valid JSON")
		return
	}
	snapshot, err := s.Runtime.Start(strings.TrimSpace(request.ProfileID))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, safeRuntimeError(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s Server) runtimeStop(w http.ResponseWriter) {
	if s.Runtime == nil {
		writeJSON(w, http.StatusOK, RuntimeSnapshot{Status: "stopped"})
		return
	}
	snapshot, err := s.Runtime.Stop()
	if err != nil {
		writeError(w, http.StatusInternalServerError, safeRuntimeError(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s Server) runtimeSelect(w http.ResponseWriter, r *http.Request) {
	if s.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime is not configured")
		return
	}
	var request runtimeSelectRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "request must be valid JSON")
		return
	}
	outbound := strings.TrimSpace(request.Outbound)
	if request.Auto || strings.EqualFold(outbound, "auto") {
		snapshot, err := s.Runtime.SelectByPolicy()
		if err != nil {
			status := http.StatusConflict
			if errors.Is(err, ErrRuntimeNotConnected) {
				status = http.StatusConflict
			}
			writeError(w, status, safeRuntimeError(err))
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	if outbound == "" {
		writeError(w, http.StatusBadRequest, "outbound is required")
		return
	}
	snapshot, err := s.Runtime.Select(
		strings.TrimSpace(request.ProfileID),
		outbound,
	)
	if err != nil {
		writeError(w, http.StatusConflict, safeRuntimeError(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s Server) probeUnifiedNodes(w http.ResponseWriter, r *http.Request) {
	if s.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime is not configured")
		return
	}
	if err := s.Runtime.ProbeNodes(r.Context()); err != nil {
		switch {
		case errors.Is(err, ErrNoProbeProfiles), errors.Is(err, ErrProbeInProgress):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrRuntimeNotConnected):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusUnprocessableEntity, safeRuntimeError(err))
		}
		return
	}
	s.listUnifiedNodes(w)
}

func (s Server) listDiscoveredApps(w http.ResponseWriter) {
	apps := DiscoverUserProcesses()
	if apps == nil {
		apps = []DiscoveredApp{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (s Server) fetchSnapshot(r *http.Request, rawURL string) (subscription.Snapshot, error) {
	if s.Runtime != nil {
		if proxy := s.Runtime.ActiveFetchProxy(); proxy != "" {
			snapshot, err := s.Subscriptions.FetchViaProxy(r.Context(), rawURL, proxy)
			if err == nil {
				return s.validateSnapshot(r, snapshot)
			}
			s.logFetchError(err)
		}
	}
	snapshot, err := s.Subscriptions.Fetch(r.Context(), rawURL)
	if err != nil {
		return subscription.Snapshot{}, err
	}
	return s.validateSnapshot(r, snapshot)
}

func decodePreviewRequest(w http.ResponseWriter, r *http.Request) (previewRequest, bool) {
	var request previewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxImportBodyBytes))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "request must be valid JSON")
		return previewRequest{}, false
	}
	return request, true
}

func (s Server) resolveSnapshot(r *http.Request, request previewRequest) (subscription.Snapshot, string, error) {
	sourceURL := ""
	if strings.TrimSpace(request.URL) != "" {
		resolved, err := subscription.ResolveFetchURL(request.URL)
		if err != nil {
			return subscription.Snapshot{}, "", err
		}
		sourceURL = resolved
	}
	if strings.TrimSpace(request.Config) != "" {
		snapshot, err := s.snapshotFromSupplied(request)
		if err != nil {
			return subscription.Snapshot{}, "", err
		}
		snapshot, err = s.validateSnapshot(r, snapshot)
		return snapshot, sourceURL, err
	}
	if sourceURL == "" {
		return subscription.Snapshot{}, "", errors.New("url or config is required")
	}
	snapshot, err := s.fetchSnapshot(r, sourceURL)
	return snapshot, sourceURL, err
}

func decodeOptionalPreview(w http.ResponseWriter, r *http.Request) (previewRequest, bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return previewRequest{}, true
	}
	var request previewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxImportBodyBytes))
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return previewRequest{}, true
		}
		writeError(w, http.StatusBadRequest, "request must be valid JSON")
		return previewRequest{}, false
	}
	return request, true
}

func (s Server) snapshotFromSupplied(request previewRequest) (subscription.Snapshot, error) {
	snapshot := snapshotFromConfig([]byte(request.Config), request.Name)
	if strings.TrimSpace(request.URL) == "" &&
		request.ProfileTitle == "" &&
		request.SubscriptionUserinfo == "" &&
		request.ProfileUpdateInterval == "" &&
		request.ProviderName == "" {
		return snapshot, nil
	}
	header := make(http.Header)
	if strings.TrimSpace(request.ProfileTitle) != "" {
		header.Set("profile-title", request.ProfileTitle)
	}
	if strings.TrimSpace(request.SubscriptionUserinfo) != "" {
		header.Set("Subscription-Userinfo", request.SubscriptionUserinfo)
	}
	if strings.TrimSpace(request.ProfileUpdateInterval) != "" {
		header.Set("profile-update-interval", request.ProfileUpdateInterval)
	}
	if strings.TrimSpace(request.ProviderName) != "" {
		header.Set("isp-name", request.ProviderName)
	}
	if err := subscription.ApplySubscriptionHeaders(&snapshot, header); err != nil {
		return subscription.Snapshot{}, err
	}
	return snapshot, nil
}

func writeImportSnapshotError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "url or config is required"),
		strings.Contains(message, "subscription URL"),
		strings.Contains(message, "unsupported nagavpn"),
		strings.Contains(message, "install-config"):
		writeError(w, http.StatusBadRequest, message)
	default:
		writeError(w, http.StatusUnprocessableEntity, safeFetchError(err))
	}
}

func snapshotFromConfig(raw []byte, name string) subscription.Snapshot {
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	return subscription.Snapshot{
		Config:       raw,
		ProfileTitle: profileTitleFromFileName(name),
	}
}

func profileTitleFromFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if ext := path.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "Naga Network"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	return name
}

func (s Server) validateSnapshot(r *http.Request, snapshot subscription.Snapshot) (subscription.Snapshot, error) {
	if olcrtc.IsDocument(snapshot.Config) {
		if _, err := olcrtc.ParseDocument(snapshot.Config); err != nil {
			return subscription.Snapshot{}, err
		}
		return snapshot, nil
	}
	if env, ok, envErr := profile.ParseEnvelope(snapshot.Config); ok {
		if envErr != nil {
			return subscription.Snapshot{}, envErr
		}
		if err := profile.ValidateSingBoxConfig(env.SingBox); err != nil {
			return subscription.Snapshot{}, errors.New("profile is not a valid sing-box config")
		}
		if s.Validator != nil {
			if err := s.Validator.Validate(r.Context(), env.SingBox); err != nil {
				s.logValidationError(err)
				return subscription.Snapshot{}, errors.New("profile failed engine validation")
			}
		}
		if len(env.AmneziaConf) > 0 {
			if _, err := awgcfg.Parse(env.AmneziaConf); err != nil {
				return subscription.Snapshot{}, err
			}
		}
		if fieldErr := profile.OlcRTCError(snapshot.Config); fieldErr != nil && s.Journal != nil {
			s.Journal.Warn("subscription", "olcrtc_ignored", fieldErr.Error(), nil)
		}
		return snapshot, nil
	}
	format, classifyErr := awgcfg.Classify(snapshot.Config)
	if classifyErr != nil && format != awgcfg.FormatJSON {
		if classifyErr == awgcfg.ErrPlainWireGuard {
			return subscription.Snapshot{}, classifyErr
		}
		if classifyErr != awgcfg.ErrUnknownFormat && classifyErr != awgcfg.ErrEmptyConfig {
			return subscription.Snapshot{}, classifyErr
		}
	}
	if format == awgcfg.FormatAmneziaWG {
		if _, err := awgcfg.Parse(snapshot.Config); err != nil {
			return subscription.Snapshot{}, err
		}
		return snapshot, nil
	}
	if err := profile.ValidateSingBoxConfig(snapshot.Config); err != nil {
		return subscription.Snapshot{}, errors.New("profile is not a valid sing-box config")
	}
	if s.Validator != nil {
		if err := s.Validator.Validate(r.Context(), snapshot.Config); err != nil {
			s.logValidationError(err)
			return subscription.Snapshot{}, errors.New("profile failed engine validation")
		}
	}
	return snapshot, nil
}

func envelopeAmneziaSelected(config []byte, mode string) bool {
	if len(profile.RuntimeAmneziaConf(config)) == 0 {
		return false
	}
	return mode == awgcfg.LeafTag || mode == awgcfg.SelectorTag
}

func snapshotEngine(snapshot subscription.Snapshot) profile.Engine {
	if olcrtc.IsDocument(snapshot.Config) || olcrtc.IsURI(snapshot.Config) {
		return profile.EngineOlcRTC
	}
	format, err := awgcfg.Classify(snapshot.Config)
	if err == nil && format == awgcfg.FormatAmneziaWG {
		return profile.EngineAmneziaWG
	}
	return profile.EngineSingBox
}

func olcrtcShareURI(raw []byte) string {
	if cfg, err := olcrtc.ParseDocument(raw); err == nil && cfg != nil && cfg.ShareURI() != "" {
		return cfg.ShareURI()
	}
	if olcrtc.IsURI(raw) {
		return string(bytes.TrimSpace(raw))
	}
	if cfg := profile.RuntimeOlcRTC(raw); cfg != nil {
		return cfg.ShareURI()
	}
	return ""
}

func olcrtcAvailable(raw []byte) bool {
	if cfg, err := olcrtc.ParseDocument(raw); err == nil && cfg != nil {
		return true
	}
	if olcrtc.IsURI(raw) {
		profile, err := olcrtc.ParseURI(string(bytes.TrimSpace(raw)))
		return err == nil && profile.RuntimeAllowed
	}
	return profile.RuntimeOlcRTC(raw) != nil
}

func defaultOlcRTCMode(snapshot subscription.Snapshot) string {
	if snapshotEngine(snapshot) == profile.EngineOlcRTC {
		return olcrtc.NodeTag
	}
	return ""
}

func (s Server) previewFor(snapshot subscription.Snapshot) Preview {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	preview := Preview{
		ProfileTitle:          snapshot.ProfileTitle,
		ProviderName:          snapshot.ProviderName,
		Engine:                string(snapshotEngine(snapshot)),
		UploadBytes:           snapshot.Subscription.Upload,
		DownloadBytes:         snapshot.Subscription.Download,
		TotalBytes:            snapshot.Subscription.Total,
		Unlimited:             snapshot.Subscription.Unlimited(),
		UpdateIntervalSeconds: updateIntervalSeconds(snapshot.UpdateInterval),
		CanConnect:            !snapshot.Subscription.Expired(now()),
		OlcRTCAvailable:       olcrtcAvailable(snapshot.Config),
	}
	if !snapshot.Subscription.Expire.IsZero() {
		preview.ExpireUTC = snapshot.Subscription.Expire.UTC().Format(time.RFC3339)
	}
	return preview
}

func (s Server) logValidationError(err error) {
	if s.Journal == nil || err == nil {
		return
	}
	// The user-facing message stays generic, but the diagnostics log keeps the
	// redacted engine reason so a file import that fails the sing-box "check"
	// pass can finally be explained instead of guessed at.
	s.Journal.Error("subscription", "validation_failed", awgcfg.SafeError(err), nil)
}

func (s Server) logFetchError(err error) {
	if s.Journal == nil || err == nil {
		return
	}
	message := awgcfg.SafeError(err)
	if !strings.Contains(message, "fetch subscription") {
		return
	}
	safe := safeFetchError(err)
	s.Journal.Warn("subscription", "fetch_failed", safe, map[string]any{
		"class": fetchErrorClass(safe),
	})
}

func fetchErrorClass(safe string) string {
	switch safe {
	case "subscription tls certificate could not be verified":
		return "tls"
	case "subscription dns lookup failed":
		return "dns"
	case "subscription endpoint timed out":
		return "timeout"
	case "subscription endpoint blocked":
		return "blocked"
	case "subscription URL host is not allowed":
		return "host"
	default:
		return "unavailable"
	}
}

func safeFetchError(err error) string {
	// Keep protocol diagnostics useful, but never echo the URL or transport data.
	message := awgcfg.SafeError(err)
	switch {
	case strings.Contains(message, "host is not allowed"):
		return "subscription URL host is not allowed"
	case strings.Contains(message, "subscription URL"):
		return "subscription URL must use HTTPS"
	case strings.Contains(message, "subscription returned HTTP"):
		return message
	case strings.Contains(message, "profile-title") ||
		strings.Contains(message, "profile-update-interval") ||
		strings.Contains(message, "subscription-userinfo"):
		return message
	case strings.Contains(message, "fetch subscription"):
		switch {
		case strings.Contains(message, "x509") || strings.Contains(message, "certificate") || strings.Contains(message, "tls:"):
			return "subscription tls certificate could not be verified"
		case strings.Contains(message, "no such host") || strings.Contains(message, "server misbehaving"):
			return "subscription dns lookup failed"
		case fetchLooksLikeTimeout(message):
			return "subscription endpoint timed out"
		case strings.Contains(message, "destination is not allowed") || strings.Contains(message, "destination is invalid"):
			return "subscription URL host is not allowed"
		case fetchLooksLikeBlocked(message):
			return "subscription endpoint blocked"
		default:
			return "subscription endpoint unavailable"
		}
	case strings.Contains(message, "read subscription"):
		return "subscription response could not be read"
	case strings.Contains(message, "amneziawg") ||
		strings.Contains(message, "wireguard profile") ||
		strings.Contains(message, "profile is not") ||
		strings.Contains(message, "profile failed engine") ||
		strings.Contains(message, "profile is empty"):
		return message
	default:
		return "subscription response is invalid"
	}
}

func fetchLooksLikeTimeout(message string) bool {
	return strings.Contains(message, "timeout") ||
		strings.Contains(message, "Timeout") ||
		strings.Contains(message, "i/o deadline") ||
		strings.Contains(message, "period of time") ||
		strings.Contains(message, "failed to respond")
}

func fetchLooksLikeBlocked(message string) bool {
	return strings.Contains(message, "connectex") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "forcibly closed") ||
		strings.Contains(message, "connection was aborted") ||
		strings.Contains(message, "network is unreachable") ||
		strings.Contains(message, "host is unreachable") ||
		strings.Contains(message, "no route to host") ||
		strings.Contains(message, "wsasend") ||
		strings.Contains(message, "wsarecv") ||
		strings.Contains(message, "EOF")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
