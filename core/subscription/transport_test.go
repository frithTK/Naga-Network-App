package subscription

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/dns/dnsmessage"
)

func TestEmbeddedISRGRootsParse(t *testing.T) {
	for name, pem := range map[string][]byte{"x1": isrgRootX1PEM, "x2": isrgRootX2PEM} {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatalf("parse ISRG %s", name)
		}
	}
	if defaultSubscriptionTransport.ForceAttemptHTTP2 {
		t.Fatal("subscription HTTP/2 should be disabled")
	}
	if defaultSubscriptionTransport.DialTLSContext == nil {
		t.Fatal("subscription fetch should use the browser TLS dialer")
	}
	if subscriptionRootCAs() == nil {
		t.Fatal("missing extra TLS roots")
	}
}

func TestSubscriptionClientHelloMatchesChromeHTTP1(t *testing.T) {
	spec, err := subscriptionClientHello()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.CipherSuites) < 10 {
		t.Fatalf("cipher suites = %d", len(spec.CipherSuites))
	}
	var alpn []string
	for _, ext := range spec.Extensions {
		if typed, ok := ext.(*utls.ALPNExtension); ok {
			alpn = typed.AlpnProtocols
		}
	}
	if len(alpn) != 1 || alpn[0] != "http/1.1" {
		t.Fatalf("ALPN = %#v", alpn)
	}
}

func TestPreferIPv4NetworkTriesIPv4First(t *testing.T) {
	got := preferIPv4Network("tcp")
	if len(got) != 2 || got[0] != "tcp4" || got[1] != "tcp6" {
		t.Fatalf("prefer IPv4 = %#v", got)
	}
	if got := preferIPv4Network("tcp4"); len(got) != 1 || got[0] != "tcp4" {
		t.Fatalf("tcp4 = %#v", got)
	}
}

func TestSubscriptionSocketRejectsNonPublicDestinations(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:443", "10.0.0.1:443", "192.168.1.1:443", "169.254.169.254:443",
		"100.64.0.1:443", "0.1.2.3:443", "224.0.0.1:443", "255.255.255.255:443",
		"[::1]:443", "[fc00::1]:443", "[fe80::1]:443", "[ff02::1]:443",
		"[::ffff:127.0.0.1]:443", "unresolved.example:443", "bad-address",
	} {
		t.Run(address, func(t *testing.T) {
			if err := publicSubscriptionSocket(context.Background(), "tcp", address, nil); err == nil {
				t.Fatal("allowed non-public destination")
			}
		})
	}
	for _, address := range []string{"1.1.1.1:443", "[2606:4700:4700::1111]:443"} {
		if err := publicSubscriptionSocket(context.Background(), "tcp", address, nil); err != nil {
			t.Fatal(err)
		}
	}
}

// A controlled DNS server returns loopback for a normal-looking hostname.
// The real net.Dialer must reject it before sending traffic to the listener.
func TestSubscriptionDialerRejectsPrivateDNSAnswerBeforeConnect(t *testing.T) {
	dns, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dns.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, peer, err := dns.ReadFrom(buf)
			if err != nil {
				return
			}
			var query dnsmessage.Message
			if query.Unpack(buf[:n]) != nil {
				continue
			}
			response := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			for _, question := range query.Questions {
				if question.Type == dnsmessage.TypeA {
					response.Answers = append(response.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
						Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
					})
				}
			}
			packet, err := response.Pack()
			if err == nil {
				_, _ = dns.WriteTo(packet, peer)
			}
		}
	}()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			accepted.Store(true)
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	dialer := subscriptionDialer()
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", dns.LocalAddr().String())
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("subscription.example", port))
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "destination is not allowed") {
		t.Fatalf("expected socket policy error, got %v", err)
	}
	_ = listener.Close()
	<-done
	if accepted.Load() {
		t.Fatal("contacted private destination before rejecting it")
	}
}

func TestSubscriptionRedirectLoopIsBounded(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {fmt.Sprintf("https://example.invalid/%d", calls)}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})
	_, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(context.Background(), "https://example.invalid/start")
	if err == nil || !strings.Contains(err.Error(), "redirect limit") || calls != 10 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
