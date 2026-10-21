// Package config loads tomomoro's settings from a TOML file.
//
// Precedence is: command-line flags (applied by main) > config file > Default.
// Paths follow XDG on every OS, including macOS: $XDG_CONFIG_HOME, else
// ~/.config. os.UserConfigDir is deliberately not used.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration is a time.Duration that reads and writes as a Go duration string
// in TOML ("25m", "1h30m", "90s").
type Duration struct {
	time.Duration
}

// UnmarshalText parses a Go duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalText formats the duration compactly ("25m", not "25m0s").
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(compact(d.Duration)), nil
}

// compact formats d like time.Duration.String but drops trailing zero units:
// "25m0s" becomes "25m" and "1h0m0s" becomes "1h". Inner zeros stay
// ("1h0m30s"), so the result always parses back with time.ParseDuration.
func compact(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// Alerts toggles each phase-end alert channel.
type Alerts struct {
	Notify bool `toml:"notify"` // desktop notification
	Sound  bool `toml:"sound"`  // chime
	Bell   bool `toml:"bell"`   // terminal BEL
}

// Config is the full set of user settings.
type Config struct {
	Focus          Duration `toml:"focus"`
	ShortBreak     Duration `toml:"short_break"`
	LongBreak      Duration `toml:"long_break"`
	LongBreakEvery int      `toml:"long_break_every"`
	Theme          string   `toml:"theme"`
	AutoContinue   bool     `toml:"auto_continue"`
	Alerts         Alerts   `toml:"alerts"`
	// HistoryPath overrides where history is stored. Empty means the history
	// package default. A leading "~/" is expanded by Load.
	HistoryPath string `toml:"history_path"`
}

// Default returns 25m focus, 5m short break, 15m long break every 4 focus
// phases, theme "rose-pine", auto-continue off, all alerts on.
func Default() Config {
	return Config{
		Focus:          Duration{25 * time.Minute},
		ShortBreak:     Duration{5 * time.Minute},
		LongBreak:      Duration{15 * time.Minute},
		LongBreakEvery: 4,
		Theme:          "rose-pine",
		AutoContinue:   false,
		Alerts:         Alerts{Notify: true, Sound: true, Bell: true},
	}
}

// Path returns the config file location:
// $XDG_CONFIG_HOME/tomomoro/config.toml, else ~/.config/tomomoro/config.toml.
func Path() string {
	// The XDG spec says relative values are invalid and must be ignored.
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "tomomoro", "config.toml")
	}
	home, _ := os.UserHomeDir() // empty on failure: falls back to a relative path
	return filepath.Join(home, ".config", "tomomoro", "config.toml")
}

// Load reads the file at path on top of Default, so a partial file only
// overrides the keys it sets. A missing file returns Default and a nil error.
// The result is validated; an invalid file returns an error naming the key.
func Load(path string) (Config, error) {
	cfg := Default()
	md, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = strconv.Quote(k.String())
		}
		noun := "key"
		if len(names) > 1 {
			noun = "keys"
		}
		return Config{}, fmt.Errorf("config %s: unknown %s %s", path, noun, strings.Join(names, ", "))
	}
	if cfg.HistoryPath, err = expandHome(cfg.HistoryPath); err != nil {
		return Config{}, fmt.Errorf("config %s: history_path: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// expandHome replaces a leading "~/" in p with the user's home directory.
func expandHome(p string) (string, error) {
	rest, ok := strings.CutPrefix(p, "~/")
	if !ok {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rest), nil
}

// Validate reports the first invalid setting: durations must be ≥ 1s,
// LongBreakEvery ≥ 1. Theme names are validated by the caller.
func (c Config) Validate() error {
	for _, f := range []struct {
		key string
		d   Duration
	}{
		{"focus", c.Focus},
		{"short_break", c.ShortBreak},
		{"long_break", c.LongBreak},
	} {
		if f.d.Duration < time.Second {
			return fmt.Errorf("%s must be at least 1s, got %s", f.key, compact(f.d.Duration))
		}
	}
	if c.LongBreakEvery < 1 {
		return fmt.Errorf("long_break_every must be at least 1, got %d", c.LongBreakEvery)
	}
	return nil
}

// WriteDefault writes a commented starter config to path, creating parent
// directories. It refuses to overwrite: an existing file returns an error
// satisfying errors.Is(err, fs.ErrExist).
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(starter()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// starter renders the commented config template from Default, so the file
// always loads back to exactly Default.
func starter() string {
	c := Default()
	return fmt.Sprintf(`# tomomoro configuration.
# Durations use Go syntax: "25m", "1h30m", "90s". Command-line flags override
# anything set here; delete a line to fall back to its default.

# Length of a focus phase.
focus = %q
# Length of a short break.
short_break = %q
# Length of a long break.
long_break = %q
# Take a long break after this many focus phases.
long_break_every = %d
# Color theme. Available: rose-pine, catppuccin-mocha, tokyo-night, gruvbox.
theme = %q
# Start the next phase automatically when one ends.
auto_continue = %t
# History file ("~/" expands). Empty uses $XDG_DATA_HOME, else ~/.local/share.
history_path = %q

# Alerts fired when a phase ends.
[alerts]
# Desktop notification.
notify = %t
# Chime sound.
sound = %t
# Terminal bell.
bell = %t
`,
		compact(c.Focus.Duration),
		compact(c.ShortBreak.Duration),
		compact(c.LongBreak.Duration),
		c.LongBreakEvery,
		c.Theme,
		c.AutoContinue,
		c.HistoryPath,
		c.Alerts.Notify,
		c.Alerts.Sound,
		c.Alerts.Bell,
	)
}
