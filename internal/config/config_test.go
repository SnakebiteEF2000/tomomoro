package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFile writes content to a config.toml in a fresh temp dir and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefault(t *testing.T) {
	want := Config{
		Focus:          Duration{25 * time.Minute},
		ShortBreak:     Duration{5 * time.Minute},
		LongBreak:      Duration{15 * time.Minute},
		LongBreakEvery: 4,
		Theme:          "rose-pine",
		Alerts:         Alerts{Notify: true, Sound: true, Bell: true},
	}
	if got := Default(); got != want {
		t.Fatalf("Default() = %+v, want %+v", got, want)
	}
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope", "config.toml"))
	if err != nil {
		t.Fatalf("Load(missing) error = %v", err)
	}
	if cfg != Default() {
		t.Fatalf("Load(missing) = %+v, want Default()", cfg)
	}
}

func TestLoadPartialOverride(t *testing.T) {
	path := writeFile(t, `
focus = "50m"
theme = "gruvbox"

[alerts]
sound = false
`)
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Default()
	want.Focus = Duration{50 * time.Minute}
	want.Theme = "gruvbox"
	want.Alerts.Sound = false
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFullFile(t *testing.T) {
	path := writeFile(t, `
focus = "1h30m"
short_break = "90s"
long_break = "20m"
long_break_every = 3
theme = "tokyo-night"
auto_continue = true
history_path = "/var/tmp/h.jsonl"

[alerts]
notify = false
sound = false
bell = false
`)
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		Focus:          Duration{90 * time.Minute},
		ShortBreak:     Duration{90 * time.Second},
		LongBreak:      Duration{20 * time.Minute},
		LongBreakEvery: 3,
		Theme:          "tokyo-night",
		AutoContinue:   true,
		Alerts:         Alerts{},
		HistoryPath:    "/var/tmp/h.jsonl",
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadErrorsNameKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
	}{
		{"bad duration", `focus = "abc"`, "focus"},
		{"duration without unit", `short_break = 25`, "short_break"},
		{"wrong type", "[alerts]\nnotify = \"yes\"", "alerts.notify"},
		{"wrong int type", `long_break_every = "four"`, "long_break_every"},
		{"unknown key", `focuss = "25m"`, "focuss"},
		{"unknown nested key", "[alerts]\nchime = true", "alerts.chime"},
		{"unknown table", "[colors]\nbg = \"#000\"", "colors"},
		{"too short", `long_break = "500ms"`, "long_break"},
		{"zero", `focus = "0s"`, "focus"},
		{"negative", `short_break = "-5m"`, "short_break"},
		{"long break every zero", `long_break_every = 0`, "long_break_every"},
		{"syntax", `focus = `, "focus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, tt.content)
			cfg, err := Load(path)
			if err == nil {
				t.Fatalf("Load() = %+v, want error", cfg)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("error %q does not name key %q", err, tt.key)
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name file %q", err, path)
			}
			if cfg != (Config{}) {
				t.Fatalf("Load() returned %+v with error, want zero Config", cfg)
			}
		})
	}
}

func TestLoadExpandsHistoryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		in, want string
	}{
		{"~/notes/pomo.jsonl", filepath.Join(home, "notes", "pomo.jsonl")},
		{"/abs/pomo.jsonl", "/abs/pomo.jsonl"},
		{"~user/pomo.jsonl", "~user/pomo.jsonl"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			path := writeFile(t, `history_path = "`+tt.in+`"`)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.HistoryPath != tt.want {
				t.Fatalf("HistoryPath = %q, want %q", cfg.HistoryPath, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		key    string // empty means valid
	}{
		{"default", func(c *Config) {}, ""},
		{"one second minimum ok", func(c *Config) {
			c.Focus, c.ShortBreak, c.LongBreak = Duration{time.Second}, Duration{time.Second}, Duration{time.Second}
		}, ""},
		{"long every one ok", func(c *Config) { c.LongBreakEvery = 1 }, ""},
		{"empty theme left to caller", func(c *Config) { c.Theme = "" }, ""},
		{"focus zero", func(c *Config) { c.Focus = Duration{} }, "focus"},
		{"focus sub-second", func(c *Config) { c.Focus = Duration{999 * time.Millisecond} }, "focus"},
		{"short negative", func(c *Config) { c.ShortBreak = Duration{-time.Minute} }, "short_break"},
		{"long zero", func(c *Config) { c.LongBreak = Duration{} }, "long_break"},
		{"long every zero", func(c *Config) { c.LongBreakEvery = 0 }, "long_break_every"},
		{"long every negative", func(c *Config) { c.LongBreakEvery = -2 }, "long_break_every"},
		{"first invalid wins", func(c *Config) {
			c.Focus = Duration{}
			c.LongBreakEvery = 0
		}, "focus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(&c)
			err := c.Validate()
			if tt.key == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error naming %q", tt.key)
			}
			if !strings.HasPrefix(err.Error(), tt.key+" ") {
				t.Fatalf("Validate() = %q, want it to start with key %q", err, tt.key)
			}
		})
	}
}

