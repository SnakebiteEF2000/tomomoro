package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/SnakebiteEF2000/tomomoro/internal/config"
	"github.com/SnakebiteEF2000/tomomoro/internal/history"
	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
)

func TestFormatClock(t *testing.T) {
	tests := []struct {
		remaining, phase time.Duration
		want             string
	}{
		{25 * time.Minute, 25 * time.Minute, "25:00"},
		{25*time.Minute - time.Millisecond, 25 * time.Minute, "25:00"},
		{24*time.Minute + 59*time.Second + time.Nanosecond, 25 * time.Minute, "25:00"},
		{24*time.Minute + 59*time.Second, 25 * time.Minute, "24:59"},
		{200 * time.Millisecond, 25 * time.Minute, "00:01"},
		{0, 25 * time.Minute, "00:00"},
		{-time.Second, 25 * time.Minute, "00:00"},
		{90 * time.Minute, 90 * time.Minute, "1:30:00"},
		{time.Hour, time.Hour, "1:00:00"},
		{59*time.Minute + 59*time.Second, time.Hour, "0:59:59"},
		{10 * time.Second, 2 * time.Hour, "0:00:10"},
		{12*time.Hour + 5*time.Second, 12 * time.Hour, "12:00:05"},
	}
	for _, tt := range tests {
		if got := formatClock(tt.remaining, tt.phase); got != tt.want {
			t.Errorf("formatClock(%v, %v) = %q, want %q", tt.remaining, tt.phase, got, tt.want)
		}
	}
}

func TestFreshPhaseShowsFullDuration(t *testing.T) {
	h := newHarness(t, nil)
	h.press("space")
	h.clk.add(300 * time.Millisecond)
	if v := h.view(); !hasClock(v, "25:00") {
		t.Errorf("a phase 300ms in does not show 25:00:\n%s", v)
	}
}

func TestFullView(t *testing.T) {
	h := newHarness(t, nil)
	v := h.view()
	for _, want := range []string{
		"● FOCUS · session 1 of 4",
		"0%",
		"● ○ ○ ○   ·  writing report",
		"space start · s skip · t task · ? help · q quit",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if !hasClock(v, "25:00") {
		t.Errorf("view lacks 25:00 in big digits:\n%s", v)
	}
}

// TestFullLayoutGeometry checks the layout against the approved preview at
// several sizes: the digits are centered, the bar starts at the digits' left
// edge and is exactly as wide, and the header and hints are centered.
func TestFullLayoutGeometry(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {46, 14}} {
		h := newHarness(t, nil)
		h.size(size[0], size[1])
		h.press("space")
		h.clk.add(10*time.Minute + 30*time.Second)
		rows := strings.Split(h.view(), "\n")
		if len(rows) > size[1] {
			t.Fatalf("%v: %d rows", size, len(rows))
		}

		digits := theme.BigDigits("14:30")
		dw := ansi.StringWidth(digits[0])
		top := -1
		for i, r := range rows {
			if strings.HasSuffix(r, digits[0]) {
				top = i
				break
			}
		}
		if top < 2 {
			t.Fatalf("%v: digits not found:\n%s", size, h.view())
		}
		left := (size[0] - dw) / 2
		for i, d := range digits {
			if rows[top+i] != strings.Repeat(" ", left)+d {
				t.Errorf("%v: digit row %d = %q", size, i, rows[top+i])
			}
		}
		bar := rows[top+len(digits)+1]
		want := strings.Repeat(" ", left) + strings.Repeat("━", 13) + strings.Repeat("─", dw-13) + "  42%"
		if bar != want {
			t.Errorf("%v: bar row\n got %q\nwant %q", size, bar, want)
		}
		for _, i := range []int{top - 2, top + len(digits) + 3, top + len(digits) + 5} {
			r := rows[i]
			text := strings.TrimLeft(r, " ")
			pad := len(r) - len(text)
			if want := (size[0] - ansi.StringWidth(text)) / 2; pad != want {
				t.Errorf("%v: row %q is indented %d, want %d", size, r, pad, want)
			}
		}
	}
}

