//go:build windows

package control

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	internetOptionSettingsChanged     = 39
	internetOptionRefresh             = 37
	internetOptionPerConnectionOption = 75
	internetPerConnFlags              = 1
	internetPerConnProxyServer        = 2
	internetPerConnProxyBypass        = 3
	internetPerConnAutoconfigURL      = 4
	proxyTypeDirect                   = 1
	proxyTypeProxy                    = 2
	proxyTypeAutoProxyURL             = 4
)

type proxySettings struct {
	enable     uint32
	server     string
	override   string
	pac        string
	pacPresent bool
}

type proxyStore interface {
	snapshot() (proxySettings, error)
	apply(proxySettings) error
}

type proxyNotifier interface {
	apply(proxySettings) error
}

// WinINETSystemProxy writes HKCU Internet Settings and notifies WinINET.
type WinINETSystemProxy struct {
	store  proxyStore
	notify proxyNotifier
}

func NewSystemProxyManager() SystemProxyManager {
	return &WinINETSystemProxy{
		store:  registryProxyStore{},
		notify: wininetNotify{},
	}
}

func (m *WinINETSystemProxy) Configure(host string, port int) (func() error, error) {
	if strings.TrimSpace(host) == "" || port < 1 || port > 65535 {
		return nil, fmt.Errorf("system proxy address is invalid")
	}
	store := m.store
	if store == nil {
		store = registryProxyStore{}
	}
	notify := m.notify
	if notify == nil {
		notify = wininetNotify{}
	}
	previous, err := store.snapshot()
	if err != nil {
		return nil, err
	}
	next := proxySettings{
		enable:   1,
		server:   fmt.Sprintf("http=%s:%d;https=%s:%d;socks=%s:%d", host, port, host, port, host, port),
		override: mergeProxyOverride(previous.override, "localhost", "127.0.0.1", "<local>"),
	}
	rollback := func() error {
		var result error
		if err := store.apply(previous); err != nil {
			result = err
		}
		if err := notify.apply(previous); err != nil {
			result = errors.Join(result, err)
		}
		return result
	}
	if err := store.apply(next); err != nil {
		_ = rollback()
		return nil, fmt.Errorf("enable desktop proxy: %w", err)
	}
	if err := notify.apply(next); err != nil {
		_ = rollback()
		return nil, fmt.Errorf("notify desktop proxy: %w", err)
	}
	return rollback, nil
}

func mergeProxyOverride(existing string, extra ...string) string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	add := func(part string) {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	for _, part := range strings.FieldsFunc(existing, func(r rune) bool {
		return r == ';' || r == ','
	}) {
		add(part)
	}
	for _, part := range extra {
		add(part)
	}
	return strings.Join(out, ";")
}

type registryProxyStore struct{}

func (registryProxyStore) snapshot() (proxySettings, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return proxySettings{}, nil
		}
		return proxySettings{}, fmt.Errorf("read desktop proxy settings: %w", err)
	}
	defer key.Close()
	var settings proxySettings
	if value, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		settings.enable = uint32(value)
	} else if !errors.Is(err, registry.ErrNotExist) {
		return proxySettings{}, fmt.Errorf("read desktop proxy settings: %w", err)
	}
	if value, _, err := key.GetStringValue("ProxyServer"); err == nil {
		settings.server = value
	} else if !errors.Is(err, registry.ErrNotExist) {
		return proxySettings{}, fmt.Errorf("read desktop proxy settings: %w", err)
	}
	if value, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		settings.override = value
	} else if !errors.Is(err, registry.ErrNotExist) {
		return proxySettings{}, fmt.Errorf("read desktop proxy settings: %w", err)
	}
	if value, _, err := key.GetStringValue("AutoConfigURL"); err == nil {
		settings.pac = value
		settings.pacPresent = true
	} else if !errors.Is(err, registry.ErrNotExist) {
		return proxySettings{}, fmt.Errorf("read desktop proxy settings: %w", err)
	}
	return settings, nil
}

func (registryProxyStore) apply(settings proxySettings) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("write desktop proxy settings: %w", err)
	}
	defer key.Close()
	if err := key.SetDWordValue("ProxyEnable", settings.enable); err != nil {
		return fmt.Errorf("write desktop proxy settings: %w", err)
	}
	if err := key.SetStringValue("ProxyServer", settings.server); err != nil {
		return fmt.Errorf("write desktop proxy settings: %w", err)
	}
	if err := key.SetStringValue("ProxyOverride", settings.override); err != nil {
		return fmt.Errorf("write desktop proxy settings: %w", err)
	}
	if settings.pacPresent {
		if err := key.SetStringValue("AutoConfigURL", settings.pac); err != nil {
			return fmt.Errorf("write desktop proxy settings: %w", err)
		}
		return nil
	}
	if err := key.DeleteValue("AutoConfigURL"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("write desktop proxy settings: %w", err)
	}
	return nil
}

type wininetNotify struct{}

var (
	wininet                = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOptionW = wininet.NewProc("InternetSetOptionW")
)

type internetPerConnOption struct {
	Option uint32
	_      uint32
	Value  uintptr
}

type internetPerConnOptionList struct {
	Size        uint32
	_           uint32
	Connection  *uint16
	OptionCount uint32
	OptionError uint32
	Options     *internetPerConnOption
}

func (wininetNotify) apply(settings proxySettings) error {
	flags := uint32(proxyTypeDirect)
	if settings.enable != 0 {
		flags |= proxyTypeProxy
	}
	if settings.pacPresent && strings.TrimSpace(settings.pac) != "" {
		flags |= proxyTypeAutoProxyURL
	}
	server, err := windows.UTF16PtrFromString(settings.server)
	if err != nil {
		return err
	}
	bypass, err := windows.UTF16PtrFromString(settings.override)
	if err != nil {
		return err
	}
	pac, err := windows.UTF16PtrFromString(settings.pac)
	if err != nil {
		return err
	}
	opts := []internetPerConnOption{
		{Option: internetPerConnFlags, Value: uintptr(flags)},
		{Option: internetPerConnProxyServer, Value: uintptr(unsafe.Pointer(server))},
		{Option: internetPerConnProxyBypass, Value: uintptr(unsafe.Pointer(bypass))},
		{Option: internetPerConnAutoconfigURL, Value: uintptr(unsafe.Pointer(pac))},
	}
	list := internetPerConnOptionList{
		Size:        uint32(unsafe.Sizeof(internetPerConnOptionList{})),
		OptionCount: uint32(len(opts)),
		Options:     &opts[0],
	}
	if err := internetSetOption(internetOptionPerConnectionOption, unsafe.Pointer(&list), uint32(unsafe.Sizeof(list))); err != nil {
		return err
	}
	if err := internetSetOption(internetOptionSettingsChanged, nil, 0); err != nil {
		return err
	}
	return internetSetOption(internetOptionRefresh, nil, 0)
}

func internetSetOption(option uint32, value unsafe.Pointer, length uint32) error {
	r1, _, err := procInternetSetOptionW.Call(0, uintptr(option), uintptr(value), uintptr(length))
	if r1 == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return err
		}
		return errors.New("InternetSetOption failed")
	}
	return nil
}

func clearStuckOlcProxy() {
	store := registryProxyStore{}
	current, err := store.snapshot()
	if err != nil {
		return
	}
	if !strings.Contains(strings.ToLower(current.server), ":10808") {
		return
	}
	disabled := current
	disabled.enable = 0
	disabled.server = ""
	_ = store.apply(disabled)
	_ = wininetNotify{}.apply(disabled)
}
