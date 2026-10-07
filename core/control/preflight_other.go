//go:build !linux && !windows

package control

import "errors"

type unsupportedTUNPreflight struct{}

func (unsupportedTUNPreflight) CheckTUN() error {
	return errors.New("TUN пока не реализован на этой платформе")
}

func NewLinuxTUNPreflight() TUNPreflight {
	return unsupportedTUNPreflight{}
}
