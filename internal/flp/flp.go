package flp

import (
	"github.com/alengaa/ptx-to-flp/internal/ptx"

	_ "embed"
	"encoding/binary"
	"errors"
	"math"
)

// FL Studio accepts tempos from 10 to 522 BPM (v11+).
const (
	flMinBPM = 10.0
	flMaxBPM = 522.0
	flPPQ    = 96
)

func flText(id byte, s string) []byte {
	b := append([]byte(s), 0)
	out := []byte{id}
	n := len(b)
	for {
		c := byte(n & 0x7f)
		n >>= 7
		if n > 0 {
			out = append(out, c|0x80)
		} else {
			out = append(out, c)
			break
		}
	}
	return append(out, b...)
}

func flDword(id byte, v uint32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{id}, v)
}

// ClampFLTempo limits a tempo to the range FL Studio supports.
func ClampFLTempo(bpm float64) (float64, bool) {
	if bpm < flMinBPM {
		return flMinBPM, true
	}
	if bpm > flMaxBPM {
		return flMaxBPM, true
	}
	return bpm, false
}

// BuildFLP creates a minimal, empty FL Studio project with the given tempo.
func BuildFLP(bpm float64) []byte {
	bpm, _ = ClampFLTempo(bpm)
	var ev []byte
	ev = append(ev, flText(199, "20.8.4.2576")...) // FL version
	ev = append(ev, flDword(159, 2576)...)         // FL build
	ev = append(ev, flDword(156, uint32(math.Round(bpm*1000)))...)

	out := []byte("FLhd")
	out = binary.LittleEndian.AppendUint32(out, 6)
	out = binary.LittleEndian.AppendUint16(out, 0)     // format: project
	out = binary.LittleEndian.AppendUint16(out, 0)     // channel count
	out = binary.LittleEndian.AppendUint16(out, flPPQ) // ppq
	out = append(out, "FLdt"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(ev)))
	return append(out, ev...)
}

// ---- Tempo automation -------------------------------------------------------
//
// The automation clip is built from a template project saved by FL Studio 26.1.6
// that contains one automation clip linked to the project Tempo control. Only the
// tempo, the clip's points, its min/max range and its playlist length are patched.

//go:embed assets/template.flp
var flpTemplate []byte

const (
	tplTempoOff    = 150  // event 156: project tempo (u32, BPM*1000)
	tplLevelsOff   = 459  // event 219: automation clip Min/Max knobs
	tplPlaylistOff = 1369 // event 233: playlist item for the clip
	tplLinkOff     = 255  // event 216: automation link record (holds the tempo)
	tplAutoOff     = 704  // event 234: automation points
	tplAutoLen     = 229
	tplAutoHdr     = 3 // id + 2-byte length
	tplPoints      = 4 // points in the template clip
)

func f32(v float64) float64 { return float64(float32(v)) }

type autoPoint struct {
	delta float64 // beats since previous point
	value float64 // 0..1 within the clip's Min..Max range
	kind  uint32  // 0 for the first point, pointKind for the rest (as FL writes them)
}

// pointKind is the last 4 bytes of every point after the first: 00 00 00 02 on disk.
const pointKind = 0x02000000

// ShiftToStart makes the song begin at the given 1-based bar (4/4): events before
// it are dropped, the tempo in effect there becomes the first event at position 0,
// and later events move earlier by that many bars.
func ShiftToStart(events []ptx.TempoEvent, startBar int) []ptx.TempoEvent {
	if startBar <= 1 || len(events) == 0 {
		return events
	}
	cut := uint64(startBar-1) * 4 * ptx.TicksPerBeat
	cur := events[0].BPM
	var out []ptx.TempoEvent
	for _, e := range events {
		if e.Ticks <= cut {
			cur = e.BPM
			continue
		}
		out = append(out, ptx.TempoEvent{Ticks: e.Ticks - cut, BPM: e.BPM})
	}
	return append([]ptx.TempoEvent{{Ticks: 0, BPM: cur}}, out...)
}

func varLen(n int) []byte {
	var out []byte
	for {
		c := byte(n & 0x7f)
		n >>= 7
		if n > 0 {
			out = append(out, c|0x80)
		} else {
			return append(out, c)
		}
	}
}

