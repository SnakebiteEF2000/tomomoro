package alert

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive really shows a notification and plays both chimes through the OS
// tools. It is skipped unless TOMOMORO_ALERT_LIVE=1.
func TestLive(t *testing.T) {
	if os.Getenv("TOMOMORO_ALERT_LIVE") != "1" {
		t.Skip("set TOMOMORO_ALERT_LIVE=1 to pop a notification and play both chimes")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // leave the real cache alone
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	if err := Notify(ctx, `tomomoro "live" test`, `quotes ' " and a backslash \ arrive intact`); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	t.Log("Notify: ok")

	for _, k := range kinds {
		start := time.Now()
		if err := Chime(ctx, k); err != nil {
			t.Fatalf("Chime(%s): %v", focusOrBreak(k), err)
		}
		t.Logf("Chime(%s): played for %v", focusOrBreak(k), time.Since(start).Round(time.Millisecond))
	}
}
