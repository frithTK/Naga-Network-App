package subscription

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	socksproxy "golang.org/x/net/proxy"
	"naga.network/core/profile"
)

const (
	maxProfileSize = 16 << 20
	maxRuleSetSize = 8 << 20
)

type Client struct {
	HTTPClient *http.Client
	UserAgent  string
}

type Snapshot struct {
	Config         []byte
	Subscription   profile.SubscriptionInfo
	ProfileTitle   string
	ProviderName   string
	ProviderURL    string
	ProviderFAQ    string
	UpdateInterval time.Duration
	ContentType    string
}

func (c Client) Fetch(ctx context.Context, rawURL string) (Snapshot, error) {
	body, header, err := c.fetchHTTPS(ctx, rawURL, maxProfileSize, "subscription")
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		Config:         body,
		ProfileTitle:   "Naga Network",
		UpdateInterval: 24 * time.Hour,
		ContentType:    header.Get("Content-Type"),
	}
	if err := ApplySubscriptionHeaders(&snapshot, header); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// FetchBytes downloads an HTTPS body with the same uTLS transport and URL
// restrictions as subscription fetch. Used for remote sing-box rule-sets.
func (c Client) FetchBytes(ctx context.Context, rawURL string) ([]byte, error) {
	body, _, err := c.fetchHTTPS(ctx, rawURL, maxRuleSetSize, "rule-set")
	return body, err
}

func (c Client) fetchHTTPS(ctx context.Context, rawURL string, maxBytes int, kind string) ([]byte, http.Header, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, nil, errors.New("subscription URL must use HTTPS")
	}
	if err := forbiddenSubscriptionURL(parsed); err != nil {
		return nil, nil, err
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	clientCopy := *client
	if clientCopy.Transport == nil {
		clientCopy.Transport = defaultSubscriptionTransport
	}
	if clientCopy.Timeout == 0 {
		clientCopy.Timeout = 30 * time.Second
	}
	previousRedirect := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("subscription exceeded redirect limit")
		}
		if err := forbiddenSubscriptionURL(request.URL); err != nil {
			if request.URL != nil && request.URL.Scheme != "https" {
				return errors.New("subscription redirect left HTTPS")
			}
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		return nil
	}
	client = &clientCopy
	userAgent := c.UserAgent
	if userAgent == "" {
		userAgent = "NagaNetwork/0.1"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s request: %w", kind, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch %s: %w", kind, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("%s returned HTTP %d", kind, resp.StatusCode)
	}
	if resp.Request != nil {
		if err := forbiddenSubscriptionURL(resp.Request.URL); err != nil {
			if resp.Request.URL.Scheme != "https" {
				return nil, nil, errors.New("subscription redirect left HTTPS")
			}
			return nil, nil, err
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", kind, err)
	}
	if len(body) > maxBytes {
		return nil, nil, fmt.Errorf("%s exceeds %d bytes", kind, maxBytes)
	}
	header := http.Header{}
	if resp.Header != nil {
		header = resp.Header.Clone()
	}
	return body, header, nil
}

// FetchViaProxy loads a subscription through an already running local mixed
// inbound. TLS still terminates at the origin; only the TCP path is the tunnel.
func (c Client) FetchViaProxy(ctx context.Context, rawURL, proxy string) (Snapshot, error) {
	proxyURL, err := url.Parse(strings.TrimSpace(proxy))
	if err != nil || proxyURL.Host == "" {
		return Snapshot{}, errors.New("subscription proxy URL is invalid")
	}
	transport := &http.Transport{
		ForceAttemptHTTP2:     false,
		ResponseHeaderTimeout: 20 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    subscriptionRootCAs(),
		},
	}
	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5":
		dialer, err := socksproxy.SOCKS5("tcp", proxyURL.Host, nil, socksproxy.Direct)
		if err != nil {
			return Snapshot{}, err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return Snapshot{}, errors.New("subscription proxy URL is invalid")
	}
	proxied := c
	proxied.HTTPClient = &http.Client{Timeout: 30 * time.Second, Transport: transport}
	return proxied.Fetch(ctx, rawURL)
}

// ApplySubscriptionHeaders copies the subscription response headers onto a
// snapshot the UI already downloaded. An empty header keeps the current title
// and the 24-hour interval.
func ApplySubscriptionHeaders(snapshot *Snapshot, header http.Header) error {
	if snapshot == nil {
		return errors.New("subscription response is invalid")
	}
	if snapshot.UpdateInterval == 0 {
		snapshot.UpdateInterval = 24 * time.Hour
	}
	userinfo, err := profile.ParseSubscriptionUserinfo(header.Get("Subscription-Userinfo"))
	if err != nil {
		if strings.TrimSpace(header.Get("Subscription-Userinfo")) != "" {
			return err
		}
		userinfo = profile.SubscriptionInfo{}
	}
	snapshot.Subscription = userinfo
	title, err := profile.DecodeProfileTitle(header.Get("profile-title"))
	if err != nil {
		return err
	}
	if title != "" {
		snapshot.ProfileTitle = title
	}
	if raw := strings.TrimSpace(header.Get("profile-update-interval")); raw != "" {
		days, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || days <= 0 || days > 365 {
			return errors.New("profile-update-interval is invalid")
		}
		snapshot.UpdateInterval = time.Duration(days) * 24 * time.Hour
	}
	if name := strings.TrimSpace(header.Get("isp-name")); name != "" {
		snapshot.ProviderName = name
	}
	if raw := strings.TrimSpace(header.Get("isp-url")); raw != "" {
		snapshot.ProviderURL = raw
	}
	if raw := strings.TrimSpace(header.Get("isp-faq")); raw != "" {
		snapshot.ProviderFAQ = raw
	}
	return nil
}
