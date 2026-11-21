package amneziawg

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"

	awgcfg "naga.network/core/profile/amneziawg"
)

var activeBindings = struct {
	sync.Mutex
	values map[string]struct{}
}{values: make(map[string]struct{})}

// Concurrent devices using one client/server key pair can move the server's
// remembered client endpoint. Keep that pair and any fixed UDP port exclusive.
func reserveBinding(cfg awgcfg.Config) (func(), error) {
	keys := make([]string, 0, len(cfg.Peers)+1)
	private, err := keyToHex(cfg.PrivateKey)
	if err != nil {
		return nil, ErrDeviceConfiguration
	}
	raw, err := hex.DecodeString(private)
	if err != nil {
		return nil, ErrDeviceConfiguration
	}
	client, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return nil, ErrDeviceConfiguration
	}
	clientPublic := string(client.PublicKey().Bytes())
	for _, peer := range cfg.Peers {
		public, err := keyToHex(peer.PublicKey)
		if err != nil {
			return nil, ErrDeviceConfiguration
		}
		hash := sha256.Sum256([]byte(clientPublic + public))
		keys = append(keys, string(hash[:]))
	}
	if cfg.ListenPort > 0 {
		keys = append(keys, "port:"+strconv.Itoa(cfg.ListenPort))
	}
	activeBindings.Lock()
	defer activeBindings.Unlock()
	for _, key := range keys {
		if _, inUse := activeBindings.values[key]; inUse {
			return nil, ErrBindingInUse
		}
	}
	for _, key := range keys {
		activeBindings.values[key] = struct{}{}
	}
	return func() {
		activeBindings.Lock()
		defer activeBindings.Unlock()
		for _, key := range keys {
			delete(activeBindings.values, key)
		}
	}, nil
}
