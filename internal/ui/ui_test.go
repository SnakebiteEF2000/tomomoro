package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/SnakebiteEF2000/tomomoro/internal/alert"
	"github.com/SnakebiteEF2000/tomomoro/internal/config"
	"github.com/SnakebiteEF2000/tomomoro/internal/history"
	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
	"github.com/SnakebiteEF2000/tomomoro/internal/timer"
)

// clock is a settable fake time source.
type clock struct{ t time.Time }

func (c *clock) now() time.Time         { return c.t }
func (c *clock) add(d time.Duration)    { c.t = c.t.Add(d) }
func (c *clock) set(t time.Time) *clock { c.t = t; return c }

// fired records one alert.Fire call.
type fired struct {
	opts        alert.Options
	kind        alert.Kind
	title, body string
}

// harness is a model wired to a fake clock, a temporary history file and an
// alert recorder, sized 80x24.
type harness struct {
	t     *testing.T
	m     *model
	clk   *clock
	path  string
	fired []fired
}

// newHarness builds a harness; edit adjusts the default config.
func newHarness(t *testing.T, edit func(*config.Config)) *harness {
	t.Helper()
	cfg := config.Default()
	cfg.Alerts = config.Alerts{}
	if edit != nil {
		edit(&cfg)
	}
	h := &harness{
		t:    t,
		clk:  new(clock).set(time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)),
		path: filepath.Join(t.TempDir(), "history.jsonl"),
	}
	m, err := newModel(Options{Config: cfg, Task: "writing report", Log: history.Open(h.path), Now: h.clk.now})
	if err != nil {
		t.Fatal(err)
	}
	m.fire = func(_ context.Context, o alert.Options, k alert.Kind, title, body string) {
		h.fired = append(h.fired, fired{o, k, title, body})
	}
	h.m = m
	h.size(80, 24)
	return h
}

func (h *harness) size(w, ht int) { h.m.Update(tea.WindowSizeMsg{Width: w, Height: ht}) }

// tick runs the tick handler's work and the commands it returns. Update
// re-arms the tick loop on a real timer, so tests call advance directly;
// TestTickRearms covers the loop itself.
func (h *harness) tick() []tea.Msg { return run(h.m.advance()) }

// press sends each key and returns the messages its commands produced.
func (h *harness) press(keys ...string) []tea.Msg {
	var msgs []tea.Msg
	for _, k := range keys {
		_, cmd := h.m.Update(keyMsg(k))
		msgs = append(msgs, run(cmd)...)
	}
	return msgs
}

// run executes cmd inline, expanding batches, and returns the non-nil
// messages.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, run(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// hasClock reports whether v shows s in big digits.
func hasClock(v, s string) bool {
	for _, row := range theme.BigDigits(s) {
		if !strings.Contains(v, row) {
			return false
		}
	}
	return true
}

// view is the ANSI-stripped screen.
func (h *harness) view() string { return ansi.Strip(h.m.View().Content) }

