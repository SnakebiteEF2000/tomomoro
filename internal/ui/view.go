package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
	"github.com/SnakebiteEF2000/tomomoro/internal/timer"
)

const (
	fullHeight = 14 // rows the full layout needs: 13 for the clock block, 1 for the status line
	pctWidth   = 6  // room for "  100%" after the bar
	compactBar = 10 // bar width in the compact layout
	maxDots    = 12 // longer cycles show "● n/N" instead of one dot per session
	taskSep    = "   ·  "
)

// View renders the model.
func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	v.ProgressBar = m.progressBar()
	if m.hasColor {
		// Bubble Tea resets the terminal background on exit, kill, panic
		// and suspend (cursedRenderer.close).
		v.BackgroundColor = m.pal.Base
	}
	return v
}

// render draws the window content for the current size.
func (m *model) render() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return "" // no WindowSizeMsg yet
	}
	switch m.screen {
	case screenStats:
		return frame(m.statsLines(w), w, h, m.statusLine())
	case screenHelp:
		return frame(m.helpLines(w), w, h, m.statusLine())
	}
	if lines, ok := m.fullLines(w); ok && h >= fullHeight {
		return frame(lines, w, h, m.statusLine())
	}
	return m.compactView(w, h)
}

// fits reports whether the window is large enough for the full layout.
func (m *model) fits() bool {
	_, ok := m.fullLines(m.width)
	return ok && m.height >= fullHeight
}

// fullLines lays out the big clock for a window w cells wide: header, digits,
// bar, session dots with the task, and key hints. The digits are centered and
// the bar sits flush under them with the percentage hanging to its right. It
// reports false when the layout does not fit.
func (m *model) fullLines(w int) ([]string, bool) {
	acc := m.accent()
	rows := theme.BigDigits(m.clock())
	dw := lipgloss.Width(rows[0])
	left := (w - dw) / 2
	header, hints := m.header(), m.hints(w)
	if left < 0 || left+dw+pctWidth > w || lipgloss.Width(header) > w || lipgloss.Width(hints) > w {
		return nil, false
	}

	from, to := acc[0], acc[1]
	if m.timer.State() == timer.Paused {
		from, to = m.pal.Muted, m.pal.Muted
	}
	indent := strings.Repeat(" ", left)
	lines := []string{center(header, w), ""}
	for _, r := range theme.GradientLines(rows, from, to) {
		lines = append(lines, indent+r)
	}
	bar := theme.Bar(dw, m.timer.Progress(), acc[0], acc[1], m.pal.Surface)
	pct := fg(m.pal.Subtle).Render(fmt.Sprintf("  %d%%", m.percent()))
	return append(lines,
		"",
		indent+bar+pct,
		"",
		center(m.sessionLine(w), w),
		"",
		center(hints, w),
	), true
}

// compactView is the one-line fallback for small windows, with a hint line
// below it when the height allows. Parts are dropped from the right until
// the line fits.
func (m *model) compactView(w, h int) string {
	acc := m.accent()
	label, clockColor := strings.ToUpper(m.timer.Phase().String()), acc[0]
	if m.timer.State() == timer.Paused {
		label, clockColor = "PAUSED", m.pal.Muted
	}
	icon := m.icon()
	head := icon + " " + fg(acc[0]).Bold(true).Render(label)
	clock := fg(clockColor).Bold(true).Render(m.clock())
	bar := theme.Bar(compactBar, m.timer.Progress(), acc[0], acc[1], m.pal.Surface)

	var line string
	for _, parts := range [][]string{
		{head, clock, bar, m.dots()},
		{head, clock, bar},
		{head, clock},
		{icon, clock},
		{clock},
	} {
		line = strings.Join(parts, "  ")
		if lipgloss.Width(line) <= w {
			break
		}
	}

	var second string
	switch {
	case m.editing:
		second = m.input.View()
	case m.toast != "" || m.status != "":
		second = m.statusLine()
	case m.waiting:
		second = fg(acc[0]).Render("space to start " + m.timer.Phase().String())
	default:
		second = m.hints(w)
	}
	second = ansi.Truncate(second, w, "…")

	lines := []string{center(line, w)}
	switch {
	case h >= 3:
		lines = append(lines, "", center(second, w))
	case h == 2:
		lines = append(lines, center(second, w))
	}
	return frame(lines, w, h, "")
}

