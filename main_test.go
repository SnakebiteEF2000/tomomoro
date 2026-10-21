package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnakebiteEF2000/tomomoro/internal/config"
	"github.com/SnakebiteEF2000/tomomoro/internal/history"
)

// sandbox points every XDG and home lookup at temporary directories and
// returns the config directory.
func sandbox(t *testing.T) (cfgDir, dataDir string) {
	t.Helper()
	root := t.TempDir()
	cfgDir, dataDir = filepath.Join(root, "config"), filepath.Join(root, "data")
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	return cfgDir, dataDir
}

// call runs the command line and captures its output.
func call(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelp(t *testing.T) {
	sandbox(t)
	for _, args := range [][]string{{"help"}, {"-h"}, {"-help"}, {"stats", "-h"}} {
		code, out, errOut := call(args...)
		if code != 0 || !strings.Contains(out, "Usage:") || errOut != "" {
			t.Errorf("%v: code %d, stderr %q, stdout:\n%s", args, code, errOut, out)
		}
	}
	// Every flag is documented.
	_, out, _ := call("help")
	flags(new(options)).VisitAll(func(f *flag.Flag) {
		if !strings.Contains(out, "-"+f.Name+" ") && !strings.Contains(out, "-"+f.Name+",") {
			t.Errorf("usage does not mention -%s", f.Name)
		}
	})
	for _, want := range []string{"rose-pine, catppuccin-mocha, tokyo-night, gruvbox", "(default 25m)", "config init"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := call("version")
	if code != 0 || !strings.HasPrefix(out, "tomomoro ") {
		t.Errorf("version: %d %q", code, out)
	}
}

func TestConfigPathAndInit(t *testing.T) {
	cfgDir, _ := sandbox(t)
	want := filepath.Join(cfgDir, "tomomoro", "config.toml")

	if code, out, _ := call("config", "path"); code != 0 || out != want+"\n" {
		t.Errorf("config path: %d %q, want %q", code, out, want)
	}
	if code, out, errOut := call("config", "init"); code != 0 || out != "wrote "+want+"\n" {
		t.Fatalf("config init: %d %q %q", code, out, errOut)
	}
	cfg, err := config.Load(want)
	if err != nil || cfg != config.Default() {
		t.Errorf("written config loads as %+v, %v", cfg, err)
	}
	if code, _, errOut := call("config", "init"); code != 1 || !strings.Contains(errOut, "already exists") {
		t.Errorf("second init: %d %q", code, errOut)
	}

	other := filepath.Join(t.TempDir(), "x", "custom.toml")
	if code, out, _ := call("-config", other, "config", "path"); code != 0 || out != other+"\n" {
		t.Errorf("config path with -config: %d %q", code, out)
	}
	if code, _, _ := call("config", "init", "-config", other); code != 0 {
		t.Errorf("config init with a trailing -config failed: %d", code)
	}
	if _, err := os.Stat(other); err != nil {
		t.Error(err)
	}
}

func TestStats(t *testing.T) {
	_, dataDir := sandbox(t)
	now := time.Now()
	log := history.Open(filepath.Join(dataDir, "tomomoro", "history.jsonl"))
	for range 2 {
		if err := log.Append(history.Entry{Start: now.Add(-25 * time.Minute), End: now, Kind: history.KindFocus, Duration: 25 * time.Minute}); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := call("stats")
	if code != 0 || errOut != "" {
		t.Fatalf("stats: %d %q", code, errOut)
	}
	for _, want := range []string{"● tomomoro stats", "today     2 sessions · 50m", "🔥 1 day", "history  " + log.Path()} {
		if !strings.Contains(out, want) {
			t.Errorf("stats lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("stats to a non-terminal carries escape codes:\n%q", out)
	}
}

func TestErrors(t *testing.T) {
	cfgDir, _ := sandbox(t)
	bad := filepath.Join(cfgDir, "bad.toml")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("focus = \"soon\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args    []string
		code    int
		message string
	}{
		{[]string{"-theme", "solarized", "stats"}, 2, "available: rose-pine, catppuccin-mocha, tokyo-night, gruvbox"},
		{[]string{"-focus", "0s", "stats"}, 2, "focus must be at least 1s"},
		{[]string{"-nope"}, 2, "Run 'tomomoro help'"},
		{[]string{"frobnicate"}, 2, `unknown command "frobnicate"`},
		{[]string{"config"}, 2, `unknown command "config"`},
		{[]string{"-config", bad, "stats"}, 1, "config " + bad},
		{[]string{"-config", filepath.Join(cfgDir, "missing.toml"), "stats"}, 1, "missing.toml"},
	}
	for _, tt := range tests {
		code, _, errOut := call(tt.args...)
		if code != tt.code || !strings.Contains(errOut, tt.message) {
			t.Errorf("%v: code %d, stderr %q; want %d and %q", tt.args, code, errOut, tt.code, tt.message)
		}
	}
}

func TestSettingsPrecedence(t *testing.T) {
	cfgDir, _ := sandbox(t)
	path := filepath.Join(cfgDir, "tomomoro", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file := "focus = \"30m\"\nshort_break = \"10m\"\ntheme = \"gruvbox\"\nauto_continue = true\n[alerts]\nsound = false\n"
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}

	var o options
	fset := flags(&o)
	fset.SetOutput(io.Discard)
	if err := fset.Parse([]string{"-focus", "50m", "-auto=false", "-no-bell", "-every", "3", "-t", "docs"}); err != nil {
		t.Fatal(err)
	}
	cfg, code := settings(fset, &o, io.Discard)
	if code != 0 {
		t.Fatalf("settings: code %d", code)
	}
	want := config.Default()
	want.Focus.Duration = 50 * time.Minute      // flag beats file
	want.ShortBreak.Duration = 10 * time.Minute // file beats default
	want.LongBreakEvery = 3                     // flag
	want.Theme = "gruvbox"                      // file
	want.AutoContinue = false                   // -auto=false beats the file's true
	want.Alerts = config.Alerts{Notify: true, Sound: false, Bell: false}
	if cfg != want {
		t.Errorf("settings =\n%+v\nwant\n%+v", cfg, want)
	}
	if o.task != "docs" {
		t.Errorf("task = %q", o.task)
	}
}

func TestHistoryPath(t *testing.T) {
	_, dataDir := sandbox(t)
	if got, want := historyPath(config.Default()), filepath.Join(dataDir, "tomomoro", "history.jsonl"); got != want {
		t.Errorf("default history path = %q, want %q", got, want)
	}
	cfg := config.Default()
	cfg.HistoryPath = "/elsewhere/h.jsonl"
	if got := historyPath(cfg); got != cfg.HistoryPath {
		t.Errorf("configured history path = %q", got)
	}
}