// lines reads the history file's lines.
func (h *harness) lines() []string {
	h.t.Helper()
	b, err := os.ReadFile(h.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// keyMsg builds the key press a terminal would send for k.
func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// quits reports whether msgs contain a quit.
func quits(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func TestKeyMsgStrings(t *testing.T) {
	for _, k := range []string{"space", "tab", "esc", "enter", "ctrl+c", "T", "?", "q"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

func TestSpaceStartsPausesResumes(t *testing.T) {
	h := newHarness(t, nil)
	steps := []struct {
		advance time.Duration
		want    timer.State
		header  string
		hint    string
		clock   string
	}{
		{0, timer.Running, "● FOCUS · session 1 of 4", "space pause", "25:00"},
		{time.Minute, timer.Paused, "● PAUSED · FOCUS · session 1 of 4", "space resume", "24:00"},
		{time.Hour, timer.Running, "● FOCUS · session 1 of 4", "space pause", "24:00"}, // paused time does not count
	}
	for _, s := range steps {
		h.clk.add(s.advance)
		h.press("space")
		if got := h.m.timer.State(); got != s.want {
			t.Fatalf("after space: state %v, want %v", got, s.want)
		}
		v := h.view()
		if !strings.Contains(v, s.header) || !strings.Contains(v, s.hint) || !hasClock(v, s.clock) {
			t.Errorf("state %v: want %q, %q and %s in:\n%s", s.want, s.header, s.hint, s.clock, v)
		}
	}
}

func TestSkipNeverLogsOrAlerts(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Alerts = config.Alerts{Notify: true, Sound: true, Bell: true} })
	h.press("space")
	h.clk.add(10 * time.Minute)
	msgs := h.press("s")
	if got := h.m.timer.Phase(); got != timer.ShortBreak {
		t.Fatalf("phase after skip = %v, want short break", got)
	}
	if got := h.m.timer.State(); got != timer.Idle {
		t.Errorf("state after skip = %v, want idle", got)
	}
	if len(h.lines()) != 0 || len(h.fired) != 0 || len(msgs) != 0 {
		t.Errorf("skip logged %d lines, fired %d alerts, produced %v", len(h.lines()), len(h.fired), msgs)
	}
	if v := h.view(); !strings.Contains(v, "● SHORT BREAK · after session 1") || strings.Contains(v, "done") {
		t.Errorf("view after skip:\n%s", v)
	}
}

func TestResetReloadsPhase(t *testing.T) {
	h := newHarness(t, nil)
	h.press("space")
	h.clk.add(3 * time.Minute)
	h.press("r")
	if h.m.timer.State() != timer.Idle || h.m.timer.Remaining() != 25*time.Minute {
		t.Errorf("after reset: %v with %v left", h.m.timer.State(), h.m.timer.Remaining())
	}
	if v := h.view(); !strings.Contains(v, "space start") {
		t.Errorf("idle view lacks start hint:\n%s", v)
	}
}

func TestAutoToggleToast(t *testing.T) {
	h := newHarness(t, nil)
	h.press("a")
	if !h.m.auto || !strings.Contains(h.view(), "auto-continue on") {
		t.Fatalf("auto=%v, view:\n%s", h.m.auto, h.view())
	}
	h.clk.add(toastFor)
	h.tick()
	if strings.Contains(h.view(), "auto-continue") {
		t.Errorf("toast still shown after %v", toastFor)
	}
	h.press("a")
	if h.m.auto || !strings.Contains(h.view(), "auto-continue off") {
		t.Errorf("second toggle: auto=%v, view:\n%s", h.m.auto, h.view())
	}
}

func TestThemeCycle(t *testing.T) {
	h := newHarness(t, nil)
	for _, want := range append(theme.Names()[1:], theme.Names()[0]) {
		h.press("T")
		if h.m.pal.Name != want {
			t.Fatalf("theme = %q, want %q", h.m.pal.Name, want)
		}
		if v := h.view(); !strings.Contains(v, "theme "+want) || !strings.Contains(v, "-theme") {
			t.Errorf("toast for %s missing:\n%s", want, v)
		}
	}
}

func TestStatsScreen(t *testing.T) {
	h := newHarness(t, nil)
	h.press("tab")
	if h.m.screen != screenStats {
		t.Fatal("tab did not open stats")
	}
	v := h.view()
	for _, want := range []string{"● STATS", "today     0 sessions · 0m", "all time  0 sessions · 0m", "tab back"} {
		if !strings.Contains(v, want) {
			t.Errorf("stats view lacks %q:\n%s", want, v)
		}
	}
	h.press("space") // ignored on the stats screen
	if h.m.timer.State() != timer.Idle {
		t.Error("space acted on the timer from the stats screen")
	}
	for _, back := range []string{"tab", "esc"} {
		h.m.screen = screenStats
		h.press(back)
		if h.m.screen != screenTimer {
			t.Errorf("%s did not leave stats", back)
		}
	}
}

func TestHelpScreen(t *testing.T) {
	h := newHarness(t, nil)
	h.press("?")
	v := h.view()
	for _, want := range []string{"● KEYS", "space  start · pause · resume", "T      next color theme (rose-pine)", "any key to close"} {
		if !strings.Contains(v, want) {
			t.Errorf("help lacks %q:\n%s", want, v)
		}
	}
	h.press("x")
	if h.m.screen != screenTimer {
		t.Error("a key did not close help")
	}
	h.press("?")
	if !quits(h.press("q")) {
		t.Error("q did not quit from help")
	}
}

func TestTaskEdit(t *testing.T) {
	h := newHarness(t, nil)
	h.press("t")
	if !h.m.editing {
		t.Fatal("t did not start editing")
	}
	if v := h.view(); !strings.Contains(v, "enter save · esc cancel") {
		t.Errorf("editing view lacks hints:\n%s", v)
	}
	// Every key goes to the input, including ones bound outside it.
	msgs := h.press("backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "q", "s", " ", "T", "?")
	if quits(msgs) || h.m.timer.State() != timer.Idle || h.m.screen != screenTimer || h.m.pal.Name != theme.Default {
		t.Fatal("a key leaked out of the task input")
	}
	h.press("enter")
	if h.m.editing || h.m.task != "writing qs T?" {
		t.Fatalf("after enter: editing=%v task=%q", h.m.editing, h.m.task)
	}
	if v := h.view(); !strings.Contains(v, "○   ·  writing qs T?") {
		t.Errorf("task not shown:\n%s", v)
	}

	h.press("t", "x", "esc")
	if h.m.editing || h.m.task != "writing qs T?" {
		t.Errorf("esc: editing=%v task=%q", h.m.editing, h.m.task)
	}

	h.press("t")
	h.m.input.SetValue("   ")
	h.press("enter")
	if h.m.task != "" || strings.Contains(h.view(), "·  ") {
		t.Errorf("blank task kept: %q\n%s", h.m.task, h.view())
	}

	h.press("t")
	if !quits(h.press("ctrl+c")) {
		t.Error("ctrl+c did not quit while editing")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		h := newHarness(t, nil)
		if !quits(h.press(k)) {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestFocusEndAppendsOneEntry(t *testing.T) {
	h := newHarness(t, nil)
	start := h.clk.now()
	h.press("space")
	h.clk.add(25 * time.Minute)
	h.tick()
	h.tick()

	lines := h.lines()
	if len(lines) != 1 {
		t.Fatalf("history has %d lines, want 1: %q", len(lines), lines)
	}
	entries, err := history.Open(h.path).All()
	if err != nil || len(entries) != 1 {
		t.Fatalf("All() = %v, %v", entries, err)
	}
	e := entries[0]
	if !e.Start.Equal(start) || !e.End.Equal(start.Add(25*time.Minute)) || e.Kind != history.KindFocus ||
		e.Duration != 25*time.Minute || e.Task != "writing report" {
		t.Errorf("entry = %+v", e)
	}
	if h.m.timer.Phase() != timer.ShortBreak || h.m.timer.State() != timer.Idle {
		t.Errorf("next: %v %v, want short break idle", h.m.timer.Phase(), h.m.timer.State())
	}
	v := h.view()
	for _, want := range []string{"✓ focus done · space to start short break", "space start"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if !hasClock(v, "05:00") {
		t.Errorf("view does not show the 05:00 break:\n%s", v)
	}

	// The break ends without a history entry and leads to session 2.
	h.press("space")
	if strings.Contains(h.view(), "done") {
		t.Error("prompt stays after starting the break")
	}
	h.clk.add(5 * time.Minute)
	h.tick()
	if n := len(h.lines()); n != 1 {
		t.Errorf("a break was logged: %d lines", n)
	}
	if v := h.view(); !strings.Contains(v, "✓ short break done · space to start focus") {
		t.Errorf("view after break:\n%s", v)
	}
	h.press("space")
	if v := h.view(); !strings.Contains(v, "● FOCUS · session 2 of 4") {
		t.Errorf("view in session 2:\n%s", v)
	}
}

func TestPhaseEndAlerts(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Alerts = config.Alerts{Notify: true, Sound: false, Bell: true} })
	h.press("space")
	h.clk.add(25 * time.Minute)
	msgs := h.tick()

	if len(h.fired) != 1 {
		t.Fatalf("fired %d alerts, want 1", len(h.fired))
	}
	want := fired{alert.Options{Notify: true}, alert.FocusDone, "Focus complete 🍅", "Done: writing report · Short break · 5m"}
	if h.fired[0] != want {
		t.Errorf("fired %+v, want %+v", h.fired[0], want)
	}
	if len(msgs) != 1 || msgs[0] != (tea.RawMsg{Msg: "\a"}) {
		t.Errorf("messages = %#v, want one bell", msgs)
	}

	h.press("space")
	h.clk.add(5 * time.Minute)
	h.tick()
	if len(h.fired) != 2 || h.fired[1].kind != alert.BreakDone || h.fired[1].title != "Break over" {
		t.Errorf("break alert = %+v", h.fired)
	}
}

func TestNoAlertsWhenDisabled(t *testing.T) {
	h := newHarness(t, nil)
	h.press("space")
	h.clk.add(25 * time.Minute)
	if msgs := h.tick(); len(msgs) != 0 || len(h.fired) != 0 {
		t.Errorf("alerts with all channels off: %v %v", msgs, h.fired)
	}
}

func TestNotice(t *testing.T) {
	tests := []struct {
		done, next  timer.Phase
		dur         time.Duration
		task        string
		title, body string
	}{
		{timer.Focus, timer.ShortBreak, 5 * time.Minute, "", "Focus complete 🍅", "Short break · 5m"},
		{timer.Focus, timer.LongBreak, 15 * time.Minute, "report", "Focus complete 🍅", "Done: report · Long break · 15m"},
		{timer.ShortBreak, timer.Focus, 25 * time.Minute, "", "Break over", "Back to focus · 25m"},
		{timer.LongBreak, timer.Focus, 90 * time.Minute, "report", "Break over", "Back to focus · 1h30m · report"},
	}
	for _, tt := range tests {
		title, body := notice(tt.done, tt.next, tt.dur, tt.task)
		if title != tt.title || body != tt.body {
			t.Errorf("notice(%v, %v) = %q, %q; want %q, %q", tt.done, tt.next, title, body, tt.title, tt.body)
		}
	}
}

func TestAutoContinueStartsNextPhase(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.AutoContinue = true })
	h.press("space")
	h.clk.add(25 * time.Minute)
	h.tick()
	if h.m.timer.Phase() != timer.ShortBreak || h.m.timer.State() != timer.Running || h.m.waiting {
		t.Errorf("auto-continue: %v %v waiting=%v", h.m.timer.Phase(), h.m.timer.State(), h.m.waiting)
	}
	if v := h.view(); !strings.Contains(v, "● SHORT BREAK · after session 1") {
		t.Errorf("view:\n%s", v)
	}
}

func TestLongBreakResetsDots(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.LongBreakEvery = 2 })
	wantDots := []string{"● ○", "● ○", "● ●", "● ●", "● ○"} // F1 S1 F2 L F1
	wantHeader := []string{"FOCUS · session 1 of 2", "SHORT BREAK · after session 1", "FOCUS · session 2 of 2", "LONG BREAK", "FOCUS · session 1 of 2"}
	for i := range wantDots {
		v := h.view()
		if !strings.Contains(v, wantDots[i]+"   ·  writing report") || !strings.Contains(v, wantHeader[i]) {
			t.Errorf("step %d: want %q and %q in:\n%s", i, wantDots[i], wantHeader[i], v)
		}
		h.press("space")
		h.clk.add(h.m.timer.Duration())
		h.tick()
		h.m.waiting = false // show the phase header rather than the prompt
	}
	if n := len(h.lines()); n != 3 {
		t.Errorf("logged %d focus phases, want 3", n)
	}
}

func TestKeyAfterExpiryFinishesPhaseFirst(t *testing.T) {
	h := newHarness(t, nil)
	h.press("space")
	h.clk.add(26 * time.Minute) // no tick arrived in time
	h.press("space")
	if h.m.timer.Phase() != timer.ShortBreak || h.m.timer.State() != timer.Idle {
		t.Errorf("after late key: %v %v, want short break idle", h.m.timer.Phase(), h.m.timer.State())
	}
	if n := len(h.lines()); n != 1 {
		t.Errorf("history lines = %d, want 1", n)
	}
}

func TestHistoryErrorShownInStatusLine(t *testing.T) {
	h := newHarness(t, nil)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.m.log = history.Open(filepath.Join(blocker, "history.jsonl")) // parent is a file
	h.press("space")
	h.clk.add(25 * time.Minute)
	h.tick()
	if h.m.timer.Phase() != timer.ShortBreak {
		t.Fatal("a failed history write stopped the timer")
	}
	rows := strings.Split(h.view(), "\n")
	if len(rows) != 24 || !strings.Contains(rows[23], "✗ history:") {
		t.Errorf("status row = %q (of %d rows)", rows[len(rows)-1], len(rows))
	}
}

func TestUnknownTheme(t *testing.T) {
	cfg := config.Default()
	cfg.Theme = "nope"
	if _, err := newModel(Options{Config: cfg}); err == nil || !strings.Contains(err.Error(), "rose-pine") {
		t.Errorf("err = %v", err)
	}
}

func TestBackgroundFollowsColorProfile(t *testing.T) {
	h := newHarness(t, nil)
	if h.m.View().BackgroundColor != nil {
		t.Error("background set before the color profile is known")
	}
	// colorprofile.TrueColor is 5 and colorprofile.ASCII is 2.
	h.m.Update(tea.ColorProfileMsg{Profile: 5})
	if h.m.View().BackgroundColor != h.m.pal.Base {
		t.Error("background not set on a color terminal")
	}
	h.m.Update(tea.ColorProfileMsg{Profile: 2})
	if h.m.View().BackgroundColor != nil {
		t.Error("background set without color support")
	}
}

func TestStatsRefreshWhenFocusEnds(t *testing.T) {
	h := newHarness(t, nil)
	h.press("space", "tab")
	h.clk.add(25 * time.Minute)
	h.tick()
	if v := h.view(); !strings.Contains(v, "today     1 session · 25m") || !strings.Contains(v, "🔥 1 day") {
		t.Errorf("stats screen not refreshed:\n%s", v)
	}
}
