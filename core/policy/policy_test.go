package policy

import (
	"testing"
)

func TestRankCandidatesKeepsTUICLastEvenWithLowerLatency(t *testing.T) {
	candidates := []Candidate{
		{ID: "tuic", Protocol: "TUIC", LatencyMS: 10},
		{ID: "vless", Protocol: "VLESS", LatencyMS: 100},
		{ID: "hy2", Protocol: "Hysteria2", LatencyMS: 20},
	}

	got := RankCandidates(NetworkCellular, candidates)
	if got[0].ID != "vless" || got[1].ID != "hy2" || got[2].ID != "tuic" {
		t.Fatalf("cellular order = %#v", got)
	}
}

func TestRankCandidatesUsesLatencyInsideNonTUICGroup(t *testing.T) {
	candidates := []Candidate{
		{ID: "slow", Protocol: "Trojan", LatencyMS: 100},
		{ID: "fast", Protocol: "VLESS", LatencyMS: 20},
		{ID: "unknown", Protocol: "Hysteria2"},
	}

	got := RankCandidates(NetworkWiFi, candidates)
	if got[0].ID != "fast" || got[1].ID != "slow" || got[2].ID != "unknown" {
		t.Fatalf("wifi order = %#v", got)
	}
}

func TestRankCandidatesKeepsAmneziaWGAheadOfTUIC(t *testing.T) {
	candidates := []Candidate{
		{ID: "tuic", Protocol: "TUIC", LatencyMS: 10},
		{ID: "awg", Protocol: "AmneziaWG", LatencyMS: 10},
		{ID: "vless", Protocol: "VLESS", LatencyMS: 10},
	}

	got := RankCandidates(NetworkWiFi, candidates)
	if got[len(got)-1].ID != "tuic" {
		t.Fatalf("TUIC should be last, got %#v", got)
	}
	if got[0].ID != "awg" && got[1].ID != "awg" {
		t.Fatalf("AmneziaWG should share the non-TUIC group: %#v", got)
	}
}
