package subscription

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// The socket check runs after DNS resolution, for each address attempted by
// net.Dialer, immediately before connect. This also preserves Happy Eyeballs
// without a second DNS lookup between validation and connection.
func publicSubscriptionSocket(_ context.Context, _, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("subscription destination is invalid")
	}
	ip := net.ParseIP(host)
	if ip == nil || forbiddenSubscriptionIP(ip) {
		return errors.New("subscription destination is not allowed")
	}
	return nil
}

func subscriptionDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:        10 * time.Second,
		KeepAlive:      30 * time.Second,
		ControlContext: publicSubscriptionSocket,
	}
}

func preferIPv4Network(network string) []string {
	switch network {
	case "tcp":
		return []string{"tcp4", "tcp6"}
	default:
		return []string{network}
	}
}

func subscriptionDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := defaultSubscriptionDialer
	var firstErr error
	for _, netw := range preferIPv4Network(network) {
		conn, err := dialer.DialContext(ctx, netw, address)
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			return nil, err
		}
	}
	if firstErr == nil {
		return nil, errors.New("subscription destination is invalid")
	}
	return nil, firstErr
}

// Environment proxies would resolve the destination outside this process and
// bypass the address check. Fetches use OS routing (including an active VPN).
// An explicitly supplied transport is a trusted embedding/test dependency.
//
// Dual-stack "tcp" on Windows often waits on a broken AAAA lookup and then
// surfaces as a generic fetch timeout. Prefer IPv4, then fall back to IPv6.
//
// DialTLSContext uses a Chrome-shaped ClientHello. TLSClientConfig is ignored
// once DialTLSContext is set, and TLSHandshakeTimeout applies only to the
// stock dialer, so the handshake deadline lives in subscriptionDialTLSContext.
var defaultSubscriptionDialer = subscriptionDialer()

var defaultSubscriptionTransport = &http.Transport{
	DialContext:            subscriptionDialContext,
	DialTLSContext:         subscriptionDialTLSContext,
	ForceAttemptHTTP2:      false,
	TLSNextProto:           map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
	MaxIdleConns:           20,
	MaxIdleConnsPerHost:    2,
	IdleConnTimeout:        90 * time.Second,
	ResponseHeaderTimeout:  20 * time.Second,
	ExpectContinueTimeout:  time.Second,
	MaxResponseHeaderBytes: 1 << 20,
}
