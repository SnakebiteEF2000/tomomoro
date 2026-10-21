package timer

import (
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic tests.
type fakeClock struct{ t time.Time }

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

var std = Durations{
	Focus:     25 * time.Minute,
	Short:     5 * time.Minute,
	Long:      15 * time.Minute,
	LongEvery: 4,
}

// complete runs the current phase to its natural end.
func complete(t *testing.T, tm *Timer, clk *fakeClock) {
	t.Helper()
	tm.Start()
	clk.advance(tm.Duration())
	if !tm.Tick() {
		t.Fatalf("Tick() = false at end of %v", tm.Phase())
	}
}

type step struct {
	phase Phase
	round int
}

func TestCycleOrder(t *testing.T) {
	tests := []struct {
		name      string
		longEvery int
		want      []step
	}{
		{
			name:      "long every 4",
			longEvery: 4,
			want: []step{
				{Focus, 1}, {ShortBreak, 1},
				{Focus, 2}, {ShortBreak, 2},
				{Focus, 3}, {ShortBreak, 3},
				{Focus, 4}, {LongBreak, 4},
				{Focus, 1}, {ShortBreak, 1},
			},
		},
		{
			name:      "long every 1",
			longEvery: 1,
			want: []step{
				{Focus, 1}, {LongBreak, 1},
				{Focus, 1}, {LongBreak, 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock()
			d := std
			d.LongEvery = tt.longEvery
			tm := New(d, clk.now)
			focusDone := 0
			for i, want := range tt.want {
				if got := (step{tm.Phase(), tm.Round()}); got != want {
					t.Fatalf("step %d: got %v round %d, want %v round %d",
						i, got.phase, got.round, want.phase, want.round)
				}
				if tm.State() != Idle {
					t.Fatalf("step %d: state = %v, want idle", i, tm.State())
				}
				complete(t, tm, clk)
				if tm.Phase() == Focus {
					focusDone++
				}
				if got := tm.CompletedFocus(); got != focusDone {
					t.Fatalf("step %d: CompletedFocus() = %d, want %d", i, got, focusDone)
				}
				tm.Next()
			}
		})
	}
}

func TestSkipAdvancesCycleWithoutCompleting(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)

	tm.Start()
	clk.advance(10 * time.Minute)
	tm.Skip()
	if tm.Phase() != ShortBreak || tm.Round() != 1 || tm.State() != Idle {
		t.Fatalf("after skip: %v round %d %v, want short break round 1 idle",
			tm.Phase(), tm.Round(), tm.State())
	}
	if tm.CompletedFocus() != 0 {
		t.Fatalf("CompletedFocus() = %d after skip, want 0", tm.CompletedFocus())
	}
	if tm.Remaining() != std.Short {
		t.Fatalf("Remaining() = %v after skip, want %v", tm.Remaining(), std.Short)
	}

	// Skipping an Idle break still advances.
	tm.Skip()
	if tm.Phase() != Focus || tm.Round() != 2 {
		t.Fatalf("after skipping break: %v round %d, want focus round 2", tm.Phase(), tm.Round())
	}

	complete(t, tm, clk)
	if tm.CompletedFocus() != 1 {
		t.Fatalf("CompletedFocus() = %d after natural completion, want 1", tm.CompletedFocus())
	}
	// Skipping a Finished focus neither double-counts nor loses the completion.
	tm.Skip()
	if tm.CompletedFocus() != 1 || tm.Phase() != ShortBreak {
		t.Fatalf("after skipping finished focus: completed %d phase %v", tm.CompletedFocus(), tm.Phase())
	}
}

func TestBreakCompletionDoesNotCountAsFocus(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)
	tm.Skip()
	complete(t, tm, clk)
	if tm.CompletedFocus() != 0 {
		t.Fatalf("CompletedFocus() = %d after a break, want 0", tm.CompletedFocus())
	}
}

func TestPauseResumePreservesRemaining(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)

	tm.Start()
	clk.advance(10 * time.Minute)
	tm.Pause()
	want := 15 * time.Minute
	if tm.State() != Paused || tm.Remaining() != want {
		t.Fatalf("after pause: %v %v, want paused %v", tm.State(), tm.Remaining(), want)
	}

	clk.advance(time.Hour) // paused time must not count
	if tm.Remaining() != want {
		t.Fatalf("Remaining() = %v while paused an hour, want %v", tm.Remaining(), want)
	}
	if tm.Tick() {
		t.Fatal("Tick() = true while paused")
	}

	tm.Resume()
	if tm.State() != Running || tm.Remaining() != want {
		t.Fatalf("after resume: %v %v, want running %v", tm.State(), tm.Remaining(), want)
	}

	clk.advance(5 * time.Minute)
	if got := tm.Remaining(); got != 10*time.Minute {
		t.Fatalf("Remaining() = %v 5m after resume, want 10m", got)
	}
	if got, want := tm.Progress(), 0.6; got != want {
		t.Fatalf("Progress() = %v, want %v", got, want)
	}

	clk.advance(10*time.Minute - time.Nanosecond)
	if tm.Tick() {
		t.Fatal("Tick() = true 1ns before the end")
	}
	clk.advance(time.Nanosecond)
	if !tm.Tick() {
		t.Fatal("Tick() = false at the end")
	}
}

