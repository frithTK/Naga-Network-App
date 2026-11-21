//go:build !linux && !android

package amneziawg

import (
	"context"
	"net"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	awgcfg "naga.network/core/profile/amneziawg"
)

func newPlatformBind(context.Context, awgcfg.Config) (conn.Bind, error) {
	return conn.NewDefaultBind(), nil
}

func platformEndpointResolver() endpointResolver { return net.DefaultResolver }
