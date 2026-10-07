//go:build android

package control

import (
	"errors"

	"naga.network/core/androidvpn"
)

type androidTUNPreflight struct{}

func (androidTUNPreflight) CheckTUN() error {
	if androidvpn.TunFile() == nil {
		return errors.New("VPN interface is not ready")
	}
	return nil
}

func NewLinuxTUNPreflight() TUNPreflight {
	return androidTUNPreflight{}
}