func TestSleepJump(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)

	tm.Start()
	clk.advance(8 * time.Hour) // laptop slept through the phase; no ticks ran

	if got := tm.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %v before Tick, want 0", got)
	}
	if !tm.Tick() {
		t.Fatal("first Tick() after sleep = false, want true")
	}
	if tm.State() != Finished {
		t.Fatalf("State() = %v, want finished", tm.State())
	}
	if tm.Remaining() != 0 || tm.Progress() != 1 {
		t.Fatalf("Remaining() = %v Progress() = %v, want 0 and 1", tm.Remaining(), tm.Progress())
	}
	if tm.CompletedFocus() != 1 {
		t.Fatalf("CompletedFocus() = %d, want 1", tm.CompletedFocus())
	}
	for i := range 3 {
		clk.advance(time.Minute)
		if tm.Tick() {
			t.Fatalf("Tick() #%d after finish = true, want false", i+2)
		}
	}
	if tm.CompletedFocus() != 1 {
		t.Fatalf("CompletedFocus() = %d after extra ticks, want 1", tm.CompletedFocus())
	}
}

func TestReset(t *testing.T) {
	tests := []struct {
		name  string
		setup func(tm *Timer, clk *fakeClock)
	}{
		{"idle", func(tm *Timer, clk *fakeClock) {}},
		{"running", func(tm *Timer, clk *fakeClock) {
			tm.Start()
			clk.advance(2 * time.Minute)
		}},
		{"paused", func(tm *Timer, clk *fakeClock) {
			tm.Start()
			clk.advance(2 * time.Minute)
			tm.Pause()
		}},
		{"finished", func(tm *Timer, clk *fakeClock) {
			tm.Start()
			clk.advance(time.Hour)
			tm.Tick()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock()
			tm := New(std, clk.now)
			complete(t, tm, clk)
			tm.Next() // short break, round 1
			tm.Skip() // focus, round 2
			tt.setup(tm, clk)
			completed := tm.CompletedFocus()

			tm.Reset()

			if tm.Phase() != Focus || tm.Round() != 2 {
				t.Fatalf("after reset: %v round %d, want focus round 2", tm.Phase(), tm.Round())
			}
			if tm.State() != Idle {
				t.Fatalf("State() = %v, want idle", tm.State())
			}
			if tm.Remaining() != std.Focus || tm.Progress() != 0 {
				t.Fatalf("Remaining() = %v Progress() = %v, want %v and 0",
					tm.Remaining(), tm.Progress(), std.Focus)
			}
			if !tm.StartedAt().IsZero() {
				t.Fatalf("StartedAt() = %v, want zero", tm.StartedAt())
			}
			if tm.CompletedFocus() != completed {
				t.Fatalf("CompletedFocus() = %d, want %d", tm.CompletedFocus(), completed)
			}
			clk.advance(time.Hour)
			if tm.Tick() || tm.Remaining() != std.Focus {
				t.Fatal("idle timer counted down after reset")
			}
		})
	}
}

func TestToggle(t *testing.T) {
	tests := []struct {
		name  string
		setup func(tm *Timer, clk *fakeClock)
		want  State
	}{
		{"idle starts", func(tm *Timer, clk *fakeClock) {}, Running},
		{"running pauses", func(tm *Timer, clk *fakeClock) { tm.Start() }, Paused},
		{"paused resumes", func(tm *Timer, clk *fakeClock) {
			tm.Start()
			tm.Pause()
		}, Running},
		{"finished stays", func(tm *Timer, clk *fakeClock) {
			tm.Start()
			clk.advance(time.Hour)
			tm.Tick()
		}, Finished},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock()
			tm := New(std, clk.now)
			tt.setup(tm, clk)
			tm.Toggle()
			if tm.State() != tt.want {
				t.Fatalf("State() = %v, want %v", tm.State(), tt.want)
			}
		})
	}
}

