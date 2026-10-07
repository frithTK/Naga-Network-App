package olcrtc

import (
	"net"
	"testing"
)

func TestRoomIPStringsSkipsIPv6AndLoopback(t *testing.T) {
	got := roomIPStrings([]net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("203.0.113.10"),
		net.ParseIP("2001:db8::1"),
		nil,
	})
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("got %v", got)
	}
	parsed := parseRoomIPs(got)
	if len(parsed) != 1 || !parsed[0].Equal(net.ParseIP("203.0.113.10")) {
		t.Fatalf("parsed %v", parsed)
	}
}
