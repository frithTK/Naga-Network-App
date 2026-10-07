package subscription

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetch(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		headers.Set("Subscription-Userinfo", "upload=0; download=123; total=456; expire=1796169599")
		headers.Set("profile-title", "base64:TmFnYSBOZXR3b3Jr")
		headers.Set("profile-update-interval", "1")
		headers.Set("isp-name", "NagaVPN")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader(`{"outbounds":[{"type":"direct","tag":"direct"}]}`)),
			Request:    req,
		}, nil
	})

	snapshot, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(context.Background(), "https://example.invalid/profile")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProfileTitle != "Naga Network" || snapshot.ProviderName != "NagaVPN" || snapshot.Subscription.Total != 456 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestFetchAcceptsPlainTextWithoutUserinfo(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Accept"); !strings.Contains(got, "text/plain") || !strings.Contains(got, "application/json") {
			t.Fatalf("Accept = %q", got)
		}
		headers := make(http.Header)
		headers.Set("Content-Type", "text/plain")
		headers.Set("profile-title", "AWG fixture")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader("[Interface]\nAddress = 10.8.0.2/32\n")),
			Request:    req,
		}, nil
	})

	snapshot, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(
		context.Background(),
		"https://example.invalid/amnezia.conf",
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProfileTitle != "AWG fixture" || snapshot.Subscription.Total != 0 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if !strings.Contains(string(snapshot.Config), "[Interface]") {
		t.Fatalf("config = %q", snapshot.Config)
	}
}

func TestFetchRejectsMalformedUserinfo(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Subscription-Userinfo", "not-a-userinfo")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader(`{"outbounds":[{"type":"direct"}]}`)),
			Request:    req,
		}, nil
	})
	if _, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(
		context.Background(),
		"https://example.invalid/profile",
	); err == nil {
		t.Fatal("expected malformed userinfo to be rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchBytes(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"version":1,"rules":[{"domain_suffix":[".example"]}]}`)),
			Request:    req,
		}, nil
	})
	body, err := (Client{HTTPClient: &http.Client{Transport: transport}}).FetchBytes(context.Background(), "https://example.invalid/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ".example") {
		t.Fatalf("body = %s", body)
	}
}

func TestFetchBytesRejectsHTTP(t *testing.T) {
	_, err := (Client{}).FetchBytes(context.Background(), "http://example.invalid/rules.json")
	if err == nil {
		t.Fatal("expected HTTP URL to be rejected")
	}
}

func TestFetchBytesRejectsForbiddenHostsWithoutFetching(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected fetch of %s", req.URL)
		return nil, nil
	})
	client := Client{HTTPClient: &http.Client{Transport: transport}}
	if _, err := client.FetchBytes(context.Background(), "https://127.0.0.1/rules.srs"); err == nil {
		t.Fatal("expected forbidden host to be rejected")
	}
}

func TestFetchRejectsHTTP(t *testing.T) {
	_, err := (Client{}).Fetch(context.Background(), "http://example.invalid/profile")
	if err == nil {
		t.Fatal("expected HTTP URL to be rejected")
	}
}

func TestFetchRejectsInvalidUpdateInterval(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
		headers.Set("profile-update-interval", "9223372036854775807")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader(`{"outbounds":[{"type":"direct"}]}`)),
			Request:    req,
		}, nil
	})

	_, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(
		context.Background(),
		"https://example.invalid/profile",
	)
	if err == nil {
		t.Fatal("expected invalid update interval to be rejected")
	}
}

func TestFetchRejectsHTTPSRedirectToHTTP(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"outbounds":[{"type":"direct"}]}`)
	}))
	t.Cleanup(httpServer.Close)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "example.com" {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{httpServer.URL}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		t.Fatalf("followed redirect to %s", req.URL)
		return nil, nil
	})
	_, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(context.Background(), "https://example.com/profile")
	if err == nil {
		t.Fatal("expected HTTP redirect to be rejected")
	}
	if strings.Contains(err.Error(), "outbounds") {
		t.Fatalf("error leaked profile body: %v", err)
	}
}

func TestForbiddenSubscriptionURL(t *testing.T) {
	t.Parallel()
	denied := []string{
		"https://127.0.0.1/",
		"https://192.168.0.1/x",
		"https://panel.example.com/a",
		"https://localhost/x",
		"https://localhost.localdomain/x",
		"https://[::1]/",
		"https://10.0.0.1/",
		"https://172.16.1.1/",
		"https://169.254.1.1/",
		"https://100.64.1.2/",
		"https://[fc00::1]/",
	}
	for _, raw := range denied {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if err := forbiddenSubscriptionURL(parsed); err == nil {
			t.Fatalf("allowed forbidden URL %s", raw)
		}
	}
	allowed := []string{
		"https://example.invalid/profile",
		"https://example.com/profile",
		"https://mypanel.example.com/a",
	}
	for _, raw := range allowed {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if err := forbiddenSubscriptionURL(parsed); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestFetchRejectsForbiddenHostsWithoutFetching(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected fetch of %s", req.URL)
		return nil, nil
	})
	client := Client{HTTPClient: &http.Client{Transport: transport}}
	for _, raw := range []string{
		"https://127.0.0.1/",
		"https://192.168.0.1/x",
		"https://panel.example.com/a",
		"https://localhost/x",
	} {
		_, err := client.Fetch(context.Background(), raw)
		if err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
		if strings.Contains(err.Error(), "outbounds") || strings.Contains(err.Error(), "{") {
			t.Fatalf("error leaked profile body for %s: %v", raw, err)
		}
	}
}

func TestFetchRejectsPrivateHTTPSRedirect(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "example.com" {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://10.0.0.1/secret-profile"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		t.Fatalf("followed redirect to %s", req.URL)
		return nil, nil
	})
	_, err := (Client{HTTPClient: &http.Client{Transport: transport}}).Fetch(context.Background(), "https://example.com/profile")
	if err == nil {
		t.Fatal("expected private HTTPS redirect to be rejected")
	}
	if strings.Contains(err.Error(), "outbounds") {
		t.Fatalf("error leaked profile body: %v", err)
	}
}

func FuzzForbiddenSubscriptionURL(f *testing.F) {
	for _, seed := range []string{
		"https://example.com/profile",
		"https://127.0.0.1/",
		"https://[::1]/",
		"https://panel.example.com/a",
		"https://192.168.0.1/x",
		"not a url",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		parsed, err := url.Parse(raw)
		if err != nil {
			return
		}
		_ = forbiddenSubscriptionURL(parsed)
	})
}
