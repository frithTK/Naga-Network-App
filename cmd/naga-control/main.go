package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"naga.network/core/control"
	"naga.network/core/diagnostics"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/engine/olcrtc"
	"naga.network/core/engine/singbox"
	"naga.network/core/network"
	"naga.network/core/policy"
	"naga.network/core/storage"
	"naga.network/core/subscription"
	"naga.network/core/version"
	"naga.network/core/winhelper"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8765", "local control-plane address")
	token := flag.String("token", os.Getenv("NAGA_CONTROL_TOKEN"), "optional bearer token")
	tokenFile := flag.String("token-file", os.Getenv("NAGA_CONTROL_TOKEN_FILE"), "file containing the bearer token")
	production := flag.Bool("production", strings.EqualFold(os.Getenv("NAGA_CONTROL_PRODUCTION"), "true"), "require bearer authentication even on loopback")
	allowedOrigin := flag.String("allow-origin", os.Getenv("NAGA_CONTROL_ORIGIN"), "explicit browser origin allowed for CORS")
	singBoxPath := flag.String("sing-box", os.Getenv("NAGA_SINGBOX_PATH"), "sing-box binary path")
	olcHost := flag.String("olc-host", "", "elevated olcRTC TUN host pipe name")
	tunHost := flag.String("tun-host", "", "elevated TUN host pipe name")
	installHelper := flag.Bool("install-helper", false, "register elevated TUN helper (Windows)")
	uninstallHelper := flag.Bool("uninstall-helper", false, "remove elevated TUN helper")
	elevatedHelper := flag.Bool("elevated-helper", false, "run elevated TUN helper")
	flag.Parse()
	if *installHelper {
		if err := winhelper.Install(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *uninstallHelper {
		if err := winhelper.Uninstall(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *elevatedHelper {
		if err := winhelper.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if name := strings.TrimSpace(*olcHost); name != "" {
		if err := olcrtc.RunOlcHost(name); err != nil {
			log.Fatal(err)
		}
		return
	}
	if name := strings.TrimSpace(*tunHost); name != "" {
		if err := singbox.RunTunHost(name); err != nil {
			log.Fatal(err)
		}
		return
	}
	*token = strings.TrimSpace(*token)
	*tokenFile = strings.TrimSpace(*tokenFile)
	if *token == "" && *tokenFile != "" {
		loaded, err := readTokenFile(*tokenFile)
		if err != nil {
			log.Fatal(err)
		}
		*token = loaded
	}
	*allowedOrigin = strings.TrimSpace(*allowedOrigin)
	if *production && *token == "" {
		log.Fatal("a bearer token is required in production mode")
	}
	if !isLoopbackAddress(*listen) && strings.TrimSpace(*token) == "" {
		log.Fatal("a bearer token is required when control-plane is not loopback-only")
	}
	store, err := storage.NewDefault()
	if err != nil {
		log.Fatal(err)
	}
	journal, journalErr := diagnostics.New(filepath.Join(filepath.Dir(store.Root), "logs"))
	if journalErr != nil {
		log.Printf("persistent diagnostics unavailable: %v", journalErr)
	}
	if journal != nil {
		defer journal.Close()
		journal.Info("control-plane", "service_start", "Naga control-plane запускается", map[string]any{
			"production": *production,
		})
	}
	binaryPath, binaryErr := singbox.DiscoverControlRuntime(*singBoxPath)
	if binaryErr != nil {
		fmt.Println("sing-box binary not found; runtime start will remain unavailable")
	}
	adapter := singbox.Adapter{
		BinaryPath: binaryPath,
		Journal:    journal,
		Env: []string{
			"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true",
			"ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM=true",
			"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true",
		},
	}
	runtime := control.NewRuntimeController(
		&store,
		adapter,
		control.NewPlatformDNS(),
	)
	runtime.AWGFactory = awgengine.DefaultFactory()
	runtime.TUN = control.NewLinuxTUNPreflight()
	runtime.SystemProxy = control.NewSystemProxyManager()
	runtime.Journal = journal
	runtime.NetworkClass = func() policy.NetworkClass {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return network.LinuxClass(ctx)
	}
	subClient := subscription.Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  "NagaNetwork/0.1",
	}
	if runtime.RuleSets != nil {
		runtime.RuleSets.Fetcher = subClient
	}

	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()
	go network.Watcher{
		Class: func(ctx context.Context) policy.NetworkClass {
			return network.LinuxClass(ctx)
		},
		OnChange: func(_, to policy.NetworkClass) {
			if _, err := runtime.OnNetworkClass(to); err != nil && journal != nil {
				journal.Warn("runtime", "network_reselect_failed", err.Error(), map[string]any{
					"network_class": string(to),
				})
			}
		},
	}.Run(watchCtx)

	api := control.Server{
		Subscriptions: subClient,
		Storage:       &store,
		Runtime:       runtime,
		Validator:     adapter,
		Token:         *token,
		AllowedOrigin: *allowedOrigin,
		NetworkClass:  runtime.NetworkClass,
		RuntimeReady:  binaryErr == nil && binaryPath != "",
		OlcRTCReady:   olcrtc.BinaryReady(""),
		Version:       version.Version,
	}
	api.Journal = journal
	server := &http.Server{
		Addr:              *listen,
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	if *token == "" {
		fmt.Printf("naga-control listening on http://%s (local development authorization disabled)\n", *listen)
	} else {
		fmt.Printf("naga-control listening on http://%s\n", *listen)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	if journal != nil {
		journal.Info("control-plane", "service_stop", "Naga control-plane останавливается", nil)
	}
	_, _ = runtime.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func readTokenFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat control-plane token file: %w", err)
	}
	if tokenFileWorldReadable(info) {
		return "", fmt.Errorf("control-plane token file must not be group/world-readable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read control-plane token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("control-plane token file is empty")
	}
	return token, nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
