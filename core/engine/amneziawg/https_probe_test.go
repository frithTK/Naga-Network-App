package amneziawg

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSProbeRequires204AndDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusNoContent)
		case "/error":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.Redirect(w, r, "/ok", http.StatusFound)
		}
	}))
	defer server.Close()
	transport := server.Client().Transport
	if delay, err := probeHTTPS204(t.Context(), server.URL+"/ok", transport); err != nil || delay <= 0 {
		t.Fatalf("204 delay = %d, %v", delay, err)
	}
	for _, path := range []string{"/error", "/redirect"} {
		if delay, err := probeHTTPS204(t.Context(), server.URL+path, transport); !errors.Is(err, ErrHTTPSStatus) || delay != 0 {
			t.Fatalf("%s delay = %d, %v", path, delay, err)
		}
	}
}
