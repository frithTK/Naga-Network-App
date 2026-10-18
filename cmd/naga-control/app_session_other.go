//go:build !darwin && !linux

package main

import "fmt"

func lockAppSession(string) (func(), error) {
	return nil, fmt.Errorf("application child sessions are not implemented on this platform")
}

func validateSessionToken(string) error { return nil }
