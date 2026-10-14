package singbox

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestRuntimeConfigScaling(t *testing.T) {
	for _, count := range []int{10, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			tags := make([]any, 0, count)
			outbounds := make([]any, 0, count+1)
			for n := 0; n < count; n++ {
				tag := fmt.Sprintf("CZ VLESS %d", n)
				tags = append(tags, tag)
				outbounds = append(outbounds, map[string]any{"type": "vless", "tag": tag, "server": "example.invalid", "server_port": 443, "uuid": "00000000-0000-0000-0000-000000000000"})
			}
			outbounds = append([]any{map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags, "default": tags[0]}}, outbounds...)
			body, err := json.Marshal(map[string]any{"outbounds": outbounds, "route": map[string]any{"final": "proxy"}})
			if err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()
			_, api, prepareErr := prepareRuntimeConfig(body)
			err = prepareErr
			if api != nil && len(api.selectionPaths) != count+1 {
				t.Fatalf("selection paths = %d", len(api.selectionPaths))
			}
			elapsed := time.Since(start)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("nodes=%d input_bytes=%d prepare_ms=%.3f allocated_bytes=%d", count, len(body), float64(elapsed.Microseconds())/1000, after.TotalAlloc-before.TotalAlloc)
		})
	}
}