// header is the line above the digits: the phase and session, PAUSED, or the
// prompt to start the phase that is waiting.
func (m *model) header() string {
	acc := fg(m.accent()[0]).Bold(true)
	if m.waiting {
		return acc.Render(fmt.Sprintf("✓ %s done · space to start %s", m.finished, m.timer.Phase()))
	}
	label := m.phaseLabel()
	if m.timer.State() == timer.Paused {
		return acc.Render("● PAUSED") + fg(m.pal.Subtle).Render(" · "+label)
	}
	return acc.Render("● " + label)
}

// phaseLabel names the phase and its place in the cycle.
func (m *model) phaseLabel() string {
	r := m.timer.Round()
	switch m.timer.Phase() {
	case timer.ShortBreak:
		return fmt.Sprintf("SHORT BREAK · after session %d", r)
	case timer.LongBreak:
		return "LONG BREAK"
	}
	return fmt.Sprintf("FOCUS · session %d of %d", r, m.timer.Durations().LongEvery)
}

// sessionLine is the session dots followed by the task label, or by the task
// input while editing.
func (m *model) sessionLine(w int) string {
	dots := m.dots()
	sep := fg(m.pal.Muted).Render(taskSep)
	switch {
	case m.editing:
		return dots + sep + m.input.View()
	case m.task == "":
		return dots
	}
	avail := w - lipgloss.Width(dots) - lipgloss.Width(taskSep)
	return dots + sep + fg(m.pal.Subtle).Render(ansi.Truncate(m.task, avail, "…"))
}

// dots shows the position in the cycle: one dot per session, filled up to
// the current round. A long break resets the round, and with it the dots.
func (m *model) dots() string {
	n, acc := m.timer.Durations().LongEvery, m.accent()[0]
	if n > maxDots {
		return fg(acc).Render(fmt.Sprintf("● %d/%d", m.timer.Round(), n))
	}
	return theme.Dots(m.timer.Round(), n, acc, m.pal.Muted)
}

// hint is one key hint. Extra hints are left out when space is short.
type hint struct {
	key, desc string
	extra     bool
}

// hints returns the key hints for the current state, without the extra
// hints when the full set is wider than w.
func (m *model) hints(w int) string {
	var hs []hint
	switch {
	case m.editing:
		hs = []hint{{key: "enter", desc: "save"}, {key: "esc", desc: "cancel"}}
	case m.timer.State() == timer.Running:
		hs = []hint{{key: "space", desc: "pause"}, {key: "s", desc: "skip"}, {key: "r", desc: "reset"},
			{key: "?", desc: "help", extra: true}, {key: "q", desc: "quit"}}
	case m.timer.State() == timer.Paused:
		hs = []hint{{key: "space", desc: "resume"}, {key: "s", desc: "skip"}, {key: "r", desc: "reset"},
			{key: "?", desc: "help", extra: true}, {key: "q", desc: "quit"}}
	default:
		hs = []hint{{key: "space", desc: "start"}, {key: "s", desc: "skip"}, {key: "t", desc: "task", extra: true},
			{key: "?", desc: "help", extra: true}, {key: "q", desc: "quit"}}
	}
	if s := m.renderHints(hs, true); lipgloss.Width(s) <= w {
		return s
	}
	return m.renderHints(hs, false)
}

// renderHints joins hints as "key desc · key desc", keys a step brighter
// than their descriptions.
func (m *model) renderHints(hs []hint, extra bool) string {
	key, desc := fg(m.pal.Subtle), fg(m.pal.Muted)
	var parts []string
	for _, h := range hs {
		if h.extra && !extra {
			continue
		}
		parts = append(parts, key.Render(h.key)+desc.Render(" "+h.desc))
	}
	return strings.Join(parts, desc.Render(" · "))
}

// statusLine is the toast, else the lasting status, else empty.
func (m *model) statusLine() string {
	switch {
	case m.toast != "":
		return fg(m.pal.Subtle).Render(m.toast)
	case m.status != "":
		return fg(m.pal.Muted).Render(m.status)
	}
	return ""
}

// statsLines is the stats screen.
func (m *model) statsLines(w int) []string {
	lines := []string{fg(m.pal.Focus[0]).Bold(true).Render("● STATS"), ""}
	if m.statsErr != nil {
		lines = append(lines, fg(m.pal.Muted).Render("stats unavailable: "+m.statsErr.Error()))
	} else {
		lines = append(lines, strings.Split(StatsView(m.stats, m.pal), "\n")...)
	}
	lines = append(lines, "", m.renderHints([]hint{{key: "tab", desc: "back"}, {key: "q", desc: "quit"}}, true))
	return block(lines, w)
}

