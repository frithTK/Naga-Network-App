package subscription

import (
	"net/http"
	"testing"
	"time"
)

func TestUpdateIntervalUsesHoursAndPreservesMissingMetadata(t *testing.T) {
	for _, test := range []struct {
		raw           string
		initial, want time.Duration
		invalid       bool
	}{
		{"1", 0, time.Hour, false}, {"24", 0, 24 * time.Hour, false}, {"8760", 0, 365 * 24 * time.Hour, false},
		{"", 48 * time.Hour, 48 * time.Hour, false}, {"", 0, 24 * time.Hour, false}, {"0", 0, 0, true}, {"8761", 0, 0, true},
	} {
		t.Run(test.raw, func(t *testing.T) {
			s := Snapshot{UpdateInterval: test.initial}
			err := ApplySubscriptionHeaders(&s, http.Header{"Profile-Update-Interval": []string{test.raw}})
			if test.invalid {
				if err == nil {
					t.Fatal("accepted invalid interval")
				}
				return
			}
			if err != nil || s.UpdateInterval != test.want {
				t.Fatalf("interval=%s err=%v", s.UpdateInterval, err)
			}
		})
	}
}
