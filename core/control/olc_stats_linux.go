//go:build linux

package control

func readPlatformIfaceBytes(name string) (tx, rx int64, err error) {
	tx, err = readCounter("/sys/class/net/" + name + "/statistics/tx_bytes")
	if err != nil {
		return 0, 0, err
	}
	rx, err = readCounter("/sys/class/net/" + name + "/statistics/rx_bytes")
	return tx, rx, err
}
