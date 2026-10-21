// Package ui is tomomoro's Bubble Tea v2 interface.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/SnakebiteEF2000/tomomoro/internal/alert"
	"github.com/SnakebiteEF2000/tomomoro/internal/config"
	"github.com/SnakebiteEF2000/tomomoro/internal/history"
	"github.com/SnakebiteEF2000/tomomoro/internal/theme"
	"github.com/SnakebiteEF2000/tomomoro/internal/timer"
)

// Options configures a UI run. Config is final (flags already applied).
type Options struct {
	Config config.Config
	Task   string           // initial task label, may be empty
	Log    *history.Log     // where completed focus phases are appended
	Now    func() time.Time // nil means time.Now
}

// Run starts the program and blocks until the user quits.
func Run(o Options) error {
	m, err := newModel(o)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

const (
	tickEvery = 250 * time.Millisecond // clock refresh; the timer itself reads the wall clock
	toastFor  = 3 * time.Second        // how long a toast stays on screen
	taskLimit = 80                     // longest task label, in runes
)

// screen is what the window shows.
type screen int

const (
	screenTimer screen = iota
	screenStats
	screenHelp
)

// tickMsg drives the clock.
type tickMsg time.Time

// tick schedules the next tickMsg. Only Init and the tickMsg handler call it,
// so exactly one tick loop runs.
func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// model is the Bubble Tea model. Only Update mutates it.
type model struct {
	now   func() time.Time
	cfg   config.Config
	timer *timer.Timer
	log   *history.Log
	pal   theme.Palette
	// fire delivers OS alerts; tests replace it so nothing is executed.
	fire func(ctx context.Context, o alert.Options, k alert.Kind, title, body string)

	task     string
	auto     bool
	hasColor bool // the terminal shows color; false under NO_COLOR or a dumb TERM

	width, height int

	screen  screen
	editing bool
	input   textinput.Model

	// waiting is set when a phase ran to completion and the next one sits in
	// Idle until the user starts it; finished is the phase that completed.
	waiting  bool
	finished timer.Phase

	toast      string // brief notice, cleared by the first tick after toastUntil
	toastUntil time.Time
	status     string // lasting problem report, e.g. a failed history write

	stats    history.Stats
	statsErr error
}

// newModel builds the initial model. It fails only for an unknown theme.
func newModel(o Options) (*model, error) {
	c := o.Config
	pal, ok := theme.Get(c.Theme)
	if !ok {
		return nil, fmt.Errorf("unknown theme %q (available: %s)", c.Theme, strings.Join(theme.Names(), ", "))
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	m := &model{
		now: now,
		cfg: c,
		timer: timer.New(timer.Durations{
			Focus:     c.Focus.Duration,
			Short:     c.ShortBreak.Duration,
			Long:      c.LongBreak.Duration,
			LongEvery: c.LongBreakEvery,
		}, now),
		log:   o.Log,
		pal:   pal,
		fire:  alert.Fire,
		task:  cleanTask(o.Task),
		auto:  c.AutoContinue,
		input: textinput.New(),
	}
	m.input.Prompt = ""
	m.input.Placeholder = "what are you working on?"
	m.input.CharLimit = taskLimit
	m.styleInput()
	return m, nil
}

// Init starts the tick loop.
func (m *model) Init() tea.Cmd { return tick() }

// Update handles one message.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizeInput()
	case tea.ColorProfileMsg:
		// Convert returns nil for profiles without color.
		m.hasColor = msg.Profile.Convert(m.pal.Base) != nil
	case tickMsg:
		return m, tea.Batch(m.advance(), tick())
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		if m.editing {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// advance expires the toast and finishes the phase once its time is up.
func (m *model) advance() tea.Cmd {
	if m.toast != "" && !m.now().Before(m.toastUntil) {
		m.toast = ""
	}
	if m.timer.Tick() {
		return m.phaseDone()
	}
	return nil
}

// key handles a key press.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	if m.editing {
		return m.editKey(msg)
	}
	if k == "q" {
		return tea.Quit
	}
	// A phase that ended since the last tick is finished first, and the key
	// is dropped, so it never pauses or resets a phase that is already over.
	if m.timer.Tick() {
		return m.phaseDone()
	}

	switch m.screen {
	case screenHelp:
		m.screen = screenTimer // any key closes help
		return nil
	case screenStats:
		if k == "tab" || k == "esc" {
			m.screen = screenTimer
		}
		return nil
	}

	switch k {
	case "space":
		m.waiting = false
		m.timer.Toggle()
	case "s":
		m.waiting = false
		m.timer.Skip()
	case "r":
		m.timer.Reset()
	case "t":
		return m.startEdit()
	case "a":
		m.auto = !m.auto
		m.flash("auto-continue " + onOff(m.auto))
	case "T":
		m.pal, _ = theme.Get(theme.Next(m.pal.Name))
		m.styleInput()
		m.flash(fmt.Sprintf("theme %s · keep it with -theme %s or in the config", m.pal.Name, m.pal.Name))
	case "tab":
		m.loadStats()
		m.screen = screenStats
	case "?":
		m.screen = screenHelp
	}
	return nil
}

// editKey routes a key to the task input: enter saves, esc cancels, and
// everything else edits.
func (m *model) editKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.task = cleanTask(m.input.Value())
		m.stopEdit()
		return nil
	case "esc":
		m.stopEdit()
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// startEdit opens the task input on the current label.
func (m *model) startEdit() tea.Cmd {
	m.editing = true
	m.styleInput()
	m.sizeInput()
	m.input.SetValue(m.task)
	m.input.CursorEnd()
	return m.input.Focus()
}

// stopEdit closes the task input.
func (m *model) stopEdit() {
	m.editing = false
	m.input.Blur()
}

// styleInput colors the task input from the palette and the phase accent.
// The cursor does not blink, so the input needs no blink messages.
func (m *model) styleInput() {
	s := textinput.DefaultDarkStyles()
	s.Focused.Text = fg(m.pal.Text)
	s.Focused.Placeholder = fg(m.pal.Muted)
	s.Blurred = s.Focused
	s.Cursor.Color = m.accent()[0]
	s.Cursor.Blink = false
	m.input.SetStyles(s)
}

// sizeInput fits the task input to the window: beside the session dots in
// the full layout, on a line of its own in the compact one.
func (m *model) sizeInput() {
	w := m.width - 32
	if !m.fits() {
		w = m.width - 4
	}
	m.input.SetWidth(min(max(w, 8), 40))
	m.input.SetCursor(m.input.Position()) // recompute the visible part
}

// phaseDone handles a phase that ran to completion: it records a focus
// phase, alerts, and loads the next phase, starting it with auto-continue.
func (m *model) phaseDone() tea.Cmd {
	done := m.timer.Phase()
	if done == timer.Focus {
		m.record(history.Entry{
			Start:    m.timer.StartedAt(),
			End:      m.now(),
			Kind:     history.KindFocus,
			Duration: m.timer.Duration(),
			Task:     m.task,
		})
	}
	m.timer.Next()
	cmd := m.alerts(done)
	if m.auto {
		m.timer.Start()
	}
	m.waiting = !m.auto
	m.finished = done
	if m.screen == screenStats {
		m.loadStats()
	}
	return cmd
}

// record appends e to the history log. A failure is shown in the status
// line and never stops the timer.
func (m *model) record(e history.Entry) {
	if m.log == nil {
		return
	}
	if err := m.log.Append(e); err != nil {
		m.status = "✗ " + err.Error()
		return
	}
	m.status = ""
}

// alerts returns the enabled phase-end alerts as commands: the OS alerts run
// in the background and the bell goes through the renderer. It reads the
// already loaded next phase.
func (m *model) alerts(done timer.Phase) tea.Cmd {
	var cmds []tea.Cmd
	o := alert.Options{Notify: m.cfg.Alerts.Notify, Sound: m.cfg.Alerts.Sound}
	if o.Notify || o.Sound {
		kind := alert.BreakDone
		if done == timer.Focus {
			kind = alert.FocusDone
		}
		title, body := notice(done, m.timer.Phase(), m.timer.Duration(), m.task)
		fire := m.fire
		cmds = append(cmds, func() tea.Msg {
			fire(context.Background(), o, kind, title, body)
			return nil
		})
	}
	if m.cfg.Alerts.Bell {
		cmds = append(cmds, tea.Raw("\a"))
	}
	return tea.Batch(cmds...)
}

// notice returns the notification text for a completed phase, given the
// phase that follows it.
func notice(done, next timer.Phase, nextDur time.Duration, task string) (title, body string) {
	if done == timer.Focus {
		title = "Focus complete 🍅"
		body = capitalize(next.String()) + " · " + compact(nextDur)
		if task != "" {
			body = "Done: " + task + " · " + body
		}
		return title, body
	}
	title = "Break over"
	body = "Back to focus · " + compact(nextDur)
	if task != "" {
		body += " · " + task
	}
	return title, body
}

// flash shows a toast.
func (m *model) flash(s string) {
	m.toast = s
	m.toastUntil = m.now().Add(toastFor)
}

// loadStats reads the history for the stats screen.
func (m *model) loadStats() {
	m.stats, m.statsErr = history.Stats{}, nil
	if m.log == nil {
		return
	}
	entries, err := m.log.All()
	if err != nil {
		m.statsErr = err
		return
	}
	m.stats = history.Compute(entries, m.now())
}

// onOff labels a toggle.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// capitalize upper-cases the first letter of an ASCII label.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// compact formats d like the config file does: "25m", "1h30m", "90s" → "1m30s".
func compact(d time.Duration) string {
	b, _ := config.Duration{Duration: d}.MarshalText()
	return string(b)
}
