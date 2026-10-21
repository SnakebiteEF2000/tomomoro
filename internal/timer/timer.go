// Package timer implements the Pomodoro cycle as a wall-clock state machine.
//
// The timer never counts ticks. A running phase stores the instant it ends,
// and every query compares that instant with the injected clock, so the timer
// stays correct across laptop sleep and missed ticks.
package timer

import (
	"fmt"
	"time"
)

// Phase is one segment of the Pomodoro cycle.
type Phase int

const (
	Focus Phase = iota
	ShortBreak
	LongBreak
)

// String returns a lower-case human label: "focus", "short break", "long break".
func (p Phase) String() string {
	switch p {
	case Focus:
		return "focus"
	case ShortBreak:
		return "short break"
	case LongBreak:
		return "long break"
	}
	return fmt.Sprintf("Phase(%d)", int(p))
}

// State is the lifecycle of the current phase.
type State int

const (
	Idle     State = iota // phase loaded at full duration, not started
	Running               // counting down
	Paused                // countdown frozen, remaining time preserved
	Finished              // phase elapsed, waiting for Next
)

// String returns a lower-case label: "idle", "running", "paused", "finished".
func (s State) String() string {
	switch s {
	case Idle:
		return "idle"
	case Running:
		return "running"
	case Paused:
		return "paused"
	case Finished:
		return "finished"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// Durations configures the cycle. A long break follows every LongEvery-th
// focus phase; every other focus phase is followed by a short break.
type Durations struct {
	Focus     time.Duration
	Short     time.Duration
	Long      time.Duration
	LongEvery int
}

// Timer is a Pomodoro cycle. The zero value is not usable; call New.
type Timer struct {
	now       func() time.Time
	d         Durations
	phase     Phase
	state     State
	round     int           // 1..d.LongEvery
	endAt     time.Time     // meaningful while Running
	remaining time.Duration // meaningful while Paused
	startedAt time.Time     // zero while Idle
	completed int
}

// New returns a timer in round 1, phase Focus, state Idle. A nil now uses
// time.Now. Non-positive durations or LongEvery < 1 are clamped to sane
// minimums (1s and 1) so the timer never divides by zero.
func New(d Durations, now func() time.Time) *Timer {
	if now == nil {
		now = time.Now
	}
	return &Timer{
		// Round(0) strips the monotonic reading. The monotonic clock stops
		// during system suspend on macOS and Linux, so comparing monotonic
		// readings would ignore a laptop sleep; wall-clock instants do not.
		now:   func() time.Time { return now().Round(0) },
		d:     clamp(d),
		phase: Focus,
		state: Idle,
		round: 1,
	}
}

// clamp raises non-positive durations to 1s and LongEvery to at least 1.
func clamp(d Durations) Durations {
	minDur := func(v time.Duration) time.Duration {
		if v <= 0 {
			return time.Second
		}
		return v
	}
	return Durations{
		Focus:     minDur(d.Focus),
		Short:     minDur(d.Short),
		Long:      minDur(d.Long),
		LongEvery: max(d.LongEvery, 1),
	}
}

// Start moves Idle to Running and records the phase start time.
// It is a no-op in any other state.
func (t *Timer) Start() {
	if t.state != Idle {
		return
	}
	now := t.now()
	t.startedAt = now
	t.endAt = now.Add(t.Duration())
	t.state = Running
}

// Pause moves Running to Paused, freezing the remaining time.
// It is a no-op in any other state.
func (t *Timer) Pause() {
	if t.state != Running {
		return
	}
	t.remaining = max(t.endAt.Sub(t.now()), 0)
	t.state = Paused
}

// Resume moves Paused to Running. It is a no-op in any other state.
func (t *Timer) Resume() {
	if t.state != Paused {
		return
	}
	t.endAt = t.now().Add(t.remaining)
	t.state = Running
}

// Toggle starts an Idle phase, pauses a Running one, and resumes a Paused one.
// It is a no-op when Finished.
func (t *Timer) Toggle() {
	switch t.state {
	case Idle:
		t.Start()
	case Running:
		t.Pause()
	case Paused:
		t.Resume()
	}
}

// Tick reports true exactly once, on the first call where a Running phase has
// reached its end; the state then becomes Finished. A completed focus phase
// increments CompletedFocus. Tick is cheap and safe to call at any rate.
func (t *Timer) Tick() bool {
	if t.state != Running || t.now().Before(t.endAt) {
		return false
	}
	t.state = Finished
	if t.phase == Focus {
		t.completed++
	}
	return true
}

// Next loads the following phase at full duration in state Idle, advancing
// the cycle. Callers wanting auto-continue call Start afterwards.
// Cycle: Focus → ShortBreak → Focus → … and after the LongEvery-th focus,
// LongBreak; the focus after a LongBreak begins round 1 again.
func (t *Timer) Next() {
	switch t.phase {
	case Focus:
		if t.round >= t.d.LongEvery {
			t.phase = LongBreak
		} else {
			t.phase = ShortBreak
		}
	case ShortBreak:
		t.phase = Focus
		t.round++
	case LongBreak:
		t.phase = Focus
		t.round = 1
	}
	t.Reset()
}

// Skip abandons the current phase (any state) and behaves like Next. A skipped
// focus phase advances the cycle position (so breaks still alternate
// correctly) but does NOT increment CompletedFocus.
func (t *Timer) Skip() { t.Next() }

// Reset reloads the current phase at full duration in state Idle without
// moving the cycle.
func (t *Timer) Reset() {
	t.state = Idle
	t.endAt = time.Time{}
	t.remaining = 0
	t.startedAt = time.Time{}
}

// Phase returns the current phase.
func (t *Timer) Phase() Phase { return t.phase }

// State returns the current state.
func (t *Timer) State() State { return t.state }

// Duration returns the full length of the current phase.
func (t *Timer) Duration() time.Duration {
	switch t.phase {
	case ShortBreak:
		return t.d.Short
	case LongBreak:
		return t.d.Long
	}
	return t.d.Focus
}

// Remaining returns time left in the current phase, never negative.
// Idle returns the full duration; Finished returns 0.
func (t *Timer) Remaining() time.Duration {
	switch t.state {
	case Running:
		return max(t.endAt.Sub(t.now()), 0)
	case Paused:
		return t.remaining
	case Finished:
		return 0
	}
	return t.Duration()
}

// Progress returns the elapsed fraction of the current phase in [0, 1].
func (t *Timer) Progress() float64 {
	p := 1 - float64(t.Remaining())/float64(t.Duration())
	return min(max(p, 0), 1)
}

// Round returns the 1-based position in the cycle (1..LongEvery) of the
// current focus phase, or of the focus phase a break follows.
func (t *Timer) Round() int { return t.round }

// StartedAt returns when the current phase was first started (zero if Idle).
// Pausing does not change it.
func (t *Timer) StartedAt() time.Time { return t.startedAt }

// CompletedFocus returns how many focus phases ran to completion since New.
func (t *Timer) CompletedFocus() int { return t.completed }

// Durations returns the effective (clamped) configuration.
func (t *Timer) Durations() Durations { return t.d }