func TestCompactBelowThreshold(t *testing.T) {
	tests := []struct {
		w, h    int
		compact bool
	}{
		{80, 24, false},
		{46, 14, false},
		{42, 14, false},
		{80, 13, true},
		{41, 24, true},
		{40, 10, true},
		{30, 1, true},
	}
	for _, tt := range tests {
		h := newHarness(t, nil)
		h.size(tt.w, tt.h)
		v := h.view()
		if got := !strings.Contains(v, "█"); got != tt.compact {
			t.Errorf("%dx%d: compact = %v, want %v:\n%s", tt.w, tt.h, got, tt.compact, v)
		}
		if n := strings.Count(v, "\n") + 1; n > tt.h {
			t.Errorf("%dx%d: %d rows", tt.w, tt.h, n)
		}
		for _, row := range strings.Split(v, "\n") {
			if ansi.StringWidth(row) > tt.w {
				t.Errorf("%dx%d: row wider than the window: %q", tt.w, tt.h, row)
			}
		}
	}
}

func TestCompactLine(t *testing.T) {
	tests := []struct {
		w, h  int
		setup func(*harness)
		want  []string
	}{
		{40, 10, nil, []string{"🍅 FOCUS  25:00  ──────────  ● ○ ○ ○", "space start · s skip · q quit"}},
		{40, 10, func(h *harness) { h.press("space") }, []string{"space pause · s skip · r reset · q quit"}},
		{40, 10, func(h *harness) { h.press("space", "space") }, []string{"🍅 PAUSED  25:00", "space resume"}},
		{40, 10, func(h *harness) { h.press("s") }, []string{"☕ SHORT BREAK  05:00"}},
		{40, 10, func(h *harness) {
			h.press("space")
			h.clk.add(25 * time.Minute)
			h.tick()
		}, []string{"☕ SHORT BREAK  05:00", "space to start short break"}},
		{40, 10, func(h *harness) { h.press("t") }, []string{"writing report"}},
		{40, 1, nil, []string{"🍅 FOCUS  25:00"}},
		{20, 5, nil, []string{"🍅 FOCUS  25:00"}},
		{12, 5, nil, []string{"🍅  25:00"}},
	}
	for _, tt := range tests {
		h := newHarness(t, nil)
		h.size(tt.w, tt.h)
		if tt.setup != nil {
			tt.setup(h)
		}
		v := h.view()
		for _, want := range tt.want {
			if !strings.Contains(v, want) {
				t.Errorf("%dx%d: lacks %q:\n%s", tt.w, tt.h, want, v)
			}
		}
	}
}

func TestLongPhaseUsesHours(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Focus = config.Duration{Duration: 90 * time.Minute} })
	if v := h.view(); !hasClock(v, "1:30:00") {
		t.Errorf("90m focus:\n%s", v)
	}
}

func TestWindowTitleAndProgressBar(t *testing.T) {
	h := newHarness(t, nil)
	if v := h.m.View(); v.WindowTitle != "🍅 25:00 · focus" || v.ProgressBar != nil || !v.AltScreen {
		t.Errorf("idle: title %q, bar %v, alt %v", v.WindowTitle, v.ProgressBar, v.AltScreen)
	}
	h.press("space")
	h.clk.add(6 * time.Minute)
	if v := h.m.View(); v.WindowTitle != "🍅 19:00 · focus" || *v.ProgressBar != (tea.ProgressBar{State: tea.ProgressBarDefault, Value: 24}) {
		t.Errorf("running: title %q, bar %+v", v.WindowTitle, v.ProgressBar)
	}
	h.press("space")
	if v := h.m.View(); v.WindowTitle != "🍅 19:00 · focus · paused" || v.ProgressBar.State != tea.ProgressBarWarning {
		t.Errorf("paused: title %q, bar %+v", v.WindowTitle, v.ProgressBar)
	}
	h.press("s")
	if v := h.m.View(); v.WindowTitle != "☕ 05:00 · short break" {
		t.Errorf("break: title %q", v.WindowTitle)
	}
}

