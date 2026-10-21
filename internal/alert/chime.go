package alert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// chimeVersion is part of each cached file name. Bump it whenever synthesis
// changes so stale caches are not reused.
const chimeVersion = "v1"

// players lists the audio players to try on each OS, most preferred first.
// Each takes the WAV path as its only argument.
var players = map[string][]string{
	"darwin": {"afplay"},
	"linux":  {"pw-play", "paplay", "aplay"},
}

// Chime plays the sound for k and returns when playback ends. The WAV is
// synthesized in Go (sine tones with an envelope, under ~1.5s) and cached
// under the user cache dir ($XDG_CACHE_HOME or ~/.cache)/tomomoro/, written
// once per Kind and reused.
func Chime(ctx context.Context, k Kind) error {
	v, err := voiceFor(k)
	if err != nil {
		return err
	}
	player, err := findPlayer()
	if err != nil {
		return err
	}
	path, err := chimeFile(v)
	if err != nil {
		return err
	}
	return runCmd(ctx, player, path)
}

// findPlayer returns the first audio player for goos found on PATH.
func findPlayer() (string, error) {
	candidates, ok := players[goos]
	if !ok {
		return "", unsupported()
	}
	for _, name := range candidates {
		if _, err := lookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("alert: no audio player (tried %s): %w", strings.Join(candidates, ", "), exec.ErrNotFound)
}

// chimeFile returns the path of the cached WAV for v, synthesizing and writing
// it first if it is missing.
func chimeFile(v voice) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "chime-"+v.name+"-"+chimeVersion+".wav")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := writeAtomic(path, encodeWAV(v.render())); err != nil {
		return "", fmt.Errorf("alert: cache chime: %w", err)
	}
	return path, nil
}

// cacheDir returns $XDG_CACHE_HOME/tomomoro, else ~/.cache/tomomoro (on macOS
// too). A relative XDG_CACHE_HOME is ignored, as the XDG spec requires.
func cacheDir() (string, error) {
	if dir := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "tomomoro"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("alert: cache dir: %w", err)
	}
	return filepath.Join(home, ".cache", "tomomoro"), nil
}

// writeAtomic writes data to a temporary file beside path, then renames it
// into place, so a reader never sees a partial file.
func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