func TestPath(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"xdg set", xdg, filepath.Join(xdg, "tomomoro", "config.toml")},
		{"xdg unset", "", filepath.Join(home, ".config", "tomomoro", "config.toml")},
		{"xdg relative ignored", "rel/dir", filepath.Join(home, ".config", "tomomoro", "config.toml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			if got := Path(); got != tt.want {
				t.Fatalf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "config.toml") // parents don't exist yet
	if err := WriteDefault(path); err != nil {
		t.Fatalf("WriteDefault() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(written default) error = %v", err)
	}
	if got != Default() {
		t.Fatalf("Load(written default) = %+v, want Default() %+v", got, Default())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, key := range []string{
		"focus", "short_break", "long_break", "long_break_every", "theme",
		"auto_continue", "history_path", "[alerts]", "notify", "sound", "bell",
	} {
		if !strings.Contains(text, "\n"+key+" ") && !strings.Contains(text, "\n"+key+"\n") {
			t.Errorf("template is missing key %q", key)
		}
	}
	for _, theme := range []string{"rose-pine", "catppuccin-mocha", "tokyo-night", "gruvbox"} {
		if !strings.Contains(text, theme) {
			t.Errorf("template does not mention theme %q", theme)
		}
	}
	// Every setting line is preceded by a one-line comment.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if i == 0 || !strings.HasPrefix(lines[i-1], "# ") {
			t.Errorf("line %d %q has no comment above it", i+1, line)
		}
	}
	if !strings.Contains(text, `focus = "25m"`) {
		t.Errorf("template should use compact durations, got:\n%s", text)
	}
}

func TestWriteDefaultRefusesOverwrite(t *testing.T) {
	path := writeFile(t, "focus = \"1m\"\n")
	err := WriteDefault(path)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("WriteDefault(existing) = %v, want fs.ErrExist", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "focus = \"1m\"\n" {
		t.Fatalf("existing file was modified: %q", data)
	}
}

func TestDurationText(t *testing.T) {
	tests := []struct {
		d    time.Duration
		text string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{25 * time.Minute, "25m"},
		{90 * time.Minute, "1h30m"},
		{time.Hour, "1h"},
		{2*time.Hour + 30*time.Second, "2h0m30s"},
		{1500 * time.Millisecond, "1.5s"},
		{500 * time.Millisecond, "500ms"},
		{-25 * time.Minute, "-25m"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			b, err := Duration{tt.d}.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(b) != tt.text {
				t.Fatalf("MarshalText(%v) = %q, want %q", tt.d, b, tt.text)
			}
			var got Duration
			if err := got.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", b, err)
			}
			if got.Duration != tt.d {
				t.Fatalf("UnmarshalText(%q) = %v, want %v", b, got.Duration, tt.d)
			}
		})
	}
}

func TestDurationUnmarshalText(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"25m0s", 25 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"90s", 90 * time.Second, false},
		{"", 0, true},
		{"25", 0, true},
		{"abc", 0, true},
		{"5 m", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalText(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && d.Duration != tt.want {
				t.Fatalf("UnmarshalText(%q) = %v, want %v", tt.in, d.Duration, tt.want)
			}
		})
	}
}
