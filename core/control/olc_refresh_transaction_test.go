package control

import (
	"errors"
	"reflect"
	"testing"

	"naga.network/core/profile"
)

func TestReplaceOlcSessionPreservesOldOnCandidateFailure(t *testing.T) {
	old := profile.Profile{ID: "old"}
	next := profile.Profile{ID: "new"}
	var calls []string
	stop := func() error { calls = append(calls, "stop"); return nil }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		if p.ID == "new" {
			return RuntimeSnapshot{}, errors.New("start failed")
		}
		return RuntimeSnapshot{}, nil
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return nil }
	_, err := replaceOlcSession(old, next, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if err == nil || !reflect.DeepEqual(calls, []string{"stop", "start:new", "start:old"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}

func TestReplaceOlcSessionRestoresOldWhenSaveFails(t *testing.T) {
	old := profile.Profile{ID: "old"}
	next := profile.Profile{ID: "new"}
	var calls []string
	stop := func() error { calls = append(calls, "stop"); return nil }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		return RuntimeSnapshot{}, nil
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return errors.New("write failed") }
	_, err := replaceOlcSession(old, next, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if err == nil || !reflect.DeepEqual(calls, []string{"stop", "start:new", "save:new", "stop", "start:old"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}

func TestReplaceOlcSessionDoesNotRestartOverUncertainCleanup(t *testing.T) {
	var calls []string
	stop := func() error { calls = append(calls, "stop"); return nil }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		return RuntimeSnapshot{}, errOlcCleanupUncertain
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return nil }
	_, err := replaceOlcSession(profile.Profile{ID: "old"}, profile.Profile{ID: "new"}, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if !errors.Is(err, errOlcCleanupUncertain) || !reflect.DeepEqual(calls, []string{"stop", "start:new"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}

func TestReplaceOlcSessionSavesOnlyAfterSuccessfulStart(t *testing.T) {
	var calls []string
	stop := func() error { calls = append(calls, "stop"); return nil }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		return RuntimeSnapshot{}, nil
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return nil }
	_, err := replaceOlcSession(profile.Profile{ID: "old"}, profile.Profile{ID: "new"}, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if err != nil || !reflect.DeepEqual(calls, []string{"stop", "start:new", "save:new"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}

func TestReplaceOlcSessionStopsWhenOldSessionCannotStop(t *testing.T) {
	var calls []string
	stop := func() error { calls = append(calls, "stop"); return errors.New("stop failed") }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		return RuntimeSnapshot{}, nil
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return nil }
	_, err := replaceOlcSession(profile.Profile{ID: "old"}, profile.Profile{ID: "new"}, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if err == nil || !reflect.DeepEqual(calls, []string{"stop"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}

func TestReplaceOlcSessionReportsFailedRestoration(t *testing.T) {
	var calls []string
	startErr := errors.New("candidate failed")
	restoreErr := errors.New("restore failed")
	stop := func() error { calls = append(calls, "stop"); return nil }
	start := func(p profile.Profile) (RuntimeSnapshot, error) {
		calls = append(calls, "start:"+p.ID)
		if p.ID == "new" {
			return RuntimeSnapshot{}, startErr
		}
		return RuntimeSnapshot{}, restoreErr
	}
	save := func(p profile.Profile) error { calls = append(calls, "save:"+p.ID); return nil }
	_, err := replaceOlcSession(profile.Profile{ID: "old"}, profile.Profile{ID: "new"}, stop, start, save, func() RuntimeSnapshot { return RuntimeSnapshot{} })
	if !errors.Is(err, startErr) || !errors.Is(err, restoreErr) || !reflect.DeepEqual(calls, []string{"stop", "start:new", "start:old"}) {
		t.Fatalf("unexpected result: calls=%v err=%v", calls, err)
	}
}
