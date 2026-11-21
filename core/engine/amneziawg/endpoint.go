package amneziawg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	awgcfg "naga.network/core/profile/amneziawg"
)

var (
	ErrEndpointDNS         = errors.New("AmneziaWG: не удалось разрешить имя VPN-сервера")
	ErrDeviceConfiguration = errors.New("AmneziaWG: движок отклонил конфигурацию")
)

const (
	endpointResolveTimeout = 5 * time.Second
	maxEndpointAttempts    = 2
)

type endpointResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// The UAPI accepts IP endpoints only. Keep the original profile intact.
func resolveEndpoints(ctx context.Context, cfg awgcfg.Config, resolver endpointResolver) ([]awgcfg.Config, error) {
	var choices [maxEndpointAttempts]awgcfg.Config
	for i := range choices {
		choices[i] = cfg
		choices[i].Peers = append([]awgcfg.Peer(nil), cfg.Peers...)
	}
	addresses := make(map[string][]netip.Addr)
	count := 1
	for index, peer := range cfg.Peers {
		host, port, err := net.SplitHostPort(strings.TrimSpace(peer.Endpoint))
		if err != nil || host == "" {
			return nil, awgcfg.ErrPeerEndpoint
		}
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return nil, awgcfg.ErrPeerEndpoint
		}
		addr, err := netip.ParseAddr(host)
		candidates := []netip.Addr{addr}
		if err != nil {
			if strings.ContainsAny(host, ":%[]/ \\@\t\r\n") {
				return nil, awgcfg.ErrPeerEndpoint
			}
			candidates = addresses[host]
			if len(candidates) == 0 {
				found, err := resolver.LookupNetIP(ctx, "ip", host)
				if ctx.Err() != nil {
					return nil, context.Cause(ctx)
				}
				if err != nil {
					return nil, ErrEndpointDNS
				}
				for _, candidate := range found {
					candidate = candidate.Unmap()
					if !candidate.IsValid() || (len(candidates) != 0 && candidate == candidates[0]) {
						continue
					}
					candidates = append(candidates, candidate)
					if len(candidates) >= maxEndpointAttempts {
						break
					}
				}
				if len(candidates) == 0 {
					return nil, ErrEndpointDNS
				}
				addresses[host] = candidates
			}
		}
		count = max(count, len(candidates))
		for attempt := range choices {
			addr := candidates[min(attempt, len(candidates)-1)].Unmap()
			choices[attempt].Peers[index].Endpoint = netip.AddrPortFrom(addr, uint16(portNumber)).String()
		}
	}
	return choices[:count], nil
}
