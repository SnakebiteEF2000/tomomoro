package theme

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	red   = lipgloss.Color("#ff0000")
	blue  = lipgloss.Color("#0000ff")
	green = lipgloss.Color("#00ff00")
	gray  = lipgloss.Color("#333333")
)

// cell is one printed rune and the SGR sequence active when it was printed
// ("" when unstyled).
type cell struct {
	r   rune
	sgr string
}

// cells splits a styled string into printed runes, each tagged with the SGR
// sequence in effect. It understands only the CSI ... m sequences lipgloss
// emits for foreground colors.
func cells(t *testing.T, s string) []cell {
	t.Helper()
	var out []cell
	sgr := ""
	for len(s) > 0 {
		if strings.HasPrefix(s, "\x1b[") {
			end := strings.IndexByte(s, 'm')
			if end < 0 {
				t.Fatalf("unterminated escape in %q", s)
			}
			seq := s[:end+1]
			if seq == "\x1b[m" || seq == "\x1b[0m" {
				sgr = ""
			} else {
				sgr = seq
			}
			s = s[end+1:]
			continue
		}
		r := []rune(s)[0]
		out = append(out, cell{r, sgr})
		s = s[len(string(r)):]
	}
	return out
}

// sgrFor returns the SGR sequence lipgloss uses for foreground c.
func sgrFor(c color.Color) string {
	styled := lipgloss.NewStyle().Foreground(c).Render("x")
	return styled[:strings.IndexByte(styled, 'x')]
}

func TestBigDigitsShape(t *testing.T) {
	tests := []struct {
		in    string
		width int
	}{
		{"", 0},
		{"0", 6},
		{":", 3},
		{"00:00", 31},
		{"24:13", 31},
		{"12:34", 31},
		{"56:78", 31},
		{"90:00", 31},
		{"100:00", 38},
		{"5", 6},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			rows := BigDigits(tt.in)
			if len(rows) != DigitHeight {
				t.Fatalf("got %d rows, want %d", len(rows), DigitHeight)
			}
			for i, row := range rows {
				if w := ansi.StringWidth(row); w != tt.width {
					t.Errorf("row %d width = %d, want %d: %q", i, w, tt.width, row)
				}
				if ansi.Strip(row) != row {
					t.Errorf("row %d is styled: %q", i, row)
				}
			}
		})
	}
}

func TestBigDigitsMatchesApprovedDesign(t *testing.T) {
	want := map[string][]string{
		"2": {"██████", "    ██", "██████", "██    ", "██████"},
		"3": {"██████", "    ██", "██████", "    ██", "██████"},
		"4": {"██  ██", "██  ██", "██████", "    ██", "    ██"},
		":": {"   ", " ▀ ", "   ", " ▄ ", "   "},
	}
	for in, w := range want {
		got := BigDigits(in)
		if strings.Join(got, "\n") != strings.Join(w, "\n") {
			t.Errorf("BigDigits(%q) =\n%s\nwant\n%s", in, strings.Join(got, "\n"), strings.Join(w, "\n"))
		}
	}
}

func TestBigDigitsDistinct(t *testing.T) {
	seen := map[string]rune{}
	for d := '0'; d <= '9'; d++ {
		g := ansi.Strip(strings.Join(BigDigits(string(d)), "\n"))
		if strings.TrimSpace(g) == "" {
			t.Errorf("digit %c is blank", d)
		}
		if prev, ok := seen[g]; ok {
			t.Errorf("digits %c and %c share a glyph:\n%s", prev, d, g)
		}
		seen[g] = d
	}
}

func TestBigDigitsUnsupportedIsBlank(t *testing.T) {
	for _, row := range BigDigits("x") {
		if row != "      " {
			t.Errorf("unsupported rune row = %q, want 6 spaces", row)
		}
	}
	rows := BigDigits("1x1")
	if w := ansi.StringWidth(rows[0]); w != 20 {
		t.Errorf("BigDigits(\"1x1\") width = %d, want 20", w)
	}
}

func TestBigDigitsPreview(t *testing.T) {
	for _, s := range []string{"00:00", "12:34", "56:78", "90:00"} {
		t.Logf("%s\n%s", s, strings.Join(BigDigits(s), "\n"))
	}
}

func TestGradientLinesKeepsText(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"digits", BigDigits("12:34")},
		{"ragged", []string{"a b ", " x", "", "longer line here  "}},
		{"tabs", []string{"a\tb"}},
		{"empty", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GradientLines(tt.lines, red, blue)
			if len(got) != len(tt.lines) {
				t.Fatalf("got %d lines, want %d", len(got), len(tt.lines))
			}
			for i := range got {
				if s := ansi.Strip(got[i]); s != tt.lines[i] {
					t.Errorf("line %d stripped = %q, want %q", i, s, tt.lines[i])
				}
			}
		})
	}
}

