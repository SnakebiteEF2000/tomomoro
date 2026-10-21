// Command tomomoro is a terminal Pomodoro timer.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/SnakebiteEF2000/tomomoro/internal/config"
	"github.com/SnakebiteEF2000/tomomoro/internal/history"
	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
	"github.com/SnakebiteEF2000/tomomoro/internal/ui"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options holds the command-line flags.
type options struct {
	focus, short, long time.Duration
	every              int
	task, theme, path  string
	auto               bool
	noNotify, noSound  bool
	noBell             bool
}

// flags defines the command-line flags on a new flag set. Defaults come from
// config.Default, but only flags the user sets override the config file.
func flags(o *options) *flag.FlagSet {
	d := config.Default()
	fset := flag.NewFlagSet("tomomoro", flag.ContinueOnError)
	fset.DurationVar(&o.focus, "focus", d.Focus.Duration, "focus length")
	fset.DurationVar(&o.short, "short", d.ShortBreak.Duration, "short break length")
	fset.DurationVar(&o.long, "long", d.LongBreak.Duration, "long break length")
	fset.IntVar(&o.every, "every", d.LongBreakEvery, "long break after every N focus phases")
	fset.StringVar(&o.task, "t", "", "task label")
	fset.StringVar(&o.task, "task", "", "task label")
	fset.StringVar(&o.theme, "theme", d.Theme, "color theme")
	fset.BoolVar(&o.auto, "auto", d.AutoContinue, "start each next phase automatically")
	fset.BoolVar(&o.noNotify, "no-notify", false, "disable the desktop notification")
	fset.BoolVar(&o.noSound, "no-sound", false, "disable the chime")
	fset.BoolVar(&o.noBell, "no-bell", false, "disable the terminal bell")
	fset.StringVar(&o.path, "config", config.Path(), "config file")
	return fset
}

// usage is the help text.
func usage() string {
	d := config.Default()
	short := func(v config.Duration) string { b, _ := v.MarshalText(); return string(b) }
	return fmt.Sprintf(`tomomoro: a Pomodoro timer for the terminal

Usage:
  tomomoro [flags]          start the timer
  tomomoro stats            show focus statistics
  tomomoro config init      write a starter config file
  tomomoro config path      print the config file location
  tomomoro version          print the version
  tomomoro help             show this help

Flags (override the config file):
  -focus DURATION   focus length (default %s)
  -short DURATION   short break length (default %s)
  -long DURATION    long break length (default %s)
  -every N          long break after every N focus phases (default %d)
  -t, -task TEXT    task label for this run
  -theme NAME       %s
  -auto             start each next phase automatically
  -no-notify        no desktop notification
  -no-sound         no chime
  -no-bell          no terminal bell
  -config PATH      config file (default %s)

Keys:
  space  start, pause, resume     s    skip phase      r  reset phase
  t      edit task                a    auto-continue   T  next theme
  tab    stats                    ?    help            q  quit
`, short(d.Focus), short(d.ShortBreak), short(d.LongBreak), d.LongBreakEvery,
		strings.Join(theme.Names(), ", "), config.Path())
}

// run executes the command line and returns the exit code: 0 on success, 1
// on a runtime or config file error, 2 on a usage error.
func run(args []string, stdout, stderr io.Writer) int {
	var o options
	fset := flags(&o)
	fset.SetOutput(stderr)
	fset.Usage = func() {} // errors get a short pointer to help instead
	parse := func(args []string) (int, bool) {
		err := fset.Parse(args)
		switch {
		case err == nil:
			return 0, true
		case errors.Is(err, flag.ErrHelp):
			fmt.Fprint(stdout, usage())
			return 0, false
		}
		fmt.Fprintln(stderr, "Run 'tomomoro help' for usage.")
		return 2, false
	}
	// Flags may come before, between or after the subcommand's words.
	var cmd []string
	for {
		if code, ok := parse(args); !ok {
			return code
		}
		if fset.NArg() == 0 {
			break
		}
		cmd = append(cmd, fset.Arg(0))
		args = fset.Args()[1:]
	}

	switch strings.Join(cmd, " ") {
	case "":
		return startTimer(fset, &o, stderr)
	case "stats":
		return printStats(fset, &o, stdout, stderr)
	case "config init":
		return initConfig(o.path, stdout, stderr)
	case "config path":
		fmt.Fprintln(stdout, o.path)
		return 0
	case "version":
		fmt.Fprintln(stdout, "tomomoro", version())
		return 0
	case "help":
		fmt.Fprint(stdout, usage())
		return 0
	}
	fmt.Fprintf(stderr, "tomomoro: unknown command %q\nRun 'tomomoro help' for usage.\n", strings.Join(cmd, " "))
	return 2
}

