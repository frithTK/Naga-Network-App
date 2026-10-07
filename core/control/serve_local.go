package control

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"naga.network/core/diagnostics"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/engine/olcrtc"
	"naga.network/core/engine/singbox"
	"naga.network/core/policy"
	"naga.network/core/storage"
	"naga.network/core/subscription"
	"naga.network/core/version"
)

type LocalOptions struct {
	Listen       string
	DataDir      string
	TokenPath    string
	NativeLibDir string
	SingBoxPath  string
	HevPath      string
	OlcPath      string
}

type LocalServer struct {
	Token   string
	Server  *http.Server
	runtime *RuntimeController
	stop    func()
}

func StartLocal(opts LocalOptions) (*LocalServer, error) {
	listen := strings.TrimSpace(opts.Listen)
	if listen == "" {
		listen = "127.0.0.1:8765"
	}
	if host, _, err := net.SplitHostPort(listen); err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("control-plane must listen on loopback")
	}
	if strings.TrimSpace(opts.DataDir) != "" {
		if err := os.Setenv("NAGA_DATA_DIR", opts.DataDir); err != nil {
			return nil, err
		}
	}
	androidTmp := ""
	if goruntime.GOOS == "android" {
		var err error
		androidTmp, err = androidTempDir(opts.DataDir)
		if err != nil {
			return nil, err
		}
		if err := os.Setenv("TMPDIR", androidTmp); err != nil {
			return nil, err
		}
	}
	if dir := strings.TrimSpace(opts.NativeLibDir); dir != "" {
		if err := os.Setenv("NAGA_NATIVE_LIB_DIR", dir); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(opts.OlcPath) != "" && strings.TrimSpace(opts.DataDir) != "" {
		versionPath := filepath.Join(opts.DataDir, "olcrtc.version")
		if err := os.WriteFile(versionPath, []byte(olcrtc.PinnedCommit+"\n"), 0o600); err == nil {
			_ = os.Setenv("NAGA_OLCRTC_VERSION_FILE", versionPath)
		}
	}
	store, err := storage.NewDefault()
	if err != nil {
		return nil, err
	}
	tokenPath := strings.TrimSpace(opts.TokenPath)
	if tokenPath == "" {
		tokenPath = filepath.Join(store.Root, "control.token")
	}
	token, err := ensureControlToken(tokenPath)
	if err != nil {
		return nil, err
	}
	journal, journalErr := diagnostics.New(filepath.Join(store.Root, "logs"))
	if journalErr != nil {
		journal = nil
	}
	binaryPath, _ := singbox.DiscoverControlRuntime(opts.SingBoxPath)
	adapter := singbox.Adapter{
		BinaryPath: binaryPath,
		Journal:    journal,
		Env: []string{
			"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true",
			"ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM=true",
			"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true",
		},
	}
	if androidTmp != "" {
		adapter.ConfigDir = filepath.Join(androidTmp, "singbox")
	}
	runtime := NewRuntimeController(&store, adapter, NewPlatformDNS())
	runtime.AWGFactory = awgengine.DefaultFactory()
	runtime.TUN = NewLinuxTUNPreflight()
	runtime.SystemProxy = NewSystemProxyManager()
	runtime.Journal = journal
	runtime.HevBinary = opts.HevPath
	runtime.OlcRTCBinary = opts.OlcPath
	runtime.NetworkClass = func() policy.NetworkClass { return policy.NetworkUnknown }
	subClient := subscription.Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  "NagaNetwork/android",
	}
	if runtime.RuleSets != nil {
		runtime.RuleSets.Fetcher = subClient
	}
	api := Server{
		Subscriptions: subClient,
		Storage:       &store,
		Runtime:       runtime,
		Validator:     adapter,
		Token:         token,
		NetworkClass:  runtime.NetworkClass,
		RuntimeReady:  binaryPath != "",
		OlcRTCReady:   olcrtc.BinaryReady(opts.OlcPath),
		Version:       version.Version,
	}
	api.Journal = journal
	server := &http.Server{
		Addr:              listen,
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ln, err := net.Listen("tcp4", listen)
	if err != nil {
		return nil, err
	}
	server.Addr = ln.Addr().String()
	go func() { _ = server.Serve(ln) }()
	return &LocalServer{
		Token:   token,
		Server:  server,
		runtime: runtime,
		stop: func() {
			_, _ = runtime.Stop()
			_ = server.Close()
			if journal != nil {
				_ = journal.Close()
			}
		},
	}, nil
}

func (s *LocalServer) Stop() {
	if s == nil || s.stop == nil {
		return
	}
	s.stop()
}

func (s *LocalServer) StopRuntime() {
	if s == nil || s.runtime == nil {
		return
	}
	_, _ = s.runtime.Stop()
}

func ensureControlToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

// androidTempDir is an app-writable replacement for /data/local/tmp.
// Untrusted apps cannot mkdir there, so sing-box check/start fail with
// permission denied unless TMPDIR points here.
func androidTempDir(dataDir string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", fmt.Errorf("android data directory is required")
	}
	tmp := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", fmt.Errorf("create android temp directory: %w", err)
	}
	return tmp, nil
}
