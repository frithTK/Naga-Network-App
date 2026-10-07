package engine

import (
	"context"
	"time"
)

type Type string

const (
	SingBox   Type = "sing-box"
	Mihomo    Type = "mihomo"
	Xray      Type = "xray"
	AmneziaWG Type = "amneziawg"
)

type Status string

const (
	Stopped   Status = "stopped"
	Starting  Status = "starting"
	Connected Status = "connected"
	Stopping  Status = "stopping"
	Failed    Status = "failed"
)

type TrafficStats struct {
	Upload         int64
	Download       int64
	UploadRate     int64
	DownloadRate   int64
	SessionStarted time.Time
	LastUpdated    time.Time
	Available      bool
}

// RuntimeDetails contains safe information about the currently selected leaf
// outbound. It deliberately excludes server addresses and credentials.
type RuntimeDetails struct {
	Node     string
	Country  string
	Protocol string
	Latency  int
	Status   string
}

type Runtime interface {
	Stop() error
	Status() Status
	Stats() TrafficStats
}

// FailureReporter is implemented by runtimes that can expose an unexpected
// process exit to the control-plane without changing the common adapter API.
type FailureReporter interface {
	Failure() error
}

// DetailReporter is optional so existing embedders and test runtimes do not
// need to implement runtime metadata immediately.
type DetailReporter interface {
	Details() RuntimeDetails
}

// SelectorSwitcher is implemented by runtimes that can change a selector
// without stopping the VPN tunnel.
type SelectorSwitcher interface {
	Select(ctx context.Context, outbound string) error
}

// OutboundProber проверяет, что листовой outbound проходит HTTP
// connectivity check (generate_204). Работающий процесс и живой локальный
// API сами по себе не доказывают, что выбранный узел реально несёт трафик.
// Latency — второй успешный Clash /delay: приближение Clash Meta
// unified-delay на уровне Clash API, не патч ядра sing-box. При ошибке
// второго замера сохраняется первый.
type OutboundProber interface {
	Probe(ctx context.Context, outbound string, timeout time.Duration) (latencyMS int, err error)
}

type Adapter interface {
	Type() Type
	Validate(ctx context.Context, config []byte) error
	Start(ctx context.Context, config []byte) (Runtime, error)
}
