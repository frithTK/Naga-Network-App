// Package policy contains protocol selection rules independent of a platform
// or a particular engine configuration.
package policy

import (
	"sort"
	"strings"
	"time"
)

const (
	// HealthProbeInterval is how often a connected session re-checks
	// reachability. Auto mode re-probes up to AutoCandidateLimit non-TUIC
	// nodes and keeps the fastest; manual modes probe only the active leaf.
	HealthProbeInterval = 45 * time.Second
	// HealthFailThreshold consecutive probe failures of the active leaf
	// (manual modes) before the tag is put on cooldown and policy
	// selection runs again.
	HealthFailThreshold = 2
	// HealthCooldown keeps a failed tag out of the next policy selection.
	HealthCooldown = 2 * time.Minute
	// NetworkPollInterval is how often Linux asks NetworkManager for the
	// current connection class.
	NetworkPollInterval = 3 * time.Second
	// NetworkChangeDebounce coalesces interface flaps into one Select.
	NetworkChangeDebounce = 3 * time.Second
	// AutoCandidateLimit is how many non-TUIC nodes auto-select probes
	// in parallel, then enables the fastest of them.
	AutoCandidateLimit = 3
)

type NetworkClass string

const (
	NetworkUnknown  NetworkClass = "unknown"
	NetworkWiFi     NetworkClass = "wifi"
	NetworkCellular NetworkClass = "cellular"
	NetworkEthernet NetworkClass = "ethernet"
)

type Candidate struct {
	ID         string
	ProfileID  string
	SourceTag  string
	RuntimeTag string
	Country    string
	Protocol   string
	Type       string
	LatencyMS  int
}

// ProtocolPriority returns a lower value for a more preferred protocol.
// TUIC is deliberately last on every network. On cellular, VLESS is the
// preferred group; latency is used only as a tie-breaker within a group.
func ProtocolPriority(network NetworkClass, protocol string) int {
	if strings.EqualFold(strings.TrimSpace(protocol), "TUIC") {
		return 2
	}
	if network == NetworkCellular &&
		strings.EqualFold(strings.TrimSpace(protocol), "VLESS") {
		return 0
	}
	if strings.EqualFold(strings.TrimSpace(protocol), "AmneziaWG") {
		return 1
	}
	return 1
}

// RankCandidates returns a stable copy ordered according to the product
// policy. Unknown latency is placed after measured latency within a group.
func RankCandidates(network NetworkClass, candidates []Candidate) []Candidate {
	result := append([]Candidate(nil), candidates...)
	sort.SliceStable(result, func(i, j int) bool {
		leftPriority := ProtocolPriority(network, result[i].Protocol)
		rightPriority := ProtocolPriority(network, result[j].Protocol)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftLatency := result[i].LatencyMS
		rightLatency := result[j].LatencyMS
		if leftLatency == 0 {
			return false
		}
		if rightLatency == 0 {
			return true
		}
		return leftLatency < rightLatency
	})
	return result
}
