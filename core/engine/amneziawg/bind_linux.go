//go:build linux && !android

package amneziawg

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"golang.org/x/sys/unix"

	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
)

// linuxUplinkBind pins AWG's own sockets. It doesn't change the routing of
// other applications or leave routes to remove if the process exits.
type linuxUplinkBind struct {
	conn.Bind
	mu        sync.Mutex
	endpoints []netip.Addr
	v4, v6    string
}

func newPlatformBind(ctx context.Context, cfg awgcfg.Config) (conn.Bind, error) {
	bind := &linuxUplinkBind{Bind: conn.NewDefaultBind()}
	for _, peer := range cfg.Peers {
		endpoint, err := netip.ParseAddrPort(peer.Endpoint)
		if err != nil {
			return nil, awgcfg.ErrPeerEndpoint
		}
		bind.endpoints = append(bind.endpoints, endpoint.Addr())
	}
	var err error
	bind.v4, bind.v6, err = linuxUplinks(ctx, bind.endpoints)
	if err != nil {
		return nil, err
	}
	return bind, nil
}

func linuxUplinks(ctx context.Context, endpoints []netip.Addr) (v4, v6 string, err error) {
	for _, addr := range endpoints {
		family, name := "-6", &v6
		if addr.Is4() {
			family, name = "-4", &v4
		}
		iface, err := linuxUplink(ctx, family, addr)
		if err != nil {
			return "", "", err
		}
		if *name != "" && *name != iface {
			return "", "", ErrUplinkUnavailable
		}
		*name = iface
	}
	return v4, v6, nil
}

func platformEndpointResolver() endpointResolver {
	if _, err := net.InterfaceByName(profile.RuntimeTUNName); err != nil {
		return net.DefaultResolver
	}
	// While Naga's TUN is active, the host's default resolver may itself
	// depend on the current AWG node. Use DNS configured on the physical link.
	return &net.Resolver{PreferGo: true, Dial: dialLinuxBootstrapDNS}
}