// buildAutomationEvent re-creates event 234 from the template's header and tail.
func buildAutomationEvent(tpl []byte, pts []autoPoint) []byte {
	payload := tpl[tplAutoOff+tplAutoHdr : tplAutoOff+tplAutoHdr+tplAutoLen]
	head := append([]byte{}, payload[:21]...)
	binary.LittleEndian.PutUint32(head[17:], uint32(len(pts)))
	tail := payload[21+24*tplPoints:]
	body := head
	for _, p := range pts {
		body = binary.LittleEndian.AppendUint64(body, math.Float64bits(p.delta))
		body = binary.LittleEndian.AppendUint64(body, math.Float64bits(p.value))
		body = binary.LittleEndian.AppendUint32(body, math.Float32bits(0)) // tension
		body = binary.LittleEndian.AppendUint32(body, p.kind)
	}
	body = append(body, tail...)
	ev := append([]byte{234}, varLen(len(body))...)
	return append(ev, body...)
}

// AutomationPlan describes the clip derived from the tempo events.
type AutomationPlan struct {
	LoBPM, HiBPM float64 // BPM range the clip's 0..1 values span
	LengthBeats  float64
	Points       []autoPoint
}

// PlanAutomation turns tempo events into automation points (step changes).
func PlanAutomation(events []ptx.TempoEvent) AutomationPlan {
	minB, maxB := events[0].BPM, events[0].BPM
	for _, e := range events {
		minB = math.Min(minB, e.BPM)
		maxB = math.Max(maxB, e.BPM)
	}
	lo := math.Max(flMinBPM, math.Floor(minB/10)*10-10)
	hi := math.Min(flMaxBPM, math.Ceil(maxB/10)*10+10)
	val := func(b float64) float64 { return f32((b - lo) / (hi - lo)) }

	plan := AutomationPlan{LoBPM: lo, HiBPM: hi}
	pts := []autoPoint{{0, val(events[0].BPM), 0}}
	prevPos, prevBPM := 0.0, events[0].BPM
	for _, e := range events[1:] {
		if e.BPM == prevBPM {
			continue
		}
		pos := e.Beats()
		pts = append(pts, autoPoint{f32(pos - prevPos), val(prevBPM), pointKind}) // hold until the change
		pts = append(pts, autoPoint{0, val(e.BPM), pointKind})                    // jump
		prevPos, prevBPM = pos, e.BPM
	}
	last := events[len(events)-1].Beats()
	plan.LengthBeats = (math.Floor(last/4) + 4) * 4 // 4 bars after the last change
	pts = append(pts, autoPoint{f32(plan.LengthBeats - prevPos), val(prevBPM), pointKind})
	plan.Points = pts
	return plan
}

// BuildFLPWithAutomation creates a project with the first tempo as project tempo
// plus an automation clip (playlist track 1, from bar 1) linked to Tempo.
func BuildFLPWithAutomation(events []ptx.TempoEvent) ([]byte, AutomationPlan, error) {
	t := flpTemplate
	if len(t) < tplAutoOff+tplAutoHdr+tplAutoLen || t[tplTempoOff] != 156 || t[tplLevelsOff] != 219 ||
		t[tplPlaylistOff] != 233 || t[tplLinkOff] != 216 || t[tplAutoOff] != 234 {
		return nil, AutomationPlan{}, errors.New("internal template is damaged")
	}
	plan := PlanAutomation(events)
	out := append([]byte{}, t...) // fixed-size patches first, at the template's offsets

	bpm, _ := ClampFLTempo(events[0].BPM)
	binary.LittleEndian.PutUint32(out[tplTempoOff+1:], uint32(math.Round(bpm*1000)))
	// Min/Max knobs: 25 units per BPM above 10 BPM.
	binary.LittleEndian.PutUint32(out[tplLevelsOff+2:], uint32(math.Round((plan.LoBPM-10)*25)))
	binary.LittleEndian.PutUint32(out[tplLevelsOff+6:], uint32(math.Round((plan.HiBPM-10)*25)))
	// Playlist item length in ticks and end offset in beats.
	binary.LittleEndian.PutUint32(out[tplPlaylistOff+2+8:], uint32(plan.LengthBeats*flPPQ))
	binary.LittleEndian.PutUint32(out[tplPlaylistOff+2+28:], math.Float32bits(float32(plan.LengthBeats)))

	// The linked-control record (event 216) holds the tempo value too.
	binary.LittleEndian.PutUint32(out[tplLinkOff+2+8:], uint32(math.Round(bpm*1000)))

	// Splice in the rebuilt points event (its size depends on the point count).
	rest := append([]byte{}, out[tplAutoOff+tplAutoHdr+tplAutoLen:]...)
	out = append(out[:tplAutoOff], buildAutomationEvent(t, plan.Points)...)
	out = append(out, rest...)

	// FLdt chunk length
	binary.LittleEndian.PutUint32(out[18:], uint32(len(out)-22))
	return out, plan, nil
}
