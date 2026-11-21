package control

import (
	"context"
	"errors"
	"time"

	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/policy"
	awgcfg "naga.network/core/profile/amneziawg"
)

const (
	awgProbeSource = "awg_socks"
	awgPrimaryURL  = "https://www.gstatic.com/generate_204"
	awgReserveURL  = "https://cp.cloudflare.com/generate_204"
)

func (c *RuntimeController) registerAWGProbeLocked(candidate policy.Candidate, sidecar awgengine.Sidecar) {
	if prober, ok := sidecar.(awgengine.HTTPSProber); ok {
		if c.awgProbers == nil {
			c.awgProbers = make(map[string]awgengine.HTTPSProber)
		}
		c.awgProbers[candidate.RuntimeTag] = prober
	}
}

func probeAWGHTTPS(parent context.Context, candidate policy.Candidate, prober awgengine.HTTPSProber, timeout time.Duration, reserve bool) candidateProbeResult {
	if standby, ok := prober.(*deferredAWG); ok {
		if err := standby.Ready(parent); err != nil {
			return candidateProbeResult{candidate: candidate, err: err}
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	result := candidateProbeResult{candidate: candidate, source: awgProbeSource, target: "gstatic"}
	primary := ctx
	cancelPrimary := func() {}
	if reserve {
		primary, cancelPrimary = context.WithTimeout(ctx, timeout*2/3)
	}
	result.latency, result.err = prober.ProbeHTTPS(primary, awgPrimaryURL)
	cancelPrimary()
	if reserve && ctx.Err() == nil && isAWGHTTPSFailure(result.err) {
		result.target = "cloudflare"
		result.latency, result.err = prober.ProbeHTTPS(ctx, awgReserveURL)
	}
	return result
}

func isAWGHTTPSFailure(err error) bool {
	return errors.Is(err, awgengine.ErrHTTPSRequest) || errors.Is(err, awgengine.ErrHTTPSStatus) ||
		errors.Is(err, context.DeadlineExceeded)
}

func probeFailureReason(result candidateProbeResult) string {
	switch {
	case errors.Is(result.err, awgengine.ErrHandshakeTimeout):
		return "awg_handshake_timeout"
	case errors.Is(result.err, awgengine.ErrEndpointDNS):
		return "awg_endpoint_dns"
	case errors.Is(result.err, awgcfg.ErrPeerEndpoint):
		return "awg_endpoint_invalid"
	case errors.Is(result.err, awgengine.ErrDeviceConfiguration):
		return "awg_configuration"
	case errors.Is(result.err, awgengine.ErrBindingInUse):
		return "awg_in_use"
	case errors.Is(result.err, awgengine.ErrUplinkUnavailable):
		return "awg_uplink_unavailable"
	case errors.Is(result.err, awgengine.ErrHTTPSStatus):
		return "https_status"
	case errors.Is(result.err, awgengine.ErrHTTPSRequest):
		return "https_failed"
	case errors.Is(result.err, context.Canceled):
		return "cancelled"
	case errors.Is(result.err, context.DeadlineExceeded):
		if result.source == awgProbeSource {
			return "https_timeout"
		}
		return "timeout"
	default:
		return "probe_failed"
	}
}
