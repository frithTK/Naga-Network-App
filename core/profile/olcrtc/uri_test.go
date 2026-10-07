package olcrtc

import (
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseURIForms(t *testing.T) {
	plain := "olcrtc://jitsi?datachannel@https://meet.example/room#" + testKey + "$Naga CZ"
	got, err := ParseURI(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderJitsi || got.Transport != TransportDataChannel || got.Room != "https://meet.example/room" || got.Name != "Naga CZ" || got.Key != testKey || !got.RuntimeAllowed {
		t.Fatalf("plain = %+v", got)
	}
	if len(got.Params) != 0 {
		t.Fatalf("params = %+v", got.Params)
	}

	withParams := "olcrtc://jitsi?datachannel&mtu=1200@https://meet.example/room#" + strings.ToUpper(testKey) + "$"
	got, err = ParseURI(withParams)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != testKey || got.Name != "" || got.Params["mtu"] != "1200" {
		t.Fatalf("params form = %+v", got)
	}
}

func TestParseURIRejectsBroken(t *testing.T) {
	cases := []string{
		"https://example",
		"olcrtc://jitsi@https://meet.example/room#" + testKey,
		"olcrtc://jitsi?datachannel#" + testKey,
		"olcrtc://jitsi?datachannel@https://meet.example/room",
		"olcrtc://?datachannel@https://meet.example/room#" + testKey,
		"olcrtc://jitsi?datachannel@#" + testKey,
		"olcrtc://jitsi?datachannel@https://meet.example/room#" + testKey[:63],
		"olcrtc://other?datachannel@https://meet.example/room#" + testKey,
		"olcrtc://jitsi?datachannel@meet.example/room#" + testKey,
		"olcrtc://telemost?vp8channel@http://telemost.example/room#" + testKey,
	}
	for _, raw := range cases {
		if _, err := ParseURI(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestParseURIAcceptsTelemostHTTPSRoom(t *testing.T) {
	raw := "olcrtc://telemost?vp8channel@https://telemost.example/room#" + testKey + "$Telemost PL"
	got, err := ParseURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderTelemost || got.Room != "https://telemost.example/room" || got.Transport != TransportVP8Channel || !got.RuntimeAllowed {
		t.Fatalf("got = %+v", got)
	}
	idOnly := "olcrtc://telemost?vp8channel@room-1#" + testKey + "$id"
	got, err = ParseURI(idOnly)
	if err != nil || got.Room != "room-1" {
		t.Fatalf("id room = %+v err=%v", got, err)
	}
}

func TestParseURIDisabledTransport(t *testing.T) {
	raw := "olcrtc://jitsi?seichannel@https://meet.example/room#" + testKey + "$sei"
	got, err := ParseURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.RuntimeAllowed {
		t.Fatal("seichannel must not be runtime-allowed")
	}
	if got.Transport != TransportSEIChannel {
		t.Fatalf("transport = %s", got.Transport)
	}
}

func TestRedactStripsKeyAndRoom(t *testing.T) {
	raw := "olcrtc://jitsi?datachannel@https://meet.example/room#" + testKey + "$Naga"
	profile, err := ParseURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	err = Redact(errText(raw), profile)
	if strings.Contains(err.Error(), testKey) || strings.Contains(err.Error(), "meet.example") {
		t.Fatalf("redact leaked: %s", err)
	}
	if strings.Contains(RedactURI(raw), testKey) || strings.Contains(RedactURI(raw), "meet.example") {
		t.Fatalf("RedactURI leaked: %s", RedactURI(raw))
	}
}

type errText string

func (e errText) Error() string { return string(e) }
