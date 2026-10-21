package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "time/tzdata" // pin zone data so DST tests do not depend on the host
)

var cest = time.FixedZone("CEST", 2*60*60)

// at returns a time in loc with whole seconds.
func at(loc *time.Location, y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

// focus returns a completed focus entry of length dur ending at end.
func focus(end time.Time, dur time.Duration) Entry {
	return Entry{Start: end.Add(-dur), End: end, Kind: KindFocus, Duration: dur}
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"xdg set", data, filepath.Join(data, "tomomoro", "history.jsonl")},
		{"xdg empty", "", filepath.Join(home, ".local", "share", "tomomoro", "history.jsonl")},
		{"xdg relative is ignored", "rel/data", filepath.Join(home, ".local", "share", "tomomoro", "history.jsonl")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", tt.xdg)
			if got := DefaultPath(); got != tt.want {
				t.Errorf("DefaultPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never", "created.jsonl")
	l := Open(path)
	if got := l.Path(); got != path {
		t.Errorf("Path() = %q, want %q", got, path)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("Open performed I/O: stat parent err = %v", err)
	}
}

func TestAppendAllRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "history.jsonl")
	l := Open(path)
	want := []Entry{
		{
			Start: at(cest, 2026, 10, 21, 9, 0), End: at(cest, 2026, 10, 21, 9, 25),
			Kind: KindFocus, Duration: 25 * time.Minute, Task: `write "report" & <review> ✓`,
		},
		{
			Start: at(time.UTC, 2026, 10, 21, 8, 0), End: at(time.UTC, 2026, 10, 21, 8, 50),
			Kind: KindFocus, Duration: 50 * time.Minute,
		},
		{
			Start: at(cest, 2026, 10, 21, 10, 0), End: at(cest, 2026, 10, 21, 10, 5),
			Kind: Kind("break"), Duration: 5*time.Minute + 30*time.Second, Task: "stretch",
		},
	}
	for _, e := range want {
		if err := l.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := l.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("All returned %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.Start.Equal(w.Start) || !g.End.Equal(w.End) {
			t.Errorf("entry %d times = %v..%v, want %v..%v", i, g.Start, g.End, w.Start, w.End)
		}
		if _, off := g.End.Zone(); off != offsetOf(w.End) {
			t.Errorf("entry %d end offset = %d, want %d", i, off, offsetOf(w.End))
		}
		if g.Kind != w.Kind || g.Duration != w.Duration || g.Task != w.Task {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}
}

func offsetOf(t time.Time) int {
	_, off := t.Zone()
	return off
}

func TestAppendLineBytes(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{
			name: "with task",
			entry: Entry{
				Start: at(cest, 2026, 10, 21, 9, 0), End: at(cest, 2026, 10, 21, 9, 25),
				Kind: KindFocus, Duration: 25 * time.Minute, Task: "write report",
			},
			want: `{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s","task":"write report"}` + "\n",
		},
		{
			name: "empty task omitted",
			entry: Entry{
				Start: at(cest, 2026, 10, 21, 9, 0), End: at(cest, 2026, 10, 21, 9, 25),
				Kind: KindFocus, Duration: 25 * time.Minute,
			},
			want: `{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s"}` + "\n",
		},
		{
			name: "sub-second dropped, html not escaped",
			entry: Entry{
				Start: time.Date(2026, 10, 21, 9, 0, 0, 123456789, cest),
				End:   time.Date(2026, 10, 21, 9, 25, 0, 987654321, cest),
				Kind:  KindFocus, Duration: 25 * time.Minute, Task: "a & <b>",
			},
			want: `{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s","task":"a & <b>"}` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			if err := Open(path).Append(tt.entry); err != nil {
				t.Fatalf("Append: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("line =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestAllSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	content := `{"start":"2026-10-21T09:00:00+02:00","end":"2026-10-21T09:25:00+02:00","kind":"focus","duration":"25m0s","task":"first"}

not json at all
{"start":"2026-10-21T10:00:00+02:00","end":"2026-10-21T10:25:00+02:00","kind":"focus","duration":"soon"}
{"start":"2026-10-21T10:00:00+02:00","kind":"focus","duration":"25m0s"}
{"start":"yesterday","end":"2026-10-21T10:25:00+02:00","kind":"focus","duration":"25m0s"}

{"start":"2026-10-21T11:00:00+02:00","end":"2026-10-21T11:50:00+02:00","kind":"focus","duration":"50m0s"}` + "\r\n" +
		`{"start":"2026-10-21T12:00:00+02:00","end":"2026-10-21T12:25:00+02:00","kind":"focus","duration":"25m0s","task":"last, no newline"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Open(path).All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	wantTasks := []string{"first", "", "last, no newline"}
	wantDur := []time.Duration{25 * time.Minute, 50 * time.Minute, 25 * time.Minute}
	if len(got) != len(wantTasks) {
		t.Fatalf("All returned %d entries, want %d: %+v", len(got), len(wantTasks), got)
	}
	for i := range got {
		if got[i].Task != wantTasks[i] || got[i].Duration != wantDur[i] {
			t.Errorf("entry %d = %+v, want task %q duration %v", i, got[i], wantTasks[i], wantDur[i])
		}
	}
}

func TestAllMissingFile(t *testing.T) {
	for _, path := range []string{
		filepath.Join(t.TempDir(), "history.jsonl"),
		filepath.Join(t.TempDir(), "no", "such", "dir", "history.jsonl"),
	} {
		got, err := Open(path).All()
		if got != nil || err != nil {
			t.Errorf("All(%q) = %v, %v; want nil, nil", path, got, err)
		}
	}
}

func TestCompute(t *testing.T) {
	now := at(cest, 2026, 10, 21, 15, 0) // Wednesday
	day := func(d, h, min int) time.Time { return at(cest, 2026, 10, d, h, min) }
	m25, m50 := 25*time.Minute, 50*time.Minute

	tests := []struct {
		name       string
		now        time.Time
		entries    []Entry
		todayCount int
		todayFocus time.Duration
		streak     int
		week       [7]int // counts, oldest first
		totalCount int
		totalFocus time.Duration
	}{
		{
			name: "zero",
			now:  now,
		},
		{
			name:       "today counts and focus time",
			now:        now,
			entries:    []Entry{focus(day(21, 9, 25), m25), focus(day(21, 11, 0), m50), focus(day(20, 10, 0), m25)},
			todayCount: 2, todayFocus: 75 * time.Minute, streak: 2,
			week:       [7]int{0, 0, 0, 0, 0, 1, 2},
			totalCount: 3, totalFocus: 100 * time.Minute,
		},
		{
			name:       "week buckets, older entries only in totals",
			now:        now,
			entries:    []Entry{focus(day(14, 9, 0), m25), focus(day(15, 9, 0), m25), focus(day(17, 9, 0), m25), focus(day(17, 10, 0), m25), focus(day(19, 9, 0), m25), focus(day(21, 9, 0), m25)},
			todayCount: 1, todayFocus: m25, streak: 1,
			week:       [7]int{1, 0, 2, 0, 1, 0, 1},
			totalCount: 6, totalFocus: 6 * m25,
		},
		{
			name:       "streak ending today",
			now:        now,
			entries:    []Entry{focus(day(19, 9, 0), m25), focus(day(20, 9, 0), m25), focus(day(21, 9, 0), m25)},
			todayCount: 1, todayFocus: m25, streak: 3,
			week:       [7]int{0, 0, 0, 0, 1, 1, 1},
			totalCount: 3, totalFocus: 3 * m25,
		},
		{
			name:       "streak ending yesterday",
			now:        now,
			entries:    []Entry{focus(day(19, 9, 0), m25), focus(day(20, 9, 0), m25)},
			streak:     2,
			week:       [7]int{0, 0, 0, 0, 1, 1, 0},
			totalCount: 2, totalFocus: 2 * m25,
		},
		{
			name:       "gap yesterday breaks streak ending today",
			now:        now,
			entries:    []Entry{focus(day(18, 9, 0), m25), focus(day(19, 9, 0), m25), focus(day(21, 9, 0), m25)},
			todayCount: 1, todayFocus: m25, streak: 1,
			week:       [7]int{0, 0, 0, 1, 1, 0, 1},
			totalCount: 3, totalFocus: 3 * m25,
		},
		{
			name:       "no entry today or yesterday is no streak",
			now:        now,
			entries:    []Entry{focus(day(18, 9, 0), m25), focus(day(19, 9, 0), m25)},
			streak:     0,
			week:       [7]int{0, 0, 0, 1, 1, 0, 0},
			totalCount: 2, totalFocus: 2 * m25,
		},
		{
			name:       "streak longer than the week",
			now:        now,
			entries:    []Entry{focus(day(11, 9, 0), m25), focus(day(12, 9, 0), m25), focus(day(13, 9, 0), m25), focus(day(14, 9, 0), m25), focus(day(15, 9, 0), m25), focus(day(16, 9, 0), m25), focus(day(17, 9, 0), m25), focus(day(18, 9, 0), m25), focus(day(19, 9, 0), m25), focus(day(20, 9, 0), m25), focus(day(21, 9, 0), m25)},
			todayCount: 1, todayFocus: m25, streak: 11,
			week:       [7]int{1, 1, 1, 1, 1, 1, 1},
			totalCount: 11, totalFocus: 11 * m25,
		},
		{
			name: "entries around local midnight",
			now:  now,
			entries: []Entry{
				focus(time.Date(2026, 10, 20, 23, 59, 59, 0, cest), m25),
				focus(time.Date(2026, 10, 21, 0, 0, 0, 0, cest), m25),
				focus(time.Date(2026, 10, 21, 0, 0, 1, 0, cest), m25),
			},
			todayCount: 2, todayFocus: m50, streak: 2,
			week:       [7]int{0, 0, 0, 0, 0, 1, 2},
			totalCount: 3, totalFocus: 3 * m25,
		},
		{
			name: "end bucketed in now's zone, not the entry's",
			now:  at(time.UTC, 2026, 10, 21, 12, 0),
			// 00:30 +02:00 on the 21st is 22:30 UTC on the 20th.
			entries:    []Entry{focus(day(21, 0, 30), m25)},
			streak:     1,
			week:       [7]int{0, 0, 0, 0, 0, 1, 0},
			totalCount: 1, totalFocus: m25,
		},
		{
			name: "end decides the day, not start",
			now:  now,
			// Starts yesterday 23:50, ends today 00:15.
			entries:    []Entry{focus(day(21, 0, 15), m25)},
			todayCount: 1, todayFocus: m25, streak: 1,
			week:       [7]int{0, 0, 0, 0, 0, 0, 1},
			totalCount: 1, totalFocus: m25,
		},
		{
			name: "non-focus kinds ignored",
			now:  now,
			entries: []Entry{
				{Start: day(21, 9, 25), End: day(21, 9, 30), Kind: "short_break", Duration: 5 * time.Minute},
				{Start: day(20, 9, 25), End: day(20, 9, 30), Kind: "", Duration: 5 * time.Minute},
				{Start: day(20, 9, 0), End: day(20, 9, 25), Kind: "Focus", Duration: m25},
				focus(day(21, 9, 0), m25),
			},
			todayCount: 1, todayFocus: m25, streak: 1,
			week:       [7]int{0, 0, 0, 0, 0, 0, 1},
			totalCount: 1, totalFocus: m25,
		},
		{
			name: "unsorted input",
			now:  now,
			entries: []Entry{
				focus(day(20, 9, 0), m25),
				focus(day(21, 11, 0), m50),
				focus(day(15, 9, 0), m25),
				focus(day(19, 9, 0), m25),
				focus(day(21, 9, 0), m25),
			},
			todayCount: 2, todayFocus: 75 * time.Minute, streak: 3,
			week:       [7]int{1, 0, 0, 0, 1, 1, 2},
			totalCount: 5, totalFocus: 150 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Compute(tt.entries, tt.now)
			if s.TodayCount != tt.todayCount || s.TodayFocus != tt.todayFocus {
				t.Errorf("today = %d / %v, want %d / %v", s.TodayCount, s.TodayFocus, tt.todayCount, tt.todayFocus)
			}
			if s.Streak != tt.streak {
				t.Errorf("Streak = %d, want %d", s.Streak, tt.streak)
			}
			if s.TotalCount != tt.totalCount || s.TotalFocus != tt.totalFocus {
				t.Errorf("total = %d / %v, want %d / %v", s.TotalCount, s.TotalFocus, tt.totalCount, tt.totalFocus)
			}
			var counts [7]int
			for i, d := range s.Week {
				counts[i] = d.Count
			}
			if counts != tt.week {
				t.Errorf("week counts = %v, want %v", counts, tt.week)
			}
			if s.Week[6].Count != s.TodayCount || s.Week[6].Focus != s.TodayFocus {
				t.Errorf("Week[6] = %+v, disagrees with today %d / %v", s.Week[6], s.TodayCount, s.TodayFocus)
			}
			loc := tt.now.Location()
			y, m, d := tt.now.Date()
			for i, got := range s.Week {
				want := time.Date(y, m, d-6+i, 0, 0, 0, 0, loc)
				if !got.Date.Equal(want) || got.Date.Location() != loc {
					t.Errorf("Week[%d].Date = %v, want %v", i, got.Date, want)
				}
			}
		})
	}
}

// TestComputeDST checks that week buckets and streaks follow calendar days,
// not 24-hour steps, across DST transitions.
func TestComputeDST(t *testing.T) {
	berlin := mustLoad(t, "Europe/Berlin")
	santiago := mustLoad(t, "America/Santiago")

	tests := []struct {
		name  string
		loc   *time.Location
		first time.Time // Week[0] local midnight; following days via time.Date
		// gapStart overrides the expected first instant of a day whose
		// midnight does not exist.
		gapStart map[int]time.Time
	}{
		{name: "Berlin fall back (25h Sunday)", loc: berlin, first: at(berlin, 2026, 10, 22, 0, 0)},
		{name: "Berlin spring forward (23h Sunday)", loc: berlin, first: at(berlin, 2026, 3, 26, 0, 0)},
		{
			name: "Santiago spring forward at midnight", loc: santiago, first: at(santiago, 2026, 9, 2, 0, 0),
			gapStart: map[int]time.Time{4: at(santiago, 2026, 9, 6, 1, 0)}, // 00:00 → 01:00
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y, m, d := tt.first.Date()
			var entries []Entry
			for i := range 7 {
				// One entry just after the day's first hour, one late in the evening.
				entries = append(entries,
					focus(time.Date(y, m, d+i, 1, 30, 0, 0, tt.loc), 25*time.Minute),
					focus(time.Date(y, m, d+i, 23, 30, 0, 0, tt.loc), 25*time.Minute))
			}
			now := time.Date(y, m, d+6, 12, 0, 0, 0, tt.loc)

			s := Compute(entries, now)
			for i, got := range s.Week {
				want := time.Date(y, m, d+i, 0, 0, 0, 0, tt.loc)
				if g, ok := tt.gapStart[i]; ok {
					want = g
				}
				if !got.Date.Equal(want) {
					t.Errorf("Week[%d].Date = %v, want %v", i, got.Date, want)
				}
				if gy, gm, gd := got.Date.In(tt.loc).Date(); gy != want.Year() || gm != want.Month() || gd != want.Day() {
					t.Errorf("Week[%d].Date is on %d-%02d-%02d, want %s", i, gy, gm, gd, want.Format(time.DateOnly))
				}
				if got.Count != 2 || got.Focus != 50*time.Minute {
					t.Errorf("Week[%d] (%s) = %d / %v, want 2 / 50m", i, want.Format(time.DateOnly), got.Count, got.Focus)
				}
			}
			if s.Streak != 7 {
				t.Errorf("Streak = %d, want 7", s.Streak)
			}
			if s.TodayCount != 2 || s.TotalCount != 14 {
				t.Errorf("TodayCount, TotalCount = %d, %d; want 2, 14", s.TodayCount, s.TotalCount)
			}
		})
	}

	t.Run("Berlin repeated hour lands on its day", func(t *testing.T) {
		// 02:30 occurs twice on 2026-10-25: 00:30 UTC (CEST) and 01:30 UTC (CET).
		entries := []Entry{
			focus(time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC), 25*time.Minute),
			focus(time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), 25*time.Minute),
			focus(time.Date(2026, 10, 24, 21, 50, 0, 0, time.UTC), 25*time.Minute), // 23:50 CEST on the 24th
			focus(time.Date(2026, 10, 25, 23, 10, 0, 0, time.UTC), 25*time.Minute), // 00:10 CET on the 26th
		}
		now := at(berlin, 2026, 10, 26, 12, 0)
		s := Compute(entries, now)
		want := [7]int{0, 0, 0, 0, 1, 2, 1} // Oct 20..26
		var got [7]int
		for i, d := range s.Week {
			got[i] = d.Count
		}
		if got != want {
			t.Errorf("week counts = %v, want %v", got, want)
		}
		if s.Streak != 3 {
			t.Errorf("Streak = %d, want 3", s.Streak)
		}
	})
}
