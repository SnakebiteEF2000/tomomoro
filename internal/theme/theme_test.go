package theme

import (
	"image/color"
	"slices"
	"testing"
)

func TestNames(t *testing.T) {
	want := []string{"rose-pine", "catppuccin-mocha", "tokyo-night", "gruvbox"}
	got := Names()
	if !slices.Equal(got, want) {
		t.Fatalf("Names() = %q, want %q", got, want)
	}
	if got[0] != Default {
		t.Errorf("Names()[0] = %q, want Default %q", got[0], Default)
	}

	got[0] = "mutated"
	if Names()[0] != Default {
		t.Error("mutating the Names() result changed later calls")
	}
}

func TestGet(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			p, ok := Get(name)
			if !ok {
				t.Fatalf("Get(%q) not found", name)
			}
			if p.Name != name {
				t.Errorf("Get(%q).Name = %q", name, p.Name)
			}
		})
	}

	for _, name := range []string{"", "nope", "Rose-Pine", "rose-pine "} {
		if p, ok := Get(name); ok {
			t.Errorf("Get(%q) = %+v, true; want false", name, p)
		}
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"rose-pine", "catppuccin-mocha"},
		{"catppuccin-mocha", "tokyo-night"},
		{"tokyo-night", "gruvbox"},
		{"gruvbox", "rose-pine"},
		{"", Default},
		{"unknown", Default},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Next(tt.name); got != tt.want {
				t.Errorf("Next(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}

	name := Default
	for range Names() {
		name = Next(name)
	}
	if name != Default {
		t.Errorf("cycling Next len(Names) times ended at %q, want %q", name, Default)
	}
}

func TestPaletteColors(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			p, _ := Get(name)
			colors := map[string]color.Color{
				"Base":     p.Base,
				"Surface":  p.Surface,
				"Overlay":  p.Overlay,
				"Text":     p.Text,
				"Subtle":   p.Subtle,
				"Muted":    p.Muted,
				"Focus[0]": p.Focus[0],
				"Focus[1]": p.Focus[1],
				"Short[0]": p.Short[0],
				"Short[1]": p.Short[1],
				"Long[0]":  p.Long[0],
				"Long[1]":  p.Long[1],
			}
			for field, c := range colors {
				// lipgloss.Color returns NoColor for a malformed hex, so
				// require a parsed RGBA rather than just non-nil.
				if _, ok := c.(color.RGBA); !ok {
					t.Errorf("%s = %#v, want a parsed hex color.RGBA", field, c)
				}
			}
		})
	}
}
