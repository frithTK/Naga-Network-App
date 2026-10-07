package subscription

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
)

// Go's stock ClientHello is stalled by some filters that still allow
// browsers through to the same host. Chrome 120's cipher and extension
// list passes those filters. ALPN stays HTTP/1.1 so net/http can read the
// body; JA3 does not include the ALPN protocol list.
func subscriptionClientHello() (*utls.ClientHelloSpec, error) {
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
	if err != nil {
		return nil, err
	}
	for _, ext := range spec.Extensions {
		alpn, ok := ext.(*utls.ALPNExtension)
		if !ok {
			continue
		}
		alpn.AlpnProtocols = []string{"http/1.1"}
	}
	return &spec, nil
}

func subscriptionDialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	raw, err := subscriptionDialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		_ = raw.Close()
		return nil, errors.New("subscription destination is invalid")
	}
	spec, err := subscriptionClientHello()
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	tlsConn := utls.UClient(raw, &utls.Config{
		ServerName: host,
		RootCAs:    subscriptionRootCAs(),
		MinVersion: utls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	}, utls.HelloCustom)
	if err := tlsConn.ApplyPreset(spec); err != nil {
		_ = raw.Close()
		return nil, err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		_ = raw.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("tls handshake timeout: %w", err)
		}
		return nil, err
	}
	return tlsConn, nil
}
