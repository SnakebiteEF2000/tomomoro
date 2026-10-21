# tomomoro

A calm Pomodoro timer for the terminal: big gradient digits, desktop alerts, and focus stats in a single Go binary.

```
                            ● FOCUS · session 2 of 4

                          ██   ██████     ██████ ██████
                        ████   ██  ██  ▀      ██ ██  ██
                          ██   ██  ██     ██████ ██  ██
                          ██   ██  ██  ▄      ██ ██  ██
                        ██████ ██████     ██████ ██████

                        ━━━━━━━━━━━━━━━━━━─────────────  58%

                          ● ● ○ ○   ·  writing report

                space pause · s skip · r reset · ? help · q quit
```

In a small window it shrinks to a single line:

```
  🍅 FOCUS  10:30  ━━━━━━────  ● ● ○ ○
```

## Install

```sh
go install github.com/SnakebiteEF2000/tomomoro@latest
```

Requires Go 1.26 or newer. The binary lands in `$(go env GOPATH)/bin` (or `$GOBIN`). It is pure Go with no cgo, and runs on macOS and Linux.

## Usage

```sh
tomomoro                              # 25m focus, 5m short break, 15m long break every 4
tomomoro -t "write report"            # label this run's focus phases
tomomoro -focus 50m -short 10m        # longer sessions
tomomoro -theme tokyo-night -auto     # another theme, start each phase automatically
tomomoro stats                        # today, streak, totals and a 7-day chart
tomomoro config init                  # write a commented starter config
```

A phase waits for you to press space. When it ends, tomomoro alerts you and loads the next phase, which again waits for space unless auto-continue is on. The terminal title shows the remaining time, and terminals with a native progress indicator (OSC 9;4, e.g. Ghostty or Windows Terminal) show the phase progress there too.

### Keys

| Key           | Action                                                            |
| ------------- | ----------------------------------------------------------------- |
| `space`       | start, pause, resume, or start the next phase when one is waiting |
| `s`           | skip to the next phase (not recorded, no alert)                   |
| `r`           | restart the current phase                                         |
| `t`           | edit the task label (`enter` saves, `esc` cancels)                |
| `a`           | toggle auto-continue                                              |
| `T`           | cycle the color theme for this run                                |
| `tab`         | stats view (`tab` or `esc` returns)                               |
| `?`           | key help                                                          |
| `q`, `ctrl+c` | quit                                                              |

### Flags

Flags override the config file, which overrides the built-in defaults. Only the flags you pass take effect, so `-auto=false` can switch off an `auto_continue = true` from the config.

| Flag               | Meaning                                       | Default            |
| ------------------ | --------------------------------------------- | ------------------ |
| `-focus DURATION`  | focus length (`25m`, `1h30m`, `90s`)          | `25m`              |
| `-short DURATION`  | short break length                            | `5m`               |
| `-long DURATION`   | long break length                             | `15m`              |
| `-every N`         | long break after every N focus phases         | `4`                |
| `-t`, `-task TEXT` | task label for this run                       | none               |
| `-theme NAME`      | color theme (see [Themes](#themes))           | `rose-pine`        |
| `-auto`            | start each next phase automatically           | off                |
| `-no-notify`       | no desktop notification                       | notifications on   |
| `-no-sound`        | no chime                                      | chime on           |
| `-no-bell`         | no terminal bell                              | bell on            |
| `-config PATH`     | config file to read                           | see below          |

Commands: `tomomoro stats`, `tomomoro config init`, `tomomoro config path`, `tomomoro version`, `tomomoro help` (or `-h`).

## Config

The config file lives at `$XDG_CONFIG_HOME/tomomoro/config.toml`, else `~/.config/tomomoro/config.toml`, on macOS too. `tomomoro config path` prints the location and `tomomoro config init` writes this starter file (it never overwrites an existing one):

```toml
# tomomoro configuration.
# Durations use Go syntax: "25m", "1h30m", "90s". Command-line flags override
# anything set here; delete a line to fall back to its default.

# Length of a focus phase.
focus = "25m"
# Length of a short break.
short_break = "5m"
# Length of a long break.
long_break = "15m"
# Take a long break after this many focus phases.
long_break_every = 4
# Color theme. Available: rose-pine, catppuccin-mocha, tokyo-night, gruvbox.
theme = "rose-pine"
# Start the next phase automatically when one ends.
auto_continue = false
# History file ("~/" expands). Empty uses $XDG_DATA_HOME, else ~/.local/share.
history_path = ""

# Alerts fired when a phase ends.
[alerts]
# Desktop notification.
notify = true
# Chime sound.
sound = true
# Terminal bell.
bell = true
```

A missing file means defaults. Unknown keys and invalid values are reported with the key name.

## History and stats

Every focus phase that runs to completion is appended to `$XDG_DATA_HOME/tomomoro/history.jsonl`, else `~/.local/share/tomomoro/history.jsonl` (on macOS too), or to `history_path` from the config. Skipped phases and breaks are not recorded. Each line is one JSON object:

```json
{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s","task":"write report"}
```

`tomomoro stats` (or `tab` in the timer) summarizes it in the theme's colors:

```
● tomomoro stats

today     2 sessions · 50m
streak    🔥 2 days
all time  57 sessions · 23h 45m

1h40m
█████
█████                                50m
█████                          25m  █████
█████ ▁▁▁▁▁ ▁▁▁▁▁ ▁▁▁▁▁ ▁▁▁▁▁ █████ █████
  T     F     S     S     M     T     W
```

Days follow your local midnight; the streak counts back from today, or from yesterday until you finish today's first session. The colors are dropped when the output is not a terminal.

## Themes

- `rose-pine` (default)
- `catppuccin-mocha`
- `tokyo-night`
- `gruvbox`

Pick one with `-theme` or `theme` in the config, or press `T` to cycle through them while the timer runs (that choice is not saved). While tomomoro runs it sets the terminal background to the theme's base color, and Bubble Tea restores the terminal's own background on exit. With `NO_COLOR` set, tomomoro leaves colors and the background alone.

## Alerts and platforms

When a phase ends tomomoro sends a desktop notification, plays a short synthesized chime (one for focus ending, another for a break ending), and rings the terminal bell. Each can be switched off with a flag or in `[alerts]`.

- **macOS** works out of the box: notifications go through `osascript`, the chime through `afplay`. If no banner shows up, check System Settings → Notifications; macOS may list `osascript` notifications under Script Editor.
- **Linux** needs a notifier and a player on `PATH`: `notify-send` from libnotify (`libnotify-bin` on Debian and Ubuntu, `libnotify` on Fedora and Arch), with `gdbus` as a fallback; and one of `pw-play` (PipeWire), `paplay` (PulseAudio utils) or `aplay` (ALSA utils).
- **Over SSH** only the terminal bell reaches you; notifications and sound would fire on the remote machine.

Missing tools are skipped silently, and an alert never interrupts the timer. Windows is not supported.
