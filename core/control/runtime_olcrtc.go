package control

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"time"

	"naga.network/core/androidvpn"
	"naga.network/core/engine"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

var errOlcCleanupUncertain = errors.New("состояние очистки olcRTC не подтверждено")

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
	proc, err := engineolc.Start(c.startupContextLocked(), binary, yamlPath)
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	readyCtx, cancel := context.WithTimeout(c.startupContextLocked(), 8*time.Second)
	err = proc.Ready(readyCtx)
	cancel()
	if err != nil {
		if stopErr := proc.Stop(); stopErr != nil {
			c.lastError = "olcrtc socks port was not ready; cleanup could not be confirmed"
			return c.snapshotLocked(), errors.Join(err, errOlcCleanupUncertain)
		}
		c.lastError = "olcrtc socks port was not ready"
		return c.snapshotLocked(), err
	}
	*stage = "tun"
	link, err := c.bringUpOlcTunnelLocked(value.ID, cfg)
	if err != nil {
		_ = proc.Stop()
		c.lastError = safeRuntimeError(err)
		// Platform TUN setup currently has cleanup paths which cannot report
		// every restore failure. Do not start another session over unknown routes.
		return c.snapshotLocked(), errors.Join(err, errOlcCleanupUncertain)
	}
	if c.Journal != nil {
		proc.SetLineHandler(func(line string) {
			c.Journal.Info("olcrtc", "process_log", olcprofile.RedactLine(line, cfg.Profiles), nil)
		})
	}
	c.olcFailed = false
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
	probeCtx, probeCancel := context.WithTimeout(c.startupContextLocked(), 15*time.Second)
	c.olcProbeCancel = probeCancel
	tag := olcLiveProbeTag(cfg.Profiles, activeTag, value.SelectedMode)
	go func() {
		defer probeCancel()
		latency, err := measureSOCKSLatency(probeCtx, net.JoinHostPort(olcprofile.SOCKSHost, strconv.Itoa(olcprofile.SOCKSPort)), olcProbeURL)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.olcProc == proc && probeCtx.Err() == nil && tag != "" {
			c.recordOlcProbeLocked(value.ID, tag, latency, err)
		}
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
	current, err := c.Store.Load(value.ID)
	if err != nil {
		return c.snapshotLocked(), err
	}
	old := olcProfiles(current.Config)
	next := olcProfiles(value.Config)
	if len(next) == 0 {
		return c.snapshotLocked(), errors.New("обновлённая подписка не содержит доступного узла olcRTC")
	}
	oldSelected, _, err := olcprofile.Narrow(&olcprofile.Config{Profiles: old}, current.SelectedMode)
	if err != nil {
		return c.snapshotLocked(), err
	}
	nextSelected, _, err := olcprofile.Narrow(&olcprofile.Config{Profiles: next}, value.SelectedMode)
	if err != nil {
		return c.snapshotLocked(), err
	}
	if current.SelectedMode == value.SelectedMode && reflect.DeepEqual(oldSelected.Profiles, nextSelected.Profiles) {
		if err := c.Store.SaveVersioned(value); err != nil {
			return c.snapshotLocked(), err
		}
		return c.snapshotLocked(), nil
	}
	if _, err := olcprofile.Build(nextSelected); err != nil {
		return c.snapshotLocked(), err
	}
	binary, err := engineolc.DiscoverBinary(c.OlcRTCBinary)
	if err != nil {
		return c.snapshotLocked(), err
	}
	if err := engineolc.CheckVersion(c.startupContextLocked(), binary); err != nil {
		return c.snapshotLocked(), err
	}
	result, err := replaceOlcSession(current, value, c.stopOlcRTCLocked,
		func(next profile.Profile) (RuntimeSnapshot, error) {
			stage := "refresh_olcrtc"
			return c.startOlcRTCLocked(&stage, next)
		}, c.Store.SaveVersioned, c.snapshotLocked)
	if err != nil && c.olcProc == nil {
		c.olcFailed = true
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	return result, err
}

// replaceOlcSession does not persist a candidate until it starts. If starting
// or saving it fails, it attempts to restore the previous owned session.
func replaceOlcSession(previous, candidate profile.Profile, stop func() error, start func(profile.Profile) (RuntimeSnapshot, error), save func(profile.Profile) error, snapshot func() RuntimeSnapshot) (RuntimeSnapshot, error) {
	if err := stop(); err != nil {
		return snapshot(), fmt.Errorf("остановка прежнего olcRTC не завершилась: %w", err)
	}
	result, startErr := start(candidate)
	if startErr != nil {
		if errors.Is(startErr, errOlcCleanupUncertain) {
			return snapshot(), fmt.Errorf("новый olcRTC не запустился; прежний профиль сохранён, состояние сети требует проверки: %w", startErr)
		}
		if _, restoreErr := start(previous); restoreErr != nil {
			return snapshot(), errors.Join(fmt.Errorf("новый olcRTC не запустился: %w", startErr), fmt.Errorf("не удалось восстановить прежний olcRTC: %w", restoreErr))
		}
		return snapshot(), fmt.Errorf("новый olcRTC не запустился; прежняя сессия восстановлена: %w", startErr)
	}
	if saveErr := save(candidate); saveErr != nil {
		if stopErr := stop(); stopErr != nil {
			return snapshot(), errors.Join(fmt.Errorf("не удалось сохранить профиль: %w", saveErr), fmt.Errorf("не удалось остановить новую сессию: %w", stopErr))
		}
		if _, restoreErr := start(previous); restoreErr != nil {
			return snapshot(), errors.Join(fmt.Errorf("не удалось сохранить профиль: %w", saveErr), fmt.Errorf("не удалось восстановить прежний olcRTC: %w", restoreErr))
		}
		return snapshot(), fmt.Errorf("не удалось сохранить профиль; прежняя сессия восстановлена: %w", saveErr)
	}
	return result, nil
}

func (c *RuntimeController) stopOlcRTCLocked() error {
	if c.olcProbeCancel != nil {
		c.olcProbeCancel()
		c.olcProbeCancel = nil
	}
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
		<-c.olcHevDone
		c.olcHev = nil
	}
	androidvpn.StopHev()
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
	finish := c.beginProbeTransition()
	defer finish()
	if err := c.waitProbeCleanup(context.Background()); err != nil {
		return c.Status(), err
	}
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
		return errors.Join(c.restoreDNSLocked(), c.stopSidecarsLocked())
	}
	dnsErr := c.restoreDNSLocked()
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
	return errors.Join(dnsErr, err, c.stopSidecarsLocked())
}

func (c *RuntimeController) startSelectedLocked(stage *string, value profile.Profile) (RuntimeSnapshot, error) {
	c.runtimeConfigHashes = map[string]string{value.ID: probeConfigHash(value.Config)}
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