func TestStartIsNoOpUnlessIdle(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)
	start := clk.now()
	tm.Start()

	clk.advance(5 * time.Minute)
	tm.Start() // running: must not restart the phase
	if got := tm.Remaining(); got != 20*time.Minute {
		t.Fatalf("Remaining() = %v after Start while running, want 20m", got)
	}

	tm.Pause()
	tm.Start() // paused: must not resume or restart
	if tm.State() != Paused {
		t.Fatalf("State() = %v after Start while paused, want paused", tm.State())
	}

	tm.Resume()
	clk.advance(time.Hour)
	tm.Tick()
	tm.Start() // finished: must not restart
	if tm.State() != Finished || !tm.StartedAt().Equal(start) {
		t.Fatalf("after Start while finished: %v started %v", tm.State(), tm.StartedAt())
	}
}

func TestPauseResumeNoOpOutsideTheirStates(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)
	tm.Pause()
	tm.Resume()
	if tm.State() != Idle {
		t.Fatalf("Pause/Resume on idle changed state to %v", tm.State())
	}
	tm.Start()
	tm.Resume()
	if tm.State() != Running {
		t.Fatalf("Resume on running changed state to %v", tm.State())
	}
}

func TestStartedAt(t *testing.T) {
	clk := newClock()
	tm := New(std, clk.now)
	if !tm.StartedAt().IsZero() {
		t.Fatalf("StartedAt() = %v while idle, want zero", tm.StartedAt())
	}
	start := clk.now()
	tm.Start()
	clk.advance(3 * time.Minute)
	tm.Pause()
	clk.advance(time.Hour)
	tm.Resume()
	if !tm.StartedAt().Equal(start) {
		t.Fatalf("StartedAt() = %v after pause/resume, want %v", tm.StartedAt(), start)
	}
	clk.advance(time.Hour)
	tm.Tick()
	if !tm.StartedAt().Equal(start) {
		t.Fatalf("StartedAt() = %v when finished, want %v", tm.StartedAt(), start)
	}
	tm.Next()
	if !tm.StartedAt().IsZero() {
		t.Fatalf("StartedAt() = %v after Next, want zero", tm.StartedAt())
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		in   Durations
		want Durations
	}{
		{
			name: "zero",
			in:   Durations{},
			want: Durations{Focus: time.Second, Short: time.Second, Long: time.Second, LongEvery: 1},
		},
		{
			name: "negative",
			in:   Durations{Focus: -time.Minute, Short: -1, Long: -time.Hour, LongEvery: -3},
			want: Durations{Focus: time.Second, Short: time.Second, Long: time.Second, LongEvery: 1},
		},
		{
			name: "valid untouched",
			in:   std,
			want: std,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock()
			tm := New(tt.in, clk.now)
			if got := tm.Durations(); got != tt.want {
				t.Fatalf("Durations() = %+v, want %+v", got, tt.want)
			}
			if got := tm.Progress(); got != 0 {
				t.Fatalf("Progress() = %v while idle, want 0", got)
			}
			if tm.Duration() != tt.want.Focus || tm.Remaining() != tt.want.Focus {
				t.Fatalf("Duration() = %v Remaining() = %v, want %v",
					tm.Duration(), tm.Remaining(), tt.want.Focus)
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	tm := New(std, nil) // nil clock falls back to time.Now
	if tm.Phase() != Focus || tm.State() != Idle || tm.Round() != 1 || tm.CompletedFocus() != 0 {
		t.Fatalf("New: %v %v round %d completed %d", tm.Phase(), tm.State(), tm.Round(), tm.CompletedFocus())
	}
	tm.Start()
	if got := tm.Remaining(); got <= 0 || got > std.Focus {
		t.Fatalf("Remaining() = %v with real clock, want (0, %v]", got, std.Focus)
	}
}

func TestDurationPerPhase(t *testing.T) {
	clk := newClock()
	tm := New(Durations{Focus: time.Minute, Short: 2 * time.Minute, Long: 3 * time.Minute, LongEvery: 2}, clk.now)
	want := []time.Duration{time.Minute, 2 * time.Minute, time.Minute, 3 * time.Minute}
	for i, w := range want {
		if got := tm.Duration(); got != w {
			t.Fatalf("step %d (%v): Duration() = %v, want %v", i, tm.Phase(), got, w)
		}
		tm.Next()
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{Focus.String(), "focus"},
		{ShortBreak.String(), "short break"},
		{LongBreak.String(), "long break"},
		{Phase(9).String(), "Phase(9)"},
		{Idle.String(), "idle"},
		{Running.String(), "running"},
		{Paused.String(), "paused"},
		{Finished.String(), "finished"},
		{State(-1).String(), "State(-1)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("String() = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
