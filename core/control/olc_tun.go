package control

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"naga.network/core/androidvpn"
	engineolc "naga.network/core/engine/olcrtc"
	olcprofile "naga.network/core/profile/olcrtc"
)

type osExecer struct{ ctx context.Context }

func (e osExecer) parent() context.Context {
	if e.ctx != nil {
		return e.ctx
	}
	return context.Background()
}

func (e osExecer) Run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(e.parent(), 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}

func (e osExecer) Output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(e.parent(), 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func (c *RuntimeController) bringUpOlcTunnelLocked(profileID string, cfg *olcprofile.Config) (engineolc.LinkState, error) {
	if runtime.GOOS == "android" {
		return c.startAndroidHevLocked(profileID, androidvpn.OlcIPv4, androidvpn.OlcPort)
	}
	if runtime.GOOS == "windows" {
		return c.bringUpWindowsOlcTunnelLocked(profileID, cfg)
	}
	return c.bringUpDirectOlcTunnelLocked(profileID, cfg)
}

func (c *RuntimeController) bringUpWindowsOlcTunnelLocked(profileID string, cfg *olcprofile.Config) (engineolc.LinkState, error) {
	ips := roomAddressesContext(c.startupContextLocked(), cfg)
	if cfg != nil && len(cfg.Profiles) > 0 && len(ips) == 0 {
		return engineolc.LinkState{}, errors.New("olcRTC room address could not be resolved")
	}
	binary, err := engineolc.DiscoverTunnel(c.HevBinary)
	if err != nil {
		return engineolc.LinkState{}, err
	}
	configPath, err := engineolc.WriteTunnelConfig(olcprofile.ConfigDir(c.Store.Root, profileID))
	if err != nil {
		return engineolc.LinkState{}, err
	}
	ctx, cancel := context.WithTimeout(c.startupContextLocked(), 3*time.Minute)
	defer cancel()
	host, err := engineolc.StartElevatedTunnel(ctx, binary, configPath, ips)
	if err != nil {
		return engineolc.LinkState{}, err
	}
	c.olcHost = host
	return engineolc.LinkState{}, nil
}

func (c *RuntimeController) bringUpDirectOlcTunnelLocked(profileID string, cfg *olcprofile.Config) (engineolc.LinkState, error) {
	execer := osExecer{ctx: c.startupContextLocked()}
	link, err := pinOlcRoomContext(c.startupContextLocked(), execer, cfg)
	if err != nil {
		return engineolc.LinkState{}, err
	}
	binary, err := engineolc.DiscoverTunnel(c.HevBinary)
	if err != nil {
		_ = engineolc.RestoreLink(osExecer{}, link)
		return engineolc.LinkState{}, err
	}
	configPath, err := engineolc.WriteTunnelConfig(olcprofile.ConfigDir(c.Store.Root, profileID))
	if err != nil {
		_ = engineolc.RestoreLink(osExecer{}, link)
		return engineolc.LinkState{}, err
	}
	cmd := exec.Command(binary, configPath)
	if err := cmd.Start(); err != nil {
		_ = engineolc.RestoreLink(osExecer{}, link)
		return engineolc.LinkState{}, err
	}
	c.olcHev = cmd
	done := make(chan struct{})
	c.olcHevDone = done
	go func() { _ = cmd.Wait(); close(done) }()
	waitCtx, cancel := context.WithTimeout(c.startupContextLocked(), 8*time.Second)
	err = engineolc.WaitForIface(waitCtx, engineolc.TunnelIface)
	cancel()
	if err != nil {
		_ = cmd.Process.Kill()
		<-done
		c.olcHev = nil
		_ = engineolc.RestoreLink(osExecer{}, link)
		return engineolc.LinkState{}, err
	}
	if err := engineolc.EngageDefault(execer, link); err != nil {
		_ = engineolc.RestoreLink(osExecer{}, link)
		_ = cmd.Process.Kill()
		<-done
		c.olcHev = nil
		return engineolc.LinkState{}, err
	}
	return link, nil
}

func pinOlcRoom(execer engineolc.Execer, cfg *olcprofile.Config) (engineolc.LinkState, error) {
	return pinOlcRoomContext(context.Background(), execer, cfg)
}
func pinOlcRoomContext(ctx context.Context, execer engineolc.Execer, cfg *olcprofile.Config) (engineolc.LinkState, error) {
	link, err := engineolc.CaptureLink(execer)
	if err != nil {
		return engineolc.LinkState{}, err
	}
	link.RoomIPs = roomAddressesContext(ctx, cfg)
	if err := engineolc.BypassRoom(execer, link); err != nil {
		_ = engineolc.RestoreLink(restoreExecer(execer), link)
		return engineolc.LinkState{}, err
	}
	return link, nil
}

func roomAddresses(cfg *olcprofile.Config) []net.IP {
	return roomAddressesContext(context.Background(), cfg)
}

func roomAddressesContext(parent context.Context, cfg *olcprofile.Config) []net.IP {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ips []net.IP
	for _, profile := range cfg.Profiles {
		if ctx.Err() != nil {
			break
		}
		host := profile.Room
		if parsed, err := url.Parse(profile.Room); err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
		addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			continue
		}
		for _, ip := range addrs {
			if ip = ip.To16(); ip == nil || ip.IsLoopback() {
				continue
			}
			key := ip.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ips = append(ips, ip)
		}
	}
	return ips
}

// Acquisition cancellation must not cancel the rollback of already changed
// routes. Test/embedding executors keep their own cleanup policy.
func restoreExecer(execer engineolc.Execer) engineolc.Execer {
	if _, ok := execer.(osExecer); ok {
		return osExecer{}
	}
	return execer
}
