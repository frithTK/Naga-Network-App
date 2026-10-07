package control

import (
	"os"
	"path/filepath"

	"naga.network/core/androidvpn"
	engineolc "naga.network/core/engine/olcrtc"
	olcprofile "naga.network/core/profile/olcrtc"
)

func (c *RuntimeController) startAndroidHevLocked(profileID, ipv4 string, socksPort int) (engineolc.LinkState, error) {
	binary, err := engineolc.DiscoverTunnel(c.HevBinary)
	if err != nil {
		return engineolc.LinkState{}, err
	}
	dir := olcprofile.ConfigDir(c.Store.Root, profileID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return engineolc.LinkState{}, err
	}
	path := filepath.Join(dir, "hev-fd.yml")
	body := androidvpn.FDConfigYAML(androidvpn.MTU, ipv4, androidvpn.SocksHost, socksPort)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return engineolc.LinkState{}, err
	}
	if err := androidvpn.StartHev(binary, path); err != nil {
		return engineolc.LinkState{}, err
	}
	return engineolc.LinkState{}, nil
}
