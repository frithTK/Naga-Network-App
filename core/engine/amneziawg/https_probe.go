package amneziawg

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

// HTTPSProber verifies the response from the target through this sidecar.
// It measures the AWG path independently of the system TUN and sing-box.
type HTTPSProber interface {
	ProbeHTTPS(ctx context.Context, target string) (int, error)
}

var (
	ErrHTTPSRequest = errors.New("AmneziaWG: HTTPS-проверка через туннель не удалась")
	ErrHTTPSStatus  = errors.New("AmneziaWG: контрольный сервер не вернул HTTP 204")
)

func (s *netstackSidecar) ProbeHTTPS(ctx context.Context, target string) (int, error) {
	return probeHTTPSViaSOCKS(ctx, s.ListenAddr(), target)
}

func probeHTTPSViaSOCKS(ctx context.Context, address, target string) (int, error) {
	if address == "" {
		return 0, ErrStopped
	}
	dialer, err := proxy.SOCKS5("tcp", address, nil, proxy.Direct)
	if err != nil {
		return 0, ErrHTTPSRequest
	}
	transport := &http.Transport{DialContext: dialer.(proxy.ContextDialer).DialContext}
	defer transport.CloseIdleConnections()
	return probeHTTPS204(ctx, target, transport)
}

func probeHTTPS204(ctx context.Context, target string, transport http.RoundTripper) (int, error) {
	client := http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil || request.URL.Scheme != "https" {
		return 0, ErrHTTPSRequest
	}
	start := time.Now()
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, context.Cause(ctx)
		}
		return 0, ErrHTTPSRequest
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return 0, ErrHTTPSStatus
	}
	return max(1, int(time.Since(start).Milliseconds())), nil
}
