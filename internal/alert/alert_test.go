package alert

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"
)

// realRunCmd is the production runner, captured before any test swaps it.
var realRunCmd = runCmd

// call is one recorded program invocation.
type call struct {
	name string
	args []string
}

// recorder is a runCmd fake that records each call and succeeds.
type recorder struct {
	mu    sync.Mutex
	calls []call
}

func (r *recorder) run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call{name, args})
	return nil
}

// names returns the programs run so far, sorted.
func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		out = append(out, c.name)
	}
	slices.Sort(out)
	return out
}

// fakeOS makes the package see system as the OS with only tools on PATH,
// routes every command to run, and points the chime cache at a fresh temp
// dir. Tests that call it must not run in parallel.
func fakeOS(t *testing.T, system string, tools []string, run func(context.Context, string, ...string) error) {
	t.Helper()
	oldGOOS, oldLookPath, oldRunCmd := goos, lookPath, runCmd
	t.Cleanup(func() { goos, lookPath, runCmd = oldGOOS, oldLookPath, oldRunCmd })

	goos = system
	lookPath = func(name string) (string, error) {
		if slices.Contains(tools, name) {
			return "/usr/bin/" + name, nil
		}
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	runCmd = run
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// setFireTimeout sets fireTimeout for the duration of t.
func setFireTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := fireTimeout
	t.Cleanup(func() { fireTimeout = old })
	fireTimeout = d
}

// within fails t unless done closes before limit.
func within(t *testing.T, done <-chan struct{}, limit time.Duration, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("%s did not finish within %v", what, limit)
	}
}

func TestFireToggles(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{"none", Options{}, nil},
		{"notify only", Options{Notify: true}, []string{"osascript"}},
		{"sound only", Options{Sound: true}, []string{"afplay"}},
		{"both", Options{Notify: true, Sound: true}, []string{"afplay", "osascript"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recorder
			fakeOS(t, "darwin", []string{"afplay"}, r.run)

			Fire(t.Context(), tt.opts, FocusDone, "title", "body")

			if got := r.names(); !slices.Equal(got, tt.want) {
				t.Errorf("ran %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFireRunsChannelsConcurrently(t *testing.T) {
	// Each fake command blocks until both have started, so Fire can only
	// return if it runs them at the same time.
	started := make(chan string, 2)
	release := make(chan struct{})
	fakeOS(t, "darwin", []string{"afplay"}, func(ctx context.Context, name string, _ ...string) error {
		started <- name
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	setFireTimeout(t, time.Minute)

	done := make(chan struct{})
	go func() {
		Fire(t.Context(), Options{Notify: true, Sound: true}, BreakDone, "title", "body")
		close(done)
	}()

	var got []string
	for range 2 {
		select {
		case name := <-started:
			got = append(got, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("only %q started; channels did not run concurrently", got)
		}
	}
	close(release)
	within(t, done, 5*time.Second, "Fire")

	slices.Sort(got)
	if want := []string{"afplay", "osascript"}; !slices.Equal(got, want) {
		t.Errorf("started %q, want %q", got, want)
	}
}

func TestFireTimesOutHungCommands(t *testing.T) {
	var (
		mu   sync.Mutex
		errs []error
	)
	fakeOS(t, "darwin", []string{"afplay"}, func(ctx context.Context, _ string, _ ...string) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("command context has no deadline")
		}
		<-ctx.Done() // hang until cut off
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, ctx.Err())
		return ctx.Err()
	})
	setFireTimeout(t, 50*time.Millisecond)

	done := make(chan struct{})
	go func() {
		Fire(t.Context(), Options{Notify: true, Sound: true}, FocusDone, "title", "body")
		close(done)
	}()
	within(t, done, 5*time.Second, "Fire")

	if len(errs) != 2 {
		t.Fatalf("%d commands were cut off, want 2", len(errs))
	}
	for _, err := range errs {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("command ended with %v, want %v", err, context.DeadlineExceeded)
		}
	}
}

func TestRunCmdKillsOnTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not on PATH")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := realRunCmd(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("sleep 10 succeeded under a 50ms timeout")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("runCmd returned after %v; the child was not killed", elapsed)
	}
}
