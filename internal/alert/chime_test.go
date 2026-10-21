package alert

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestChimePlayerOrder(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		tools   []string
		want    string
		wantErr error
	}{
		{name: "darwin afplay", goos: "darwin", tools: []string{"afplay"}, want: "afplay"},
		{name: "linux prefers pw-play", goos: "linux", tools: []string{"aplay", "paplay", "pw-play"}, want: "pw-play"},
		{name: "linux then paplay", goos: "linux", tools: []string{"aplay", "paplay"}, want: "paplay"},
		{name: "linux then aplay", goos: "linux", tools: []string{"aplay"}, want: "aplay"},
		{name: "linux without player", goos: "linux", tools: []string{"afplay"}, wantErr: exec.ErrNotFound},
		{name: "darwin without afplay", goos: "darwin", wantErr: exec.ErrNotFound},
		{name: "windows", goos: "windows", tools: []string{"afplay", "aplay"}, wantErr: errors.ErrUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recorder
			fakeOS(t, tt.goos, tt.tools, r.run)

			err := Chime(t.Context(), FocusDone)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Chime error = %v, want %v", err, tt.wantErr)
				}
				if len(r.calls) != 0 {
					t.Errorf("ran %v despite the error", r.calls)
				}
				if _, err := os.Stat(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "tomomoro")); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("cache dir exists after a failed Chime (stat: %v)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Chime: %v", err)
			}
			wantPath := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "tomomoro", "chime-focus-v1.wav")
			want := []call{{tt.want, []string{wantPath}}}
			if !slices.EqualFunc(r.calls, want, func(a, b call) bool {
				return a.name == b.name && slices.Equal(a.args, b.args)
			}) {
				t.Errorf("ran %v, want %v", r.calls, want)
			}
		})
	}
}

func TestChimeCacheWrittenOnceAndReused(t *testing.T) {
	var r recorder
	fakeOS(t, "darwin", []string{"afplay"}, r.run)
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "tomomoro")

	for _, k := range kinds {
		if err := Chime(t.Context(), k); err != nil {
			t.Fatalf("Chime(%s): %v", focusOrBreak(k), err)
		}
	}
	for _, k := range kinds {
		path := filepath.Join(dir, "chime-"+focusOrBreak(k)+"-v1.wav")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cached chime: %v", err)
		}
		if !bytes.Equal(got, chimeWAV(t, k)) {
			t.Errorf("%s does not hold the synthesized chime", path)
		}
	}

	// A second Chime must play the cached file as is, not rewrite it.
	focus := filepath.Join(dir, "chime-focus-v1.wav")
	marker := []byte("cached")
	if err := os.WriteFile(focus, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Chime(t.Context(), FocusDone); err != nil {
		t.Fatalf("second Chime: %v", err)
	}
	if got, _ := os.ReadFile(focus); !bytes.Equal(got, marker) {
		t.Error("second Chime rewrote the cached file")
	}
	if last := r.calls[len(r.calls)-1]; !slices.Equal(last.args, []string{focus}) {
		t.Errorf("second Chime played %q, want %q", last.args, focus)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"chime-break-v1.wav", "chime-focus-v1.wav"}; !slices.Equal(names, want) {
		t.Errorf("cache dir holds %q, want only %q (no temp files)", names, want)
	}
}

func TestChimeUnknownKind(t *testing.T) {
	var r recorder
	fakeOS(t, "darwin", []string{"afplay"}, r.run)
	if err := Chime(t.Context(), Kind(42)); err == nil {
		t.Error("Chime(42) succeeded, want an error")
	}
	if len(r.calls) != 0 {
		t.Errorf("ran %v for an unknown kind", r.calls)
	}
}

func TestCacheDir(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	tests := []struct {
		name, xdg, want string
	}{
		{"XDG_CACHE_HOME", xdg, filepath.Join(xdg, "tomomoro")},
		{"unset falls back to ~/.cache", "", filepath.Join(home, ".cache", "tomomoro")},
		{"relative is ignored", "relative/cache", filepath.Join(home, ".cache", "tomomoro")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", tt.xdg)
			got, err := cacheDir()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("cacheDir() = %s, want %s", got, tt.want)
			}
		})
	}
}