func TestGradientLinesColumns(t *testing.T) {
	lines := []string{"█ ███", "█████", "██"}
	got := GradientLines(lines, red, blue)

	colSGR := map[int]string{}
	for row, line := range got {
		for col, c := range cells(t, line) {
			if c.r == ' ' {
				if c.sgr != "" {
					t.Errorf("row %d col %d: space styled %q", row, col, c.sgr)
				}
				continue
			}
			if c.sgr == "" {
				t.Errorf("row %d col %d: %q unstyled", row, col, c.r)
				continue
			}
			if prev, ok := colSGR[col]; ok && prev != c.sgr {
				t.Errorf("col %d: row %d color %q differs from %q", col, row, c.sgr, prev)
			}
			colSGR[col] = c.sgr
		}
	}
	if colSGR[0] != sgrFor(red) {
		t.Errorf("first column = %q, want from color %q", colSGR[0], sgrFor(red))
	}
	if colSGR[4] != sgrFor(blue) {
		t.Errorf("last column = %q, want to color %q", colSGR[4], sgrFor(blue))
	}
	if colSGR[0] == colSGR[2] || colSGR[2] == colSGR[4] {
		t.Errorf("gradient does not change across columns: %v", colSGR)
	}
}

func TestGradientLinesNilColors(t *testing.T) {
	lines := BigDigits("12:34")
	got := GradientLines(lines, nil, nil)
	for i := range got {
		if got[i] != lines[i] {
			t.Errorf("line %d = %q, want unstyled %q", i, got[i], lines[i])
		}
	}
}

func TestBarWidth(t *testing.T) {
	widths := []int{-3, 0, 1, 2, 3, 7, 10, 33, 80}
	pcts := []float64{math.Inf(-1), -0.5, 0, 0.01, 0.1, 0.25, 0.5, 0.51, 0.999, 1, 1.5, math.Inf(1), math.NaN()}
	for _, w := range widths {
		for _, pct := range pcts {
			bar := Bar(w, pct, red, blue, gray)
			if w <= 0 {
				if bar != "" {
					t.Errorf("Bar(%d, %v) = %q, want empty", w, pct, bar)
				}
				continue
			}
			if got := ansi.StringWidth(bar); got != w {
				t.Errorf("Bar(%d, %v) width = %d", w, pct, got)
			}

			p := pct
			switch {
			case math.IsNaN(p) || p < 0:
				p = 0
			case p > 1:
				p = 1
			}
			wantFilled := int(math.Round(p * float64(w)))
			plain := ansi.Strip(bar)
			if got := strings.Count(plain, "━"); got != wantFilled {
				t.Errorf("Bar(%d, %v) filled = %d, want %d", w, pct, got, wantFilled)
			}
			if got := strings.Count(plain, "─"); got != w-wantFilled {
				t.Errorf("Bar(%d, %v) track = %d, want %d", w, pct, got, w-wantFilled)
			}
			if !strings.HasPrefix(plain, strings.Repeat("━", wantFilled)) {
				t.Errorf("Bar(%d, %v) = %q, filled part not first", w, pct, plain)
			}
		}
	}
}

func TestBarColors(t *testing.T) {
	const w = 24
	full := cells(t, Bar(w, 1, red, blue, gray))
	if full[0].sgr != sgrFor(red) || full[w-1].sgr != sgrFor(blue) {
		t.Errorf("full bar ends = %q … %q, want %q … %q", full[0].sgr, full[w-1].sgr, sgrFor(red), sgrFor(blue))
	}

	for filled := 0; filled <= w; filled++ {
		got := cells(t, Bar(w, float64(filled)/w, red, blue, gray))
		for i, c := range got {
			want := sgrFor(gray)
			if i < filled {
				want = full[i].sgr // gradient spans the full width
			}
			if c.sgr != want {
				t.Errorf("filled=%d cell %d color = %q, want %q", filled, i, c.sgr, want)
			}
		}
	}
}

func TestBarNilColors(t *testing.T) {
	for _, w := range []int{1, 2, 5} {
		if got := ansi.Strip(Bar(w, 0.5, nil, nil, nil)); ansi.StringWidth(got) != w {
			t.Errorf("Bar(%d) with nil colors = %q", w, got)
		}
	}
}

func TestDots(t *testing.T) {
	tests := []struct {
		done, total int
		want        string
	}{
		{0, 4, "○ ○ ○ ○"},
		{1, 4, "● ○ ○ ○"},
		{2, 4, "● ● ○ ○"},
		{4, 4, "● ● ● ●"},
		{5, 4, "● ● ● ●"},
		{-1, 4, "○ ○ ○ ○"},
		{1, 1, "●"},
		{0, 1, "○"},
		{0, 0, ""},
		{3, 0, ""},
		{2, -1, ""},
	}
	for _, tt := range tests {
		got := Dots(tt.done, tt.total, green, gray)
		if plain := ansi.Strip(got); plain != tt.want {
			t.Errorf("Dots(%d, %d) = %q, want %q", tt.done, tt.total, plain, tt.want)
		}
		for i, c := range cells(t, got) {
			var want string
			switch c.r {
			case '●':
				want = sgrFor(green)
			case '○':
				want = sgrFor(gray)
			}
			if c.sgr != want {
				t.Errorf("Dots(%d, %d) cell %d %q color = %q, want %q", tt.done, tt.total, i, c.r, c.sgr, want)
			}
		}
	}
}
