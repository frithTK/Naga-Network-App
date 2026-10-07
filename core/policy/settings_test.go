package policy

import "testing"

func TestConnectionPolicyRejectsSystemProxyOnAndroid(t *testing.T) {
	prev := currentGOOS
	currentGOOS = "android"
	t.Cleanup(func() { currentGOOS = prev })
	policy := DefaultConnectionPolicy()
	policy.TrafficMode = TrafficSystemProxy
	if err := policy.Validate(); err == nil {
		t.Fatal("expected system_proxy to be rejected on Android")
	}
}

func TestConnectionPolicyAllowsSystemProxyOffAndroid(t *testing.T) {
	prev := currentGOOS
	currentGOOS = "linux"
	t.Cleanup(func() { currentGOOS = prev })
	policy := DefaultConnectionPolicy()
	policy.TrafficMode = TrafficSystemProxy
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
}
