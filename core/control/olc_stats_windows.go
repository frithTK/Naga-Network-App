//go:build windows

package control

import (
	"net"
	"strings"

	"golang.org/x/sys/windows"
	engineolc "naga.network/core/engine/olcrtc"
)

func readPlatformIfaceBytes(name string) (tx, rx int64, err error) {
	iface, err := findTunnelInterface(name)
	if err != nil {
		return 0, 0, err
	}
	row := windows.MibIfRow{Index: uint32(iface.Index)}
	if err := windows.GetIfEntry(&row); err != nil {
		return 0, 0, err
	}
	return int64(row.OutOctets), int64(row.InOctets), nil
}

func findTunnelInterface(name string) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range ifaces {
		iface := &ifaces[i]
		got := strings.ToLower(iface.Name)
		if want != "" && (got == want || strings.Contains(got, want)) {
			return iface, nil
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			text := addr.String()
			if text == engineolc.TunnelIPv4 || strings.HasPrefix(text, engineolc.TunnelIPv4+"/") {
				return iface, nil
			}
		}
	}
	return nil, net.ErrClosed
}