// helpLines is the help screen.
func (m *model) helpLines(w int) []string {
	keys := []hint{
		{key: "space", desc: "start · pause · resume"},
		{key: "s", desc: "skip to the next phase"},
		{key: "r", desc: "restart the current phase"},
		{key: "t", desc: "edit the task label"},
		{key: "a", desc: "auto-continue (" + onOff(m.auto) + ")"},
		{key: "T", desc: "next color theme (" + m.pal.Name + ")"},
		{key: "tab", desc: "stats"},
		{key: "?", desc: "this help"},
		{key: "q", desc: "quit"},
	}
	acc := fg(m.accent()[0])
	lines := []string{acc.Bold(true).Render("● KEYS"), ""}
	for _, k := range keys {
		lines = append(lines, acc.Render(fmt.Sprintf("%-7s", k.key))+fg(m.pal.Subtle).Render(k.desc))
	}
	lines = append(lines, "", fg(m.pal.Muted).Render("any key to close"))
	return block(lines, w)
}

// windowTitle is the terminal title, e.g. "🍅 24:13 · focus".
func (m *model) windowTitle() string {
	t := fmt.Sprintf("%s %s · %s", m.icon(), m.clock(), m.timer.Phase())
	if m.timer.State() == timer.Paused {
		t += " · paused"
	}
	return t
}

// progressBar mirrors the phase progress in the terminal's own indicator:
// normal while running, warning (yellow where supported) while paused, and
// none while idle.
func (m *model) progressBar() *tea.ProgressBar {
	switch m.timer.State() {
	case timer.Running:
		return tea.NewProgressBar(tea.ProgressBarDefault, m.percent())
	case timer.Paused:
		return tea.NewProgressBar(tea.ProgressBarWarning, m.percent())
	}
	return nil
}

// accent returns the gradient of the current phase.
func (m *model) accent() [2]color.Color {
	switch m.timer.Phase() {
	case timer.ShortBreak:
		return m.pal.Short
	case timer.LongBreak:
		return m.pal.Long
	}
	return m.pal.Focus
}

// icon is 🍅 for focus and ☕ for breaks.
func (m *model) icon() string {
	if m.timer.Phase() == timer.Focus {
		return "🍅"
	}
	return "☕"
}

// clock is the remaining time as shown on screen.
func (m *model) clock() string { return formatClock(m.timer.Remaining(), m.timer.Duration()) }

// percent is the elapsed share of the phase, rounded down.
func (m *model) percent() int { return int(m.timer.Progress() * 100) }

// formatClock renders remaining time rounded up to the whole second, so a
// fresh 25m phase reads 25:00 and the clock shows 00:00 only once time is up.
// Phases of an hour or more use H:MM:SS throughout, so the digits keep their
// width as the clock counts down.
func formatClock(remaining, phase time.Duration) string {
	secs := int64(max(remaining, 0)+time.Second-1) / int64(time.Second)
	if phase >= time.Hour {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
	}
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

// cleanTask strips control characters and surrounding space from a label.
func cleanTask(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}

// frame centers lines vertically in a w×h window and puts status, if any,
// centered on the bottom row.
func frame(lines []string, w, h int, status string) string {
	out := make([]string, 0, h)
	for range max((h-len(lines))/2, 0) {
		out = append(out, "")
	}
	out = append(out, lines...)
	if status != "" {
		for len(out) < h-1 {
			out = append(out, "")
		}
		out = append(out, center(ansi.Truncate(status, w, "…"), w))
	}
	return strings.Join(out, "\n")
}

// center left-pads s to sit in the middle of w cells.
func center(s string, w int) string {
	return strings.Repeat(" ", max((w-lipgloss.Width(s))/2, 0)) + s
}

// block centers lines as one left-aligned block in w cells.
func block(lines []string, w int) []string {
	bw := 0
	for _, l := range lines {
		bw = max(bw, lipgloss.Width(l))
	}
	indent := strings.Repeat(" ", max((w-bw)/2, 0))
	out := make([]string, len(lines))
	for i, l := range lines {
		if l != "" {
			out[i] = indent + l
		}
	}
	return out
}

// fg is a style with foreground c.
func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
