package alert

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Format of every chime: 16-bit PCM, mono, 44.1 kHz.
const (
	sampleRate    = 44100
	channels      = 1
	bitsPerSample = 16
)

// Note frequencies in Hz (equal temperament, A4 = 440 Hz).
const (
	c5 = 523.25
	e5 = 659.26
	g5 = 783.99
)

// partial is one sine component of a struck tone.
type partial struct {
	ratio float64 // frequency as a multiple of the note's fundamental
	amp   float64 // amplitude relative to the fundamental
	decay float64 // decay time as a fraction of the voice's
}

// note is one strike within a chime.
type note struct {
	at   float64 // onset, seconds
	freq float64 // fundamental, Hz
	gain float64 // loudness relative to the other notes
}

// voice is a whole chime: which notes strike when, and how each one rings.
type voice struct {
	name     string // names the cache file
	notes    []note
	partials []partial // timbre shared by every note
	attack   float64   // attack time constant, seconds
	decay    float64   // the fundamental's decay time constant, seconds
	length   float64   // total duration, seconds
	fade     float64   // closing fade to silence, seconds
	peak     float64   // loudest sample, as a fraction of full scale
}

// focusVoice is a bright, rising C major arpeggio: the focus phase is over.
var focusVoice = voice{
	name: "focus",
	notes: []note{
		{at: 0, freq: c5, gain: 0.8},
		{at: 0.14, freq: e5, gain: 0.85},
		{at: 0.28, freq: g5, gain: 1},
	},
	// The inharmonic overtones of a struck bar (2.76, 5.40) give the bell
	// shimmer; they die away well before the fundamental.
	partials: []partial{
		{ratio: 1, amp: 1, decay: 1},
		{ratio: 2, amp: 0.35, decay: 0.55},
		{ratio: 2.76, amp: 0.18, decay: 0.35},
		{ratio: 5.40, amp: 0.08, decay: 0.18},
	},
	attack: 0.004,
	decay:  0.32,
	length: 1.3,
	fade:   0.08,
	peak:   0.8,
}

// breakVoice is a softer, falling two-note "ding-dong": back to work. A
// slower attack and only harmonic overtones make it gentler than focusVoice.
var breakVoice = voice{
	name: "break",
	notes: []note{
		{at: 0, freq: e5, gain: 0.9},
		{at: 0.32, freq: c5, gain: 1},
	},
	partials: []partial{
		{ratio: 1, amp: 1, decay: 1},
		{ratio: 2, amp: 0.25, decay: 0.5},
		{ratio: 3, amp: 0.08, decay: 0.3},
	},
	attack: 0.012,
	decay:  0.38,
	length: 1.25,
	fade:   0.1,
	peak:   0.7,
}

// voiceFor returns the chime for k.
func voiceFor(k Kind) (voice, error) {
	switch k {
	case FocusDone:
		return focusVoice, nil
	case BreakDone:
		return breakVoice, nil
	}
	return voice{}, fmt.Errorf("alert: unknown chime kind %d", int(k))
}

// render synthesizes v: each note's partials with a smooth attack and an
// exponential decay, summed, faded to exactly zero at the end so playback
// stops without a click, and scaled so the loudest sample is v.peak.
func (v voice) render() []int16 {
	n := int(v.length * sampleRate)
	mix := make([]float64, n)
	for _, nt := range v.notes {
		start := int(nt.at * sampleRate)
		for i := start; i < n; i++ {
			t := float64(i-start) / sampleRate
			var s float64
			for _, p := range v.partials {
				s += p.amp * math.Exp(-t/(v.decay*p.decay)) * math.Sin(2*math.Pi*nt.freq*p.ratio*t)
			}
			mix[i] += nt.gain * (1 - math.Exp(-t/v.attack)) * s
		}
	}

	fade := int(v.fade * sampleRate)
	for j := range fade {
		x := float64(j+1) / float64(fade) // reaches 1 on the last sample
		mix[n-fade+j] *= 0.5 * (1 + math.Cos(math.Pi*x))
	}

	var top float64
	for _, s := range mix {
		top = max(top, math.Abs(s))
	}
	scale := v.peak * math.MaxInt16 / top
	out := make([]int16, n)
	for i, s := range mix {
		out[i] = int16(math.Round(s * scale))
	}
	return out
}

// encodeWAV wraps samples in a canonical 44-byte RIFF/WAVE header.
func encodeWAV(samples []int16) []byte {
	const blockAlign = channels * bitsPerSample / 8
	dataLen := len(samples) * blockAlign
	le := binary.LittleEndian

	b := make([]byte, 0, 44+dataLen)
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, uint32(36+dataLen))
	b = append(b, "WAVE"...)

	b = append(b, "fmt "...)
	b = le.AppendUint32(b, 16) // fmt chunk size
	b = le.AppendUint16(b, 1)  // PCM
	b = le.AppendUint16(b, channels)
	b = le.AppendUint32(b, sampleRate)
	b = le.AppendUint32(b, sampleRate*blockAlign) // byte rate
	b = le.AppendUint16(b, blockAlign)
	b = le.AppendUint16(b, bitsPerSample)

	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(dataLen))
	for _, s := range samples {
		b = le.AppendUint16(b, uint16(s))
	}
	return b
}
