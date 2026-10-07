package olcrtc

import (
	"context"
	"errors"
	"net"
)

var errOlcHostUnsupported = errors.New("elevated olcRTC TUN host is only supported on Windows")

// ErrUACCancelled is returned when the user dismisses the elevation prompt.
var ErrUACCancelled = errors.New("запуск отменён, права администратора не выданы")

// TunnelHost is the elevated Windows process that owns hev and routes.
type TunnelHost interface {
	Stop() error
}

type olcHostRequest struct {
	HevPath    string   `json:"hev_path"`
	ConfigPath string   `json:"config_path"`
	RoomIPs    []string `json:"room_ips"`
}

type olcHostReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func roomIPStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	return out
}

func parseRoomIPs(values []string) []net.IP {
	out := make([]net.IP, 0, len(values))
	for _, value := range values {
		ip := net.ParseIP(value)
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			out = append(out, ip4)
		}
	}
	return out
}

func startElevatedUnsupported(context.Context, string, string, []net.IP) (TunnelHost, error) {
	return nil, errOlcHostUnsupported
}
