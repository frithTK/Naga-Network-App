package control

import (
	"context"
	"errors"
	"time"

	"naga.network/core/engine"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

func (c *RuntimeController) startOlcRTCLocked(stage *string, value profile.Profile) (RuntimeSnapshot, error) {
	*stage = "prepare_olcrtc"
	cfg := profile.RuntimeOlcRTC(value.Config)
	if cfg == nil {
		if doc, err := olcprofile.ParseDocument(value.Config); err == nil {
			cfg = doc
		}
	}
	if cfg == nil && olcprofile.IsURI(value.Config) {
		parsed, err := olcprofile.ParseURI(string(value.Config))
		if err != nil {
			c.lastError = err.Error()
			return c.snapshotLocked(), err
		}
		if !parsed.RuntimeAllowed {
			c.lastError = olcprofile.ErrTransportOff.Error()
			return c.snapshotLocked(), olcprofile.ErrTransportOff
		}
		cfg = &olcprofile.Config{Profiles: []olcprofile.Profile{parsed}, URIs: []string{string(value.Config)}}
	}
	if cfg == nil {
		c.lastError = "olcrtc mode is not available"
		return c.snapshotLocked(), errors.New(c.lastError)
	}
	narrowed, activeTag, err := olcprofile.Narrow(cfg, value.SelectedMode)
	if err != nil {
		c.lastError = "olcrtc profile is not available"
		return c.snapshotLocked(), err
	}
	cfg = narrowed
	doc, err := olcprofile.Build(cfg)
	if err != nil {
		c.lastError = err.Error()
		return c.snapshotLocked(), err
	}
	yamlPath, err := olcprofile.WriteFile(olcprofile.ConfigDir(c.Store.Root, value.ID), doc)
	if err != nil {
		c.lastError = "olcrtc config could not be written"
		return c.snapshotLocked(), err
	}
	*stage = "olcrtc_binary"
	binary, err := engineolc.DiscoverBinary(c.OlcRTCBinary)
	if err != nil {
		c.lastError = "olcrtc binary was not found"
		return c.snapshotLocked(), err
	}
	*stage = "olcrtc_start"
	proc, err := engineolc.Start(context.Background(), binary, yamlPath)
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	readyCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	err = proc.Ready(readyCtx)
	cancel()
	if err != nil {
		_ = proc.Stop()
		c.lastError = "olcrtc socks port was not ready"
		return c.snapshotLocked(), err
	}
	clearStuckOlcProxy()
	*stage = "tun"
	link, err := c.bringUpOlcTunnelLocked(value.ID, cfg)
	if err != nil {
		_ = proc.Stop()
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	if c.Journal != nil {
		proc.SetLineHandler(func(line string) {
			c.Journal.Info("olcrtc", "process_log", olcprofile.RedactLine(line, cfg.Profiles), nil)
		})
	}
	c.olcProc = proc
	c.olcLink = link
	c.olcLinkOn = c.olcHost == nil
	c.proxyRestore = nil
	c.dnsRestore = nil
	c.profileID = value.ID
	c.sessionIDs = map[string]struct{}{value.ID: {}}
	c.activeEngine = profile.EngineOlcRTC
	c.activeRuntimeTag = activeTag
	c.lastError = ""
	c.startOlcStatsLocked()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = c.probeOlcRTC(ctx)
	}()
	*stage = "complete"
	return c.snapshotLocked(), nil
}

func (c *RuntimeController) ActiveFetchProxy() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.olcProc != nil {
		return "socks5://127.0.0.1:10808"
	}
	if c.runtime != nil {
		status := c.runtime.Status()
		if status == engine.Starting || status == engine.Connected || status == engine.Stopping {
			return "http://127.0.0.1:2080"
		}
	}
	return ""
}