func dialLinuxBootstrapDNS(ctx context.Context, network, _ string) (net.Conn, error) {
	for _, family := range []string{"-4", "-6"} {
		routes, err := readLinuxRoutes(ctx, family, "show", "table", "main", "default")
		if err != nil {
			continue
		}
		for _, route := range routes {
			iface, err := net.InterfaceByName(route.Device)
			if err != nil || route.Device == profile.RuntimeTUNName ||
				iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || iface.Flags&net.FlagUp == 0 {
				continue
			}
			raw, err := exec.CommandContext(ctx, "resolvectl", "dns", iface.Name).Output()
			if err != nil {
				continue
			}
			_, values, ok := strings.Cut(string(raw), "):")
			if !ok {
				continue
			}
			for _, value := range strings.Fields(values) {
				server, _, _ := strings.Cut(value, "#")
				addr, err := netip.ParseAddr(server)
				if err != nil || addr.IsLoopback() || addr.IsUnspecified() {
					continue
				}
				dialer := net.Dialer{
					Control: func(_, _ string, raw syscall.RawConn) error {
						var bindingErr error
						if err := raw.Control(func(fd uintptr) {
							bindingErr = unix.BindToDevice(int(fd), iface.Name)
						}); err != nil {
							return err
						}
						return bindingErr
					},
				}
				connection, err := dialer.DialContext(ctx, network, netip.AddrPortFrom(addr, 53).String())
				if err == nil {
					return connection, nil
				}
			}
		}
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	return nil, ErrEndpointDNS
}

type linuxRoute struct {
	Device      string `json:"dev"`
	Destination string `json:"dst"`
	Metric      uint32 `json:"metric"`
}

func linuxUplink(ctx context.Context, family string, addr netip.Addr) (string, error) {
	zone := addr.Zone()
	addr = addr.WithZone("")
	args := []string{"get", addr.String()}
	if zone != "" {
		args = append(args, "oif", zone)
	}
	routes, err := readLinuxRoutes(ctx, family, args...)
	if err != nil {
		return "", err
	}
	name := firstLinuxDevice(routes)
	if name == profile.RuntimeTUNName {
		// A loaded runtime may capture the unbound lookup. Find the most
		// specific uplink in main and verify the kernel can use that device.
		prefix := netip.PrefixFrom(addr, addr.BitLen())
		routes, err := readLinuxRoutes(ctx, family, "show", "table", "main", "match", prefix.String())
		if err != nil {
			return "", err
		}
		name = bestLinuxMainDevice(routes, addr)
		if name == "" {
			return "", ErrUplinkUnavailable
		}
		routes, err = readLinuxRoutes(ctx, family, "get", addr.String(), "oif", name)
		if err != nil {
			return "", err
		}
		if firstLinuxDevice(routes) != name {
			return "", ErrUplinkUnavailable
		}
	}
	iface, err := net.InterfaceByName(name)
	if err != nil || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagPointToPoint != 0 ||
		(iface.Flags&net.FlagLoopback != 0 && !addr.IsLoopback()) || name == profile.RuntimeTUNName {
		return "", ErrUplinkUnavailable
	}
	return name, nil
}

func readLinuxRoutes(ctx context.Context, family string, args ...string) ([]linuxRoute, error) {
	raw, err := exec.CommandContext(ctx, "ip", append([]string{"-j", family, "route"}, args...)...).Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	var routes []linuxRoute
	if err != nil || json.Unmarshal(raw, &routes) != nil {
		return nil, ErrUplinkUnavailable
	}
	return routes, nil
}

func firstLinuxDevice(routes []linuxRoute) string {
	if len(routes) == 0 {
		return ""
	}
	return routes[0].Device
}

func bestLinuxMainDevice(routes []linuxRoute, addr netip.Addr) string {
	var best linuxRoute
	bits := -1
	for _, route := range routes {
		if route.Device == "" || route.Device == profile.RuntimeTUNName {
			continue
		}
		size := 0
		if route.Destination != "default" {
			prefix, err := netip.ParsePrefix(route.Destination)
			if err != nil {
				ip, err := netip.ParseAddr(route.Destination)
				if err != nil {
					continue
				}
				prefix = netip.PrefixFrom(ip, ip.BitLen())
			}
			if !prefix.Contains(addr) {
				continue
			}
			size = prefix.Bits()
		}
		if size > bits || (size == bits && route.Metric < best.Metric) {
			best, bits = route, size
		}
	}
	return best.Device
}

func (b *linuxUplinkBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	v4, v6 := b.v4, b.v6
	b.mu.Unlock()
	if v4 == "" && v6 == "" {
		return nil, 0, ErrUplinkUnavailable
	}
	recvs, port, err := b.Bind.Open(port)
	if err != nil {
		return nil, 0, err
	}
	hasV4, hasV6 := false, false
	for _, receive := range recvs {
		switch receive.PrettyName() {
		case "v4":
			hasV4 = true
		case "v6":
			hasV6 = true
		}
	}
	if (v4 != "" && !hasV4) || (v6 != "" && !hasV6) {
		_ = b.Bind.Close()
		return nil, 0, ErrUplinkUnavailable
	}
	sockets := b.Bind.(conn.PeekLookAtSocketFd)
	if err := bindLinuxSocket(v4, sockets.PeekLookAtSocketFd4); err != nil {
		_ = b.Bind.Close()
		return nil, 0, err
	}
	if err := bindLinuxSocket(v6, sockets.PeekLookAtSocketFd6); err != nil {
		_ = b.Bind.Close()
		return nil, 0, err
	}
	return recvs, port, nil
}

func (b *linuxUplinkBind) Send(buffers [][]byte, endpoint conn.Endpoint) error {
	b.mu.Lock()
	bound := b.v6
	if endpoint.DstIP().Is4() {
		bound = b.v4
	}
	b.mu.Unlock()
	if bound == "" {
		return ErrUplinkUnavailable
	}
	return b.Bind.Send(buffers, endpoint)
}

func (b *linuxUplinkBind) watchUplink(ctx context.Context, reopen func() error, closeBind func() error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return
	}
	events := os.NewFile(uintptr(fd), "naga-awg-uplink")
	defer events.Close()
	groups := uint32(unix.RTMGRP_LINK | unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE |
		unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		return
	}
	stop := context.AfterFunc(ctx, func() { _ = events.Close() })
	defer stop()
	buffer := make([]byte, 64*1024)
	retry := false
	for ctx.Err() == nil {
		if !retry {
			if _, err := events.Read(buffer); err != nil {
				return
			}
		}
		delay := 300 * time.Millisecond
		if retry {
			delay = 2 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
		v4, v6, err := linuxUplinks(lookup, b.endpoints)
		cancel()
		if ctx.Err() != nil {
			return
		}
		b.mu.Lock()
		changed := b.v4 != v4 || b.v6 != v6
		b.v4, b.v6 = v4, v6
		b.mu.Unlock()
		if err != nil {
			_ = closeBind()
			retry = false
			continue
		}
		if changed || retry {
			retry = reopen() != nil
		}
	}
}

func bindLinuxSocket(name string, socketFD func() (int, error)) error {
	if name == "" {
		return nil
	}
	fd, err := socketFD()
	if err != nil || unix.BindToDevice(fd, name) != nil {
		return ErrUplinkUnavailable
	}
	return nil
}
