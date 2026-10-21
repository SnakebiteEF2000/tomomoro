// Package alert delivers phase-end alerts through OS tools: a desktop
// notification and a short synthesized chime. It never uses cgo.
//
// Supported: macOS (osascript, afplay) and Linux desktops (notify-send with a
// gdbus fallback; pw-play, paplay or aplay, first found). Elsewhere, or when
// no tool is present, calls return an error and the caller ignores it.
//
// The terminal bell is not handled here; the UI emits BEL through its own
// renderer.
package alert

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Kind selects the chime: focus ending and break ending sound different.
type Kind int

const (
	FocusDone Kind = iota // a focus phase ended: time for a break
	BreakDone             // a break ended: back to work
)

// Options toggles each channel for Fire.
type Options struct {
	Notify bool
	Sound  bool
}

// Seams over the OS, swapped by tests so every platform's commands can be
// checked without running anything.
var (
	goos     = runtime.GOOS
	lookPath = exec.LookPath

	// runCmd runs a program to completion without a shell. Its stdio is the
	// null device, so a child can neither draw over the TUI nor read its
	// input, and the context kills it on expiry.
	runCmd = func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}
)

// fireTimeout bounds each channel run by Fire.
var fireTimeout = 5 * time.Second

// Fire runs the enabled channels concurrently, each bounded by a short
// timeout (≈5s) derived from ctx, and returns when all have finished.
// Errors are swallowed: an alert must never break the timer.
func Fire(ctx context.Context, o Options, k Kind, title, body string) {
	var wg sync.WaitGroup
	start := func(alert func(context.Context) error) {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, fireTimeout)
			defer cancel()
			_ = alert(ctx)
		})
	}
	if o.Notify {
		start(func(ctx context.Context) error { return Notify(ctx, title, body) })
	}
	if o.Sound {
		start(func(ctx context.Context) error { return Chime(ctx, k) })
	}
	wg.Wait()
}

// unsupported reports that goos has no alert tools.
func unsupported() error {
	return fmt.Errorf("alert: %s: %w", goos, errors.ErrUnsupported)
}