func (c *RuntimeController) refreshOlcRTCLocked(value profile.Profile) (RuntimeSnapshot, error) {
	next := profile.RuntimeOlcRTC(value.Config)
	if next == nil {
		if err := c.stopOlcRTCLocked(); err != nil {
			return c.snapshotLocked(), err
		}
		if !olcMatches(value.Config, value.SelectedMode) {
			value.SelectedMode = ""
		}
		if err := c.Store.SaveVersioned(value); err != nil {
			return c.snapshotLocked(), err
		}
		return c.snapshotLocked(), nil
	}
	current, err := c.Store.Load(value.ID)
	if err != nil {
		return c.snapshotLocked(), err
	}
	old := profile.RuntimeOlcRTC(current.Config)
	if olcSame(old, next) {
		if err := c.Store.SaveVersioned(value); err != nil {
			return c.snapshotLocked(), err
		}
		return c.snapshotLocked(), nil
	}
	if next == nil {
		if err := c.Store.SaveVersioned(value); err != nil {
			return c.snapshotLocked(), err
		}
		return c.snapshotLocked(), nil
	}
	doc, err := olcprofile.Build(next)
	if err != nil {
		return c.snapshotLocked(), err
	}
	dir := olcprofile.ConfigDir(c.Store.Root, value.ID)
	if _, err := olcprofile.WriteFile(dir, doc); err != nil {
		return c.snapshotLocked(), err
	}
	if err := c.Store.SaveVersioned(value); err != nil {
		return c.snapshotLocked(), err
	}
	if err := c.stopOlcRTCLocked(); err != nil {
		return c.snapshotLocked(), err
	}
	stage := "refresh_olcrtc"
	snapshot, err := c.startOlcRTCLocked(&stage, value)
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func olcSame(left, right *olcprofile.Config) bool {
	if left == nil || right == nil || len(left.Profiles) != len(right.Profiles) {
		return false
	}
	for i := range left.Profiles {
		if left.Profiles[i].Key != right.Profiles[i].Key || left.Profiles[i].Room != right.Profiles[i].Room || left.Profiles[i].Provider != right.Profiles[i].Provider {
			return false
		}
	}
	return true
}

func (c *RuntimeController) stopOlcRTCLocked() error {
	if c.olcProc == nil && c.activeEngine != profile.EngineOlcRTC && !c.olcLinkOn {
		return nil
	}
	c.stopOlcStatsLocked()
	var err error
	if c.olcHost != nil {
		err = c.olcHost.Stop()
		c.olcHost = nil
	}
	if c.olcLinkOn {
		err = errors.Join(err, engineolc.RestoreLink(osExecer{}, c.olcLink))
		c.olcLinkOn = false
	}
	if c.olcHev != nil && c.olcHev.Process != nil {
		_ = c.olcHev.Process.Kill()
		_, _ = c.olcHev.Process.Wait()
		c.olcHev = nil
	}
	if c.olcProc != nil {
		err = errors.Join(err, c.olcProc.Stop())
		c.olcProc = nil
	}
	if c.activeEngine == profile.EngineOlcRTC {
		c.activeEngine = ""
		c.activeRuntimeTag = ""
	}
	return err
}

// SwitchEngine stops the running engine and starts the selected one.
// SelectedMode is stored by the caller only after a successful start.
func (c *RuntimeController) SwitchEngine(profileID, mode string) (snapshot RuntimeSnapshot, resultErr error) {
	stage := "switch"
	c.mu.Lock()
	defer c.mu.Unlock()
	value, err := c.Store.Load(profileID)
	if err != nil {
		return c.snapshotLocked(), err
	}
	previous := value.SelectedMode
	wasRunning := c.IsRuntimeActiveLocked() || c.olcProc != nil
	if err := c.stopForSwitchLocked(); err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	value.SelectedMode = mode
	snapshot, err = c.startSelectedLocked(&stage, value)
	if err == nil {
		return snapshot, nil
	}
	if !wasRunning {
		return snapshot, err
	}
	value.SelectedMode = previous
	restored, restoreErr := c.startSelectedLocked(&stage, value)
	if restoreErr != nil {
		c.lastError = safeRuntimeError(err)
		return restored, err
	}
	c.lastError = safeRuntimeError(err)
	return restored, err
}

func (c *RuntimeController) stopForSwitchLocked() error {
	if err := c.stopOlcRTCLocked(); err != nil {
		return err
	}
	if c.runtime == nil {
		return c.stopSidecarsLocked()
	}
	err := c.runtime.Stop()
	if c.runtimeCancel != nil {
		c.runtimeCancel()
		c.runtimeCancel = nil
	}
	c.runtime = nil
	c.profileID = ""
	c.runtimeAliases = nil
	c.sessionIDs = nil
	c.resetProbeStateLocked()
	return errors.Join(err, c.stopSidecarsLocked())
}

func (c *RuntimeController) startSelectedLocked(stage *string, value profile.Profile) (RuntimeSnapshot, error) {
	if value.Subscription.Expired(nowUTC()) {
		c.lastError = "profile subscription has expired"
		return c.snapshotLocked(), errors.New(c.lastError)
	}
	if olcShouldStart(value) {
		return c.startOlcRTCLocked(stage, value)
	}
	connectionPolicy, err := c.Store.LoadConnectionPolicy()
	if err != nil {
		return c.snapshotLocked(), err
	}
	routing, err := c.Store.LoadRoutingPolicy()
	if err != nil {
		return c.snapshotLocked(), err
	}
	active := []profile.Profile{value}
	singBox := profile.SingBoxProfiles(active)
	awg := profile.AmneziaWGProfiles(active)
	if len(singBox) > 0 {
		return c.startUnifiedLocked(stage, value.ID, singBox, awg, connectionPolicy, routing)
	}
	if profile.IsAmneziaWG(value) || len(awg) > 0 {
		return c.startAmneziaOnlyLocked(stage, value.ID, awg, connectionPolicy, routing)
	}
	c.lastError = "profile has no supported VPN engine"
	return c.snapshotLocked(), errors.New(c.lastError)
}
