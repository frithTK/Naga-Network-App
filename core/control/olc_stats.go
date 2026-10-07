package control

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	engineolc "naga.network/core/engine/olcrtc"
)

func (c *RuntimeController) startOlcStatsLocked() {
	if c.olcStatsCancel != nil {
		c.olcStatsCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.olcStatsCancel = cancel
	go c.pollOlcStats(ctx)
}

func (c *RuntimeController) stopOlcStatsLocked() {
	if c.olcStatsCancel != nil {
		c.olcStatsCancel()
		c.olcStatsCancel = nil
	}
	c.olcStatsOK = false
}

func (c *RuntimeController) pollOlcStats(ctx context.Context) {
	var prevTx, prevRx int64
	var havePrev bool
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tx, rx, err := readIfaceBytes(engineolc.TunnelIface)
			if err != nil {
				c.mu.Lock()
				c.olcStatsOK = false
				c.mu.Unlock()
				havePrev = false
				continue
			}
			c.mu.Lock()
			c.olcUpload = tx
			c.olcDownload = rx
			if havePrev {
				c.olcUploadRate, c.olcDownloadRate = engineolc.TrafficRates(prevTx, prevRx, tx, rx, 1)
			}
			c.olcStatsAt = time.Now()
			c.olcStatsOK = true
			c.mu.Unlock()
			prevTx, prevRx, havePrev = tx, rx, true
		}
	}
}

func readIfaceBytes(name string) (tx, rx int64, err error) {
	return readPlatformIfaceBytes(name)
}

func readCounter(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}
