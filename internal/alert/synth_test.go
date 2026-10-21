package alert

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

var kinds = []Kind{FocusDone, BreakDone}

// wavHeader is the canonical 44-byte header of a PCM WAV file.
type wavHeader struct {
	RIFF       [4]byte
	RIFFSize   uint32
	WAVE       [4]byte
	Fmt        [4]byte
	FmtSize    uint32
	Format     uint16
	Channels   uint16
	SampleRate uint32
	ByteRate   uint32
	BlockAlign uint16
	Bits       uint16
	Data       [4]byte
	DataSize   uint32
}

// chimeWAV synthesizes the WAV file for k.
func chimeWAV(t *testing.T, k Kind) []byte {
	t.Helper()
	v, err := voiceFor(k)
	if err != nil {
		t.Fatal(err)
	}
	return encodeWAV(v.render())
}

// decode splits a 16-bit mono WAV file into its header and samples.
func decode(t *testing.T, wav []byte) (wavHeader, []int16) {
	t.Helper()
	var h wavHeader
	r := bytes.NewReader(wav)
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil {
		t.Fatalf("read header: %v", err)
	}
	samples := make([]int16, r.Len()/2)
	if err := binary.Read(r, binary.LittleEndian, samples); err != nil {
		t.Fatalf("read samples: %v", err)
	}
	return h, samples
}

func TestWAVHeader(t *testing.T) {
	for _, k := range kinds {
		t.Run(focusOrBreak(k), func(t *testing.T) {
			wav := chimeWAV(t, k)
			h, _ := decode(t, wav)

			checks := []struct {
				field     string
				got, want any
			}{
				{"RIFF tag", string(h.RIFF[:]), "RIFF"},
				{"RIFF size", h.RIFFSize, uint32(len(wav) - 8)},
				{"WAVE tag", string(h.WAVE[:]), "WAVE"},
				{"fmt tag", string(h.Fmt[:]), "fmt "},
				{"fmt size", h.FmtSize, uint32(16)},
				{"format", h.Format, uint16(1)},
				{"channels", h.Channels, uint16(1)},
				{"sample rate", h.SampleRate, uint32(44100)},
				{"byte rate", h.ByteRate, uint32(44100 * 2)},
				{"block align", h.BlockAlign, uint16(2)},
				{"bits per sample", h.Bits, uint16(16)},
				{"data tag", string(h.Data[:]), "data"},
				{"data size", h.DataSize, uint32(len(wav) - 44)},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
				}
			}
		})
	}
}

func TestChimeShape(t *testing.T) {
	for _, k := range kinds {
		t.Run(focusOrBreak(k), func(t *testing.T) {
			_, s := decode(t, chimeWAV(t, k))

			dur := time.Duration(len(s)) * time.Second / sampleRate
			if dur < 500*time.Millisecond || dur >= 1500*time.Millisecond {
				t.Errorf("duration = %v, want at least 0.5s and under 1.5s", dur)
			}

			var peak float64
			for _, v := range s {
				peak = max(peak, math.Abs(float64(v))/math.MaxInt16)
			}
			const lsb = 1.0 / math.MaxInt16 // rounding may add half of one
			if peak < 0.5 || peak > 0.8+lsb {
				t.Errorf("peak = %.4f FS, want within [0.5, 0.8]", peak)
			}

			if s[0] != 0 {
				t.Errorf("first sample = %d, want 0 (no click on start)", s[0])
			}
			if last := s[len(s)-1]; last != 0 {
				t.Errorf("last sample = %d, want 0 (no click on stop)", last)
			}
			tail := s[len(s)-sampleRate/1000:] // last millisecond
			for i, v := range tail {
				if math.Abs(float64(v)) > 0.002*math.MaxInt16 {
					t.Errorf("tail sample %d = %d, want ≈0", len(s)-len(tail)+i, v)
					break
				}
			}
		})
	}
}

func TestChimeKindsDiffer(t *testing.T) {
	if bytes.Equal(chimeWAV(t, FocusDone), chimeWAV(t, BreakDone)) {
		t.Error("FocusDone and BreakDone chimes are identical")
	}
}

func TestFocusIsThreeNotesBreakIsTwo(t *testing.T) {
	if n := len(focusVoice.notes); n != 3 {
		t.Errorf("focus chime has %d notes, want 3", n)
	}
	if n := len(breakVoice.notes); n != 2 {
		t.Errorf("break chime has %d notes, want 2", n)
	}
	for i := 1; i < len(focusVoice.notes); i++ {
		if focusVoice.notes[i].freq <= focusVoice.notes[i-1].freq {
			t.Errorf("focus arpeggio does not rise at note %d", i)
		}
	}
}

func TestVoiceForUnknownKind(t *testing.T) {
	if _, err := voiceFor(Kind(42)); err == nil {
		t.Error("voiceFor(42) succeeded, want an error")
	}
}

// focusOrBreak names k for subtests.
func focusOrBreak(k Kind) string {
	v, err := voiceFor(k)
	if err != nil {
		return "unknown"
	}
	return v.name
}
