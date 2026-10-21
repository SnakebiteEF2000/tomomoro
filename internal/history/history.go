// Package history records completed focus phases as an append-only JSONL log
// and derives statistics from it.
//
// One line per entry, e.g.
//
//	{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s","task":"write report"}
//
// Timestamps are RFC 3339 with the local UTC offset. Day boundaries for
// statistics are local midnight in now's location.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Kind labels an entry. Only focus phases are recorded today; the field
// exists so the format can grow without a migration.
type Kind string

const KindFocus Kind = "focus"

// Entry is one completed phase.
type Entry struct {
	Start    time.Time
	End      time.Time
	Kind     Kind
	Duration time.Duration // planned phase length, serialized as a Go duration string
	Task     string        // optional, omitted from JSON when empty
}

// wireEntry is the on-disk shape of an Entry. Field order fixes key order.
type wireEntry struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Kind     Kind   `json:"kind"`
	Duration string `json:"duration"`
	Task     string `json:"task,omitempty"`
}

// toWire formats e for disk, with second-precision RFC 3339 timestamps in
// each time's own offset.
func toWire(e Entry) wireEntry {
	return wireEntry{
		Start:    e.Start.Format(time.RFC3339),
		End:      e.End.Format(time.RFC3339),
		Kind:     e.Kind,
		Duration: e.Duration.String(),
		Task:     e.Task,
	}
}

// fromWire parses w, failing if a timestamp or the duration is invalid.
func fromWire(w wireEntry) (Entry, error) {
	start, err := time.Parse(time.RFC3339, w.Start)
	if err != nil {
		return Entry{}, err
	}
	end, err := time.Parse(time.RFC3339, w.End)
	if err != nil {
		return Entry{}, err
	}
	d, err := time.ParseDuration(w.Duration)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Start: start, End: end, Kind: w.Kind, Duration: d, Task: w.Task}, nil
}

// DefaultPath returns $XDG_DATA_HOME/tomomoro/history.jsonl, else
// ~/.local/share/tomomoro/history.jsonl (on macOS too).
func DefaultPath() string {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "tomomoro", "history.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No usable home: fall back to a path relative to the working directory.
		home = "."
	}
	return filepath.Join(home, ".local", "share", "tomomoro", "history.jsonl")
}

// Log is a history file. It holds no open handle between calls.
type Log struct {
	path string
}

// Open returns a Log for path. It performs no I/O.
func Open(path string) *Log { return &Log{path: path} }

// Path returns the file path.
func (l *Log) Path() string { return l.path }

// Append writes e as one JSON line, creating parent directories and the file
// as needed (0o755 / 0o644). Each call opens with O_APPEND and closes.
func (l *Log) Append(e Entry) (err error) {
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(toWire(e)); err != nil { // Encode appends the newline
		return fmt.Errorf("history: encode entry: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("history: %w", cerr)
		}
	}()
	if _, err := f.Write(line.Bytes()); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

// maxLine bounds a single history line; real entries are a few hundred bytes.
const maxLine = 1 << 20

// All reads every entry in file order. A missing file returns nil, nil.
// Malformed lines are skipped, not fatal.
func (l *Log) All() ([]Entry, error) {
	f, err := os.Open(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var w wireEntry
		if err := json.Unmarshal(line, &w); err != nil {
			continue
		}
		e, err := fromWire(w)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("history: read %s: %w", l.path, err)
	}
	return entries, nil
}

// Day is one calendar day's totals.
type Day struct {
	Date  time.Time // local midnight
	Count int
	Focus time.Duration
}

// Stats summarizes focus entries relative to now.
type Stats struct {
	TodayCount int
	TodayFocus time.Duration
	// Streak is the number of consecutive days with at least one focus entry,
	// ending today — or ending yesterday if today has none yet, so a streak
	// is not shown as broken until the day is over.
	Streak     int
	Week       [7]Day // the last 7 days, oldest first; Week[6] is today
	TotalCount int
	TotalFocus time.Duration
}

// civilDate is a calendar date, used as a map key where time.Time (which
// carries a Location pointer) would compare unreliably.
type civilDate struct {
	year  int
	month time.Month
	day   int
}

// dateOf returns t's calendar date in loc.
func dateOf(t time.Time, loc *time.Location) civilDate {
	y, m, d := t.In(loc).Date()
	return civilDate{y, m, d}
}

// addDays returns the date n calendar days after c. The arithmetic runs in
// UTC, which has no DST, so it never skips or repeats a day.
func (c civilDate) addDays(n int) civilDate {
	return dateOf(time.Date(c.year, c.month, c.day+n, 0, 0, 0, 0, time.UTC), time.UTC)
}

// start returns the first instant of c in loc: local midnight, or the end of
// the DST gap on the rare days where midnight does not exist (time.Date then
// normalizes back into the previous day).
func (c civilDate) start(loc *time.Location) time.Time {
	t := time.Date(c.year, c.month, c.day, 0, 0, 0, 0, loc)
	if dateOf(t, loc) != c {
		_, t = t.ZoneBounds()
	}
	return t
}

// Compute derives Stats from entries (any order) relative to now. Only
// KindFocus entries count; an entry belongs to the local day of its End.
func Compute(entries []Entry, now time.Time) Stats {
	loc := now.Location()
	var s Stats
	days := make(map[civilDate]Day)
	for _, e := range entries {
		if e.Kind != KindFocus {
			continue
		}
		s.TotalCount++
		s.TotalFocus += e.Duration
		key := dateOf(e.End, loc)
		d := days[key]
		d.Count++
		d.Focus += e.Duration
		days[key] = d
	}

	today := dateOf(now, loc)
	for i := range s.Week {
		date := today.addDays(i - (len(s.Week) - 1))
		day := days[date]
		day.Date = date.start(loc)
		s.Week[i] = day
	}
	s.TodayCount = days[today].Count
	s.TodayFocus = days[today].Focus

	cur := today
	if s.TodayCount == 0 {
		cur = cur.addDays(-1)
	}
	for days[cur].Count > 0 {
		s.Streak++
		cur = cur.addDays(-1)
	}
	return s
}