func TestNoContentBeforeWindowSize(t *testing.T) {
	m, err := newModel(Options{Config: config.Default()})
	if err != nil {
		t.Fatal(err)
	}
	if c := m.View().Content; c != "" {
		t.Errorf("content before WindowSizeMsg = %q", c)
	}
}

func TestTickRearms(t *testing.T) {
	h := newHarness(t, nil)
	if h.m.Init() == nil {
		t.Error("Init does not start the tick loop")
	}
	if _, cmd := h.m.Update(tickMsg(h.clk.now())); cmd == nil {
		t.Error("a tick does not schedule the next one")
	}
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 80, Height: 24}, keyMsg("a"), keyMsg("space")} {
		if _, cmd := h.m.Update(msg); cmd != nil {
			t.Errorf("%T starts a command (a second tick loop?)", msg)
		}
	}
}

func TestStatsView(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, 15+d, 0, 0, 0, 0, time.UTC) } // 15th is a Thursday
	var s history.Stats
	for i := range s.Week {
		s.Week[i].Date = day(i)
	}
	s.Week[0] = history.Day{Date: day(0), Count: 4, Focus: 100 * time.Minute}
	s.Week[5] = history.Day{Date: day(5), Count: 1, Focus: 25 * time.Minute}
	s.Week[6] = history.Day{Date: day(6), Count: 2, Focus: 50 * time.Minute}
	s.TodayCount, s.TodayFocus, s.Streak = 2, 50*time.Minute, 2
	s.TotalCount, s.TotalFocus = 57, 23*time.Hour+45*time.Minute

	pal, _ := theme.Get(theme.Default)
	got := ansi.Strip(StatsView(s, pal))
	row := func(cells ...string) string { return strings.TrimRight(strings.Join(cells, " "), " ") }
	const (
		none  = "     "
		full  = "█████"
		floor = "▁▁▁▁▁"
	)
	want := strings.Join([]string{
		"today     2 sessions · 50m",
		"streak    🔥 2 days",
		"all time  57 sessions · 23h 45m",
		"",
		row("1h40m", none, none, none, none, none, none),
		row(full, none, none, none, none, none, none),
		row(full, none, none, none, none, none, " 50m "),
		row(full, none, none, none, none, " 25m ", full),
		row(full, floor, floor, floor, floor, full, full),
		row("  T  ", "  F  ", "  S  ", "  S  ", "  M  ", "  T  ", "  W  "),
	}, "\n")
	if got != want {
		t.Errorf("StatsView:\n%s\nwant:\n%s", got, want)
	}

	s = history.Stats{}
	for i := range s.Week {
		s.Week[i].Date = day(i)
	}
	want = strings.Join([]string{
		"today     0 sessions · 0m",
		"streak    0 days",
		"all time  0 sessions · 0m",
		"",
		row(floor, floor, floor, floor, floor, floor, floor),
		row("  T  ", "  F  ", "  S  ", "  S  ", "  M  ", "  T  ", "  W  "),
	}, "\n")
	if got := ansi.Strip(StatsView(s, pal)); got != want {
		t.Errorf("empty StatsView:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatSpan(t *testing.T) {
	tests := []struct {
		d          time.Duration
		span, cell string
	}{
		{0, "0m", "0m"},
		{30 * time.Second, "30s", "30s"},
		{25 * time.Minute, "25m", "25m"},
		{2 * time.Hour, "2h", "2h"},
		{75 * time.Minute, "1h 15m", "1h15m"},
		{10*time.Hour + 30*time.Minute, "10h 30m", "10h"},
	}
	for _, tt := range tests {
		if got := formatSpan(tt.d); got != tt.span {
			t.Errorf("formatSpan(%v) = %q, want %q", tt.d, got, tt.span)
		}
		if got := spanLabel(tt.d); got != tt.cell {
			t.Errorf("spanLabel(%v) = %q, want %q", tt.d, got, tt.cell)
		}
	}
}
