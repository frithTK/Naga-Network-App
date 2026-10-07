package network

import (
	"testing"

	"naga.network/core/policy"
)

func TestParseNetworkManagerOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   policy.NetworkClass
	}{
		{name: "wifi", output: "wifi:connected\nloopback:connected\n", want: policy.NetworkWiFi},
		{name: "ethernet", output: "ethernet:connected\n", want: policy.NetworkEthernet},
		{name: "cellular wins", output: "wifi:connected\ngsm:connected\n", want: policy.NetworkCellular},
		{name: "wwan", output: "wwan:connected\n", want: policy.NetworkCellular},
		{name: "lte", output: "lte:connected\n", want: policy.NetworkCellular},
		{name: "modem", output: "modem:connected\n", want: policy.NetworkCellular},
		{name: "wifi-p2p", output: "wifi-p2p:connected\n", want: policy.NetworkWiFi},
		{name: "unknown", output: "wifi:disconnected\n", want: policy.NetworkUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseNetworkManagerOutput(test.output); got != test.want {
				t.Fatalf("network class = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseNetworkManagerConnections(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   policy.NetworkClass
	}{
		{name: "wifi connection", output: "802-11-wireless:activated\n", want: policy.NetworkWiFi},
		{name: "ethernet connection", output: "802-3-ethernet:activated\n", want: policy.NetworkEthernet},
		{name: "gsm lte", output: "gsm:activated\n", want: policy.NetworkCellular},
		{name: "gsm wins over wifi", output: "802-11-wireless:activated\ngsm:activated\n", want: policy.NetworkCellular},
		{name: "ignores vpn", output: "vpn:activated\n802-11-wireless:activated\n", want: policy.NetworkWiFi},
		{name: "disconnected ignored", output: "gsm:deactivated\n", want: policy.NetworkUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseNetworkManagerConnections(test.output); got != test.want {
				t.Fatalf("network class = %q, want %q", got, test.want)
			}
		})
	}
}

func FuzzParseNetworkManagerOutput(f *testing.F) {
	f.Add("wifi:connected\n")
	f.Add("gsm:connected\n")
	f.Add("ethernet:connected\n")
	f.Add("wifi:connected\ngsm:connected\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, output string) {
		got := ParseNetworkManagerOutput(output)
		switch got {
		case policy.NetworkUnknown, policy.NetworkWiFi, policy.NetworkCellular, policy.NetworkEthernet:
		default:
			t.Fatalf("unexpected class %q", got)
		}
	})
}

func FuzzParseNetworkManagerConnections(f *testing.F) {
	f.Add("802-11-wireless:activated\n")
	f.Add("gsm:activated\n")
	f.Add("vpn:activated\n")
	f.Fuzz(func(t *testing.T, output string) {
		got := ParseNetworkManagerConnections(output)
		switch got {
		case policy.NetworkUnknown, policy.NetworkWiFi, policy.NetworkCellular, policy.NetworkEthernet:
		default:
			t.Fatalf("unexpected class %q", got)
		}
	})
}
