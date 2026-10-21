package theme

import (
	"image/color"
	"math"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
)

// DigitHeight is the number of rows BigDigits returns.
const DigitHeight = 5

// glyph is one BigDigits character, top row first.
type glyph [DigitHeight]string

// glyphs are the BigDigits shapes: 2-cell vertical strokes and 1-row
// horizontal strokes on a 6x5 grid. "1" carries a flag and a foot and "7" a
// slanted stem so neither reads as a bare bar.
var glyphs = map[rune]glyph{
	'0': {"██████", "██  ██", "██  ██", "██  ██", "██████"},
	'1': {"  ██  ", "████  ", "  ██  ", "  ██  ", "██████"},
	'2': {"██████", "    ██", "██████", "██    ", "██████"},
	'3': {"██████", "    ██", "██████", "    ██", "██████"},
	'4': {"██  ██", "██  ██", "██████", "    ██", "    ██"},
	'5': {"██████", "██    ", "██████", "    ██", "██████"},
	'6': {"██████", "██    ", "██████", "██  ██", "██████"},
	'7': {"██████", "    ██", "   ██ ", "  ██  ", "  ██  "},
	'8': {"██████", "██  ██", "██████", "██  ██", "██████"},
	'9': {"██████", "██  ██", "██████", "    ██", "██████"},
	':': {"   ", " ▀ ", "   ", " ▄ ", "   "},
}

// blankGlyph stands in for unsupported runes.
var blankGlyph = glyph{"      ", "      ", "      ", "      ", "      "}

// BigDigits renders s as DigitHeight rows of block glyphs, built from
// █ ▀ ▄ like the approved design:
//
//	██████ ██  ██    ██  ██████ ██████
//	    ██ ██  ██  ▀ ██      ██     ██
//	██████ ██████    ██  ██████ ██████
//	██         ██  ▄ ██      ██     ██
//	██████     ██    ██  ██████ ██████
//
// Supported runes: 0-9 and ':'. Digits are 6 cells wide, the colon 3, with
// one space between glyphs. Unsupported runes render as a blank digit-width
// gap. All rows have equal display width. The rows are plain, unstyled text.
func BigDigits(s string) []string {
	var rows [DigitHeight]strings.Builder
	for i, r := range []rune(s) {
		g, ok := glyphs[r]
		if !ok {
			g = blankGlyph
		}
		for y := range rows {
			if i > 0 {
				rows[y].WriteByte(' ')
			}
			rows[y].WriteString(g[y])
		}
	}
	out := make([]string, DigitHeight)
	for y := range rows {
		out[y] = rows[y].String()
	}
	return out
}

// GradientLines colors plain lines with a left-to-right gradient from → to.
// The color is chosen per column across the widest line, so a column has the
// same color on every row. Spaces stay unstyled.
func GradientLines(lines []string, from, to color.Color) []string {
	// Columns are counted in runes: the input is single-width text such as
	// BigDigits rows.
	width := 0
	for _, line := range lines {
		width = max(width, len([]rune(line)))
	}
	colors := gradient(width, from, to)

	out := make([]string, len(lines))
	for i, line := range lines {
		var p pen
		for col, r := range []rune(line) {
			c := colors[col]
			if unicode.IsSpace(r) {
				c = nil
			}
			p.write(string(r), c)
		}
		out[i] = p.String()
	}
	return out
}

// Bar renders a progress bar exactly width cells wide. The filled part
// (round(pct·width) cells, pct clamped to [0,1]) is "━" colored per cell along
// a gradient that spans the FULL bar width, so a cell's color does not shift
// as the bar grows. The remainder is "─" in track. width ≤ 0 returns "".
func Bar(width int, pct float64, from, to, track color.Color) string {
	if width <= 0 {
		return ""
	}
	filled := int(math.Round(clamp01(pct) * float64(width)))
	colors := gradient(width, from, to)

	var p pen
	for i := range filled {
		p.write("━", colors[i])
	}
	p.write(strings.Repeat("─", width-filled), track)
	return p.String()
}

// Dots renders total dots separated by single spaces: the first done as "●"
// in on, the rest as "○" in off. done is clamped to [0,total].
func Dots(done, total int, on, off color.Color) string {
	done = min(max(done, 0), total)
	var p pen
	for i := range total {
		if i > 0 {
			p.write(" ", nil)
		}
		if i < done {
			p.write("●", on)
		} else {
			p.write("○", off)
		}
	}
	return p.String()
}

// clamp01 clamps v to [0,1]. NaN counts as 0.
func clamp01(v float64) float64 {
	switch {
	case !(v > 0):
		return 0
	case v > 1:
		return 1
	}
	return v
}

// gradient returns exactly n colors blended from → to. lipgloss.Blend1D
// returns nothing when both stops are nil; the result is then all nil, which
// pen renders unstyled.
func gradient(n int, from, to color.Color) []color.Color {
	colors := lipgloss.Blend1D(n, from, to)
	if len(colors) < n {
		colors = make([]color.Color, n)
	}
	return colors
}

// pen builds a styled string, merging consecutive writes in the same color
// into one styled run. A nil color writes the text unstyled.
type pen struct {
	out strings.Builder
	run strings.Builder
	c   color.Color
}

// write appends s drawn in c. Runs merge only on identical color values:
// equal RGBA in different types (an ANSI index and its hex twin) renders
// differently, so it starts a new run.
func (p *pen) write(s string, c color.Color) {
	if c != p.c {
		p.flush()
		p.c = c
	}
	p.run.WriteString(s)
}

// flush renders the pending run.
func (p *pen) flush() {
	if p.run.Len() == 0 {
		return
	}
	if p.c == nil {
		p.out.WriteString(p.run.String())
	} else {
		p.out.WriteString(lipgloss.NewStyle().Foreground(p.c).Render(p.run.String()))
	}
	p.run.Reset()
}

// String flushes the pending run and returns everything written.
func (p *pen) String() string {
	p.flush()
	return p.out.String()
}
