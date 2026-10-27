package flp

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/alengaa/ptx-to-flp/internal/ptx"
)

func TestBuildFLP(t *testing.T) {
	b := BuildFLP(93.5)
	if string(b[:4]) != "FLhd" || string(b[14:18]) != "FLdt" {
		t.Fatal("bad header")
	}
	if got := binary.LittleEndian.Uint32(b[18:]); int(got) != len(b)-22 {
		t.Fatalf("bad data length %d", got)
	}
}

func TestAutomationEventRoundTrip(t *testing.T) {
	// Rebuilding the template's own points must reproduce the template's event byte for byte.
	payload := flpTemplate[tplAutoOff+tplAutoHdr : tplAutoOff+tplAutoHdr+tplAutoLen]
	var pts []autoPoint
	for k := 0; k < tplPoints; k++ {
		o := 21 + 24*k
		pts = append(pts, autoPoint{
			math.Float64frombits(binary.LittleEndian.Uint64(payload[o:])),
			math.Float64frombits(binary.LittleEndian.Uint64(payload[o+8:])),
			binary.LittleEndian.Uint32(payload[o+20:]),
		})
	}
	got := buildAutomationEvent(flpTemplate, pts)
	want := flpTemplate[tplAutoOff : tplAutoOff+tplAutoHdr+tplAutoLen]
	if string(got) != string(want) {
		t.Fatal("rebuilt automation event differs from template")
	}
}

func TestBuildFLPWithAutomation(t *testing.T) {
	ev := []ptx.TempoEvent{{Ticks: 0, BPM: 140.3351}, {Ticks: 388 * ptx.TicksPerBeat, BPM: 93}}
	b, plan, err := BuildFLPWithAutomation(ev)
	if err != nil {
		t.Fatal(err)
	}
	if int(binary.LittleEndian.Uint32(b[18:])) != len(b)-22 {
		t.Fatal("bad FLdt length")
	}
	t.Logf("range %v-%v length %v beats, %d points", plan.LoBPM, plan.HiBPM, plan.LengthBeats, len(plan.Points))
	os.WriteFile(filepath.Join(os.TempDir(), "ptx_auto_out.flp"), b, 0o600)
}

func TestShiftToStart(t *testing.T) {
	ev := []ptx.TempoEvent{{Ticks: 0, BPM: 140.3351}, {Ticks: 388 * ptx.TicksPerBeat, BPM: 93}}
	got := ShiftToStart(ev, 36) // 35 bars = 140 beats
	if len(got) != 2 || got[0].Ticks != 0 || got[0].BPM != 140.3351 || got[1].Beats() != 248 || got[1].BPM != 93 {
		t.Fatalf("got %+v", got)
	}
	// starting after the change: the later tempo becomes the project tempo
	got = ShiftToStart(ev, 100)
	if len(got) != 1 || got[0].BPM != 93 || got[0].Ticks != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAutomationPointsDecode(t *testing.T) {
	ev := ShiftToStart([]ptx.TempoEvent{{Ticks: 0, BPM: 140.3351}, {Ticks: 388 * ptx.TicksPerBeat, BPM: 93}}, 36)
	b, plan, err := BuildFLPWithAutomation(ev)
	if err != nil {
		t.Fatal(err)
	}
	// locate event 234 by walking events
	pos := 22
	var payload []byte
	for pos < len(b) {
		id := b[pos]
		pos++
		switch {
		case id < 64:
			pos++
		case id < 128:
			pos += 2
		case id < 192:
			pos += 4
		default:
			n, s := 0, 0
			for {
				c := int(b[pos])
				pos++
				n |= (c & 0x7f) << s
				s += 7
				if c&0x80 == 0 {
					break
				}
			}
			if id == 234 && n > 100 && payload == nil {
				payload = b[pos : pos+n]
			}
			pos += n
		}
	}
	if payload == nil {
		t.Fatal("no automation event")
	}
	n := int(binary.LittleEndian.Uint32(payload[17:]))
	if n != len(plan.Points) {
		t.Fatalf("count %d want %d", n, len(plan.Points))
	}
	at := 0.0
	var vals []float64
	var ats []float64
	for k := 0; k < n; k++ {
		o := 21 + 24*k
		at += math.Float64frombits(binary.LittleEndian.Uint64(payload[o:]))
		v := math.Float64frombits(binary.LittleEndian.Uint64(payload[o+8:]))
		ats = append(ats, at)
		vals = append(vals, plan.LoBPM+v*(plan.HiBPM-plan.LoBPM))
		wantKind := uint32(0)
		if k > 0 {
			wantKind = 0x02000000
		}
		if got := binary.LittleEndian.Uint32(payload[o+20:]); got != wantKind {
			t.Fatalf("point %d kind %#x want %#x", k, got, wantKind)
		}
	}
	t.Logf("positions %v bpm %v", ats, vals)
	if ats[1] != 248 || ats[2] != 248 || math.Abs(vals[0]-140.3351) > 0.001 || math.Abs(vals[2]-93) > 0.001 {
		t.Fatalf("unexpected points %v %v", ats, vals)
	}
}
