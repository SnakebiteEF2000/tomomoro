package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/SnakebiteEF2000/tomomoro/internal/history"
	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
)

const (
	chartRows = 4 // bar rows in the week chart; each row splits into eighths
	chartCol  = 5 // cells per day column
)

// eighths draws a chart cell filled 0/8 to 8/8 from the bottom.
var eighths = [...]string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// StatsView renders s in p's colors: today's sessions, the streak, all-time
// totals, and a chart of focus time over the last 7 days, oldest first, with
// day letters in the focus gradient. The lines are left-aligned and carry no
// padding. The stats screen and the stats command share it.
func StatsView(s history.Stats, p theme.Palette) string {
	label, value := fg(p.Subtle), fg(p.Text)
	row := func(name, v string) string {
		return label.Render(fmt.Sprintf("%-10s", name)) + value.Render(v)
	}
	streak := plural(s.Streak, "day")
	if s.Streak > 0 {
		streak = "🔥 " + streak
	}
	lines := []string{
		row("today", plural(s.TodayCount, "session")+" · "+formatSpan(s.TodayFocus)),
		row("streak", streak),
		row("all time", plural(s.TotalCount, "session")+" · "+formatSpan(s.TotalFocus)),
		"",
	}
	lines = append(lines, weekChart(s.Week, p)...)
	return strings.Join(lines, "\n")
}

// weekChart draws one column per day: a bar scaled to the week's busiest
// day, its focus time above it, and the weekday letter below. Today, the
// last column, has a bold letter. A day without focus shows a flat floor, and
// a week without any shows only the floors.
func weekChart(week [7]history.Day, p theme.Palette) []string {
	var peak time.Duration
	for _, d := range week {
		peak = max(peak, d.Focus)
	}
	colors := lipgloss.Blend1D(len(week), p.Focus[0], p.Focus[1])
	for len(colors) < len(week) {
		colors = append(colors, p.Text)
	}

	// Bar heights in eighths of a row; any focus gets at least one eighth.
	heights := make([]int, len(week))
	for i, d := range week {
		if d.Focus > 0 {
			heights[i] = max(1, int(math.Round(float64(d.Focus)/float64(peak)*chartRows*8)))
		}
	}

	top := chartRows // r counts rows from the bottom; the top row only holds labels
	if peak == 0 {
		top = 0 // nothing to draw above the floor
	}
	lines := make([]string, 0, top+2)
	for r := top; r >= 0; r-- {
		cells := make([]string, len(week))
		for i, d := range week {
			cells[i] = chartCell(r, heights[i], d.Focus, colors[i], p)
		}
		lines = append(lines, strings.TrimRight(strings.Join(cells, " "), " "))
	}
	letters := make([]string, len(week))
	for i, d := range week {
		st := fg(colors[i])
		if i == len(week)-1 {
			st = st.Bold(true)
		}
		letters[i] = centerIn(st.Render(d.Date.Weekday().String()[:1]), chartCol)
	}
	return append(lines, strings.TrimRight(strings.Join(letters, " "), " "))
}

// chartCell draws row r, counted from the bottom, of a column whose bar is h
// eighths tall and represents focus.
func chartCell(r, h int, focus time.Duration, c color.Color, p theme.Palette) string {
	top := (h + 7) / 8 // rows the bar reaches into
	switch {
	case h == 0 && r == 0:
		return fg(p.Surface).Render(strings.Repeat("▁", chartCol))
	case r < top:
		return fg(c).Render(strings.Repeat(eighths[min(h-r*8, 8)], chartCol))
	case h > 0 && r == top:
		return centerIn(fg(p.Subtle).Render(spanLabel(focus)), chartCol)
	}
	return strings.Repeat(" ", chartCol)
}

// formatSpan formats a total for reading: "1h 15m", "25m", "2h", "0m", and
// seconds below a minute.
func formatSpan(d time.Duration) string {
	if d > 0 && d < time.Minute {
		return fmt.Sprintf("%ds", d/time.Second)
	}
	h, m := d/time.Hour, d%time.Hour/time.Minute
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// spanLabel is formatSpan squeezed into a chart column: "1h15m", or whole
// hours when that is still too wide.
func spanLabel(d time.Duration) string {
	if s := strings.ReplaceAll(formatSpan(d), " ", ""); len(s) <= chartCol {
		return s
	}
	return fmt.Sprintf("%dh", d/time.Hour)
}

// plural formats a count with its noun: "1 session", "3 sessions".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// centerIn pads s on both sides to w cells.
func centerIn(s string, w int) string {
	gap := max(w-lipgloss.Width(s), 0)
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}
