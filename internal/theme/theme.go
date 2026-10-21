// Package theme holds tomomoro's color palettes.
package theme

import (
	"image/color"
	"slices"

	"charm.land/lipgloss/v2"
)

// Default is the palette used when none is configured.
const Default = "rose-pine"

// Palette is the full set of colors the UI draws with. Build colors with
// lipgloss.Color("#rrggbb").
type Palette struct {
	Name string // the key Get accepts, e.g. "rose-pine"

	Base    color.Color // terminal background the palette is designed for
	Surface color.Color // panels, unfilled bar track
	Overlay color.Color // borders, separators

	Text   color.Color // primary text
	Subtle color.Color // secondary text (labels, task)
	Muted  color.Color // tertiary text (key hints, empty dots)

	// Per-phase accent gradients: [0] is the primary accent (headers, dots),
	// [1] is the gradient end. Focus is warm, breaks are cool.
	Focus [2]color.Color
	Short [2]color.Color
	Long  [2]color.Color
}

// palettes lists every bundled palette in Names order, Default first.
//
// Neutrals rise in brightness from Base to Text. Surface is the darkest tone
// that still reads as a thin "─" line on Base, so the unfilled bar track stays
// visible but quiet; Overlay sits one step above it for borders.
var palettes = []Palette{
	{
		Name:    "rose-pine",
		Base:    lipgloss.Color("#191724"),  // base
		Surface: lipgloss.Color("#403d52"),  // highlight med
		Overlay: lipgloss.Color("#524f67"),  // highlight high
		Text:    lipgloss.Color("#e0def4"),  // text
		Subtle:  lipgloss.Color("#908caa"),  // subtle
		Muted:   lipgloss.Color("#6e6a86"),  // muted
		Focus:   grad("#eb6f92", "#f6c177"), // love → gold
		Short:   grad("#9ccfd8", "#31748f"), // foam → pine
		Long:    grad("#c4a7e7", "#9ccfd8"), // iris → foam
	},
	{
		Name:    "catppuccin-mocha",
		Base:    lipgloss.Color("#1e1e2e"),  // base
		Surface: lipgloss.Color("#45475a"),  // surface1
		Overlay: lipgloss.Color("#585b70"),  // surface2
		Text:    lipgloss.Color("#cdd6f4"),  // text
		Subtle:  lipgloss.Color("#a6adc8"),  // subtext0
		Muted:   lipgloss.Color("#6c7086"),  // overlay0
		Focus:   grad("#f38ba8", "#fab387"), // red → peach
		Short:   grad("#a6e3a1", "#94e2d5"), // green → teal
		Long:    grad("#cba6f7", "#89b4fa"), // mauve → blue
	},
	{
		Name:    "tokyo-night",
		Base:    lipgloss.Color("#1a1b26"),  // bg
		Surface: lipgloss.Color("#414868"),  // terminal black
		Overlay: lipgloss.Color("#545c7e"),  // dark3
		Text:    lipgloss.Color("#c0caf5"),  // fg
		Subtle:  lipgloss.Color("#a9b1d6"),  // fg dark
		Muted:   lipgloss.Color("#565f89"),  // comment
		Focus:   grad("#f7768e", "#ff9e64"), // red → orange
		Short:   grad("#9ece6a", "#1abc9c"), // green → teal
		Long:    grad("#bb9af7", "#7aa2f7"), // magenta → blue
	},
	{
		Name:    "gruvbox",
		Base:    lipgloss.Color("#282828"),  // bg
		Surface: lipgloss.Color("#504945"),  // bg2
		Overlay: lipgloss.Color("#665c54"),  // bg3
		Text:    lipgloss.Color("#ebdbb2"),  // fg
		Subtle:  lipgloss.Color("#a89984"),  // fg4
		Muted:   lipgloss.Color("#928374"),  // gray
		Focus:   grad("#fb4934", "#fabd2f"), // red → yellow
		Short:   grad("#b8bb26", "#8ec07c"), // green → aqua
		Long:    grad("#d3869b", "#83a598"), // purple → blue
	},
}

// Names returns every palette key in a stable order, Default first:
// "rose-pine", "catppuccin-mocha", "tokyo-night", "gruvbox".
func Names() []string {
	names := make([]string, len(palettes))
	for i, p := range palettes {
		names[i] = p.Name
	}
	return names
}

// Get returns the palette for name, or false if unknown.
func Get(name string) (Palette, bool) {
	i := index(name)
	if i < 0 {
		return Palette{}, false
	}
	return palettes[i], true
}

// Next returns the palette name after name in Names order, wrapping around.
// An unknown name returns Default.
func Next(name string) string {
	i := index(name)
	if i < 0 {
		return Default
	}
	return palettes[(i+1)%len(palettes)].Name
}

// index returns the position of name in palettes, or -1 if unknown.
func index(name string) int {
	return slices.IndexFunc(palettes, func(p Palette) bool { return p.Name == name })
}

// grad builds a two-stop accent gradient from hex colors.
func grad(from, to string) [2]color.Color {
	return [2]color.Color{lipgloss.Color(from), lipgloss.Color(to)}
}
