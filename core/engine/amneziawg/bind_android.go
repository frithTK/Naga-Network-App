//go:build android

package amneziawg

import (
	"context"
	"net"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"

	"naga.network/core/androidvpn"
	awgcfg "naga.network/core/profile/amneziawg"
)

type androidProtectedBind struct{ conn.Bind }

func platformEndpointResolver() endpointResolver { return net.DefaultResolver }

func newPlatformBind(context.Context, awgcfg.Config) (conn.Bind, error) {
	if androidvpn.ProtectSocket == nil {
		return nil, ErrUplinkUnavailable
	}
	return &androidProtectedBind{Bind: conn.NewDefaultBind()}, nil
}

func (b *androidProtectedBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	recvs, port, err := b.Bind.Open(port)
	if err != nil {
		return nil, 0, err
	}
	sockets := b.Bind.(conn.PeekLookAtSocketFd)
	for _, receive := range recvs {
		var getFD func() (int, error)
		switch receive.PrettyName() {
		case "v4":
			getFD = sockets.PeekLookAtSocketFd4
		case "v6":
			getFD = sockets.PeekLookAtSocketFd6
		default:
			_ = b.Bind.Close()
			return nil, 0, ErrUplinkUnavailable
		}
		fd, err := getFD()
		if err != nil || fd < 0 || !androidvpn.ProtectSocket(fd) {
			_ = b.Bind.Close()
			return nil, 0, ErrUplinkUnavailable
		}
	}
	return recvs, port, nil
}