// startTimer runs the TUI.
func startTimer(fset *flag.FlagSet, o *options, stderr io.Writer) int {
	cfg, code := settings(fset, o, stderr)
	if code != 0 {
		return code
	}
	err := ui.Run(ui.Options{Config: cfg, Task: o.task, Log: history.Open(historyPath(cfg))})
	switch {
	case errors.Is(err, tea.ErrInterrupted):
		return 130 // SIGINT from outside the terminal
	case err != nil:
		fmt.Fprintln(stderr, "tomomoro:", err)
		return 1
	}
	return 0
}

// printStats writes the history summary and week chart in the theme colors.
// lipgloss drops the colors when stdout is not a terminal.
func printStats(fset *flag.FlagSet, o *options, stdout, stderr io.Writer) int {
	cfg, code := settings(fset, o, stderr)
	if code != 0 {
		return code
	}
	path := historyPath(cfg)
	entries, err := history.Open(path).All()
	if err != nil {
		fmt.Fprintln(stderr, "tomomoro:", err)
		return 1
	}
	pal, _ := theme.Get(cfg.Theme)
	title := lipgloss.NewStyle().Foreground(pal.Focus[0]).Bold(true).Render("● tomomoro stats")
	foot := lipgloss.NewStyle().Foreground(pal.Muted).Render("history  " + path)
	lipgloss.Fprintln(stdout, title+"\n\n"+ui.StatsView(history.Compute(entries, time.Now()), pal)+"\n\n"+foot)
	return 0
}

// initConfig writes the starter config, refusing to overwrite one.
func initConfig(path string, stdout, stderr io.Writer) int {
	err := config.WriteDefault(path)
	switch {
	case errors.Is(err, fs.ErrExist):
		fmt.Fprintf(stderr, "tomomoro: %s already exists; edit it, or remove it to start over\n", path)
		return 1
	case err != nil:
		fmt.Fprintln(stderr, "tomomoro:", err)
		return 1
	}
	fmt.Fprintln(stdout, "wrote", path)
	return 0
}

// settings loads the config file and applies the flags the user set on top.
// It prints any problem and returns its exit code, or 0.
func settings(fset *flag.FlagSet, o *options, stderr io.Writer) (config.Config, int) {
	set := map[string]bool{}
	fset.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// A missing file means defaults, unless the user named it explicitly.
	if set["config"] {
		if _, err := os.Stat(o.path); err != nil {
			fmt.Fprintln(stderr, "tomomoro:", err)
			return config.Config{}, 1
		}
	}
	cfg, err := config.Load(o.path)
	if err != nil {
		fmt.Fprintln(stderr, "tomomoro:", err)
		return config.Config{}, 1
	}

	for name := range set {
		switch name {
		case "focus":
			cfg.Focus.Duration = o.focus
		case "short":
			cfg.ShortBreak.Duration = o.short
		case "long":
			cfg.LongBreak.Duration = o.long
		case "every":
			cfg.LongBreakEvery = o.every
		case "theme":
			cfg.Theme = o.theme
		case "auto":
			cfg.AutoContinue = o.auto
		case "no-notify":
			cfg.Alerts.Notify = !o.noNotify
		case "no-sound":
			cfg.Alerts.Sound = !o.noSound
		case "no-bell":
			cfg.Alerts.Bell = !o.noBell
		}
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, "tomomoro: invalid flag:", err)
		return config.Config{}, 2
	}
	if _, ok := theme.Get(cfg.Theme); !ok {
		fmt.Fprintf(stderr, "tomomoro: unknown theme %q; available: %s\n", cfg.Theme, strings.Join(theme.Names(), ", "))
		return config.Config{}, 2
	}
	return cfg, 0
}

// historyPath is the configured history file, else the default location.
func historyPath(cfg config.Config) string {
	if cfg.HistoryPath != "" {
		return cfg.HistoryPath
	}
	return history.DefaultPath()
}

// version reports the module version: the tag for a `go install`ed binary,
// "(devel)" plus the VCS revision for a local build.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	v := info.Main.Version
	if v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value[:min(len(s.Value), 12)]
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return "(devel)"
	}
	return "(devel) " + rev + dirty
}
