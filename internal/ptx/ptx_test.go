package ptx

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func record(ticks uint64, bpm float64) []byte {
	r := make([]byte, 61)
	v := zeroTicks + ticks
	for i := 0; i < 5; i++ {
		r[i] = byte(v >> (8 * i))
	}
	binary.LittleEndian.PutUint64(r[10:], math.Float64bits(bpm))
	return r
}

// buildSynthetic assembles an encrypted PTX holding one tempo block built from
// the given records (layout per third-party notes; NOT from a real automated session).
func buildSynthetic(t *testing.T, events []TempoEvent) string {
	var payload []byte
	payload = append(payload, 0x28, 0x20)
	payload = append(payload, []byte("Tempo")...)
	payload = append(payload, make([]byte, 40)...)
	for _, e := range events {
		payload = append(payload, record(e.Ticks, e.BPM)...)
	}
	payload = append(payload, make([]byte, 8)...)
	plain := make([]byte, 0x14)
	copy(plain, "\x030010111100101")
	plain[0x12] = 0x05
	plain[0x13] = 0x3c
	plain = append(plain, make([]byte, 33)...)
	plain = append(plain, 0x5a, 0x28, 0x00)
	plain = binary.LittleEndian.AppendUint32(plain, uint32(len(payload)))
	plain = append(plain, payload...)
	plain = append(plain, make([]byte, 20)...)

	xd := genXorDelta(0x3c, 11, true)
	enc := make([]byte, len(plain))
	copy(enc, plain[:0x14])
	for i := 0x14; i < len(plain); i++ {
		enc[i] = plain[i] ^ byte(((i>>12)&0xff)*xd)
	}
	p := filepath.Join(t.TempDir(), "synthetic.ptx")
	if err := os.WriteFile(p, enc, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSyntheticAutomation(t *testing.T) {
	in := []TempoEvent{{0, 120}, {16 * TicksPerBeat, 60}, {32 * TicksPerBeat, 90.5}}
	r, err := ExtractTempo(buildSynthetic(t, in))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Events) != 3 {
		t.Fatalf("got %+v", r.Events)
	}
	for i := range in {
		if r.Events[i] != in[i] {
			t.Fatalf("event %d: got %+v want %+v", i, r.Events[i], in[i])
		}
	}
	out := r.Format()
	for _, want := range []string{"Tempo automation: YES - 3 tempo events", "5|1.000", "0:08.000", "0:24.000", "90.5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	t.Log("\n" + out)
}

func TestRampManyEvents(t *testing.T) {
	var in []TempoEvent
	for i := 0; i < 276; i++ {
		in = append(in, TempoEvent{uint64(i) * TicksPerBeat / 4, 90 + float64(i)*30/276})
	}
	r, err := ExtractTempo(buildSynthetic(t, in))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Events) != 276 {
		t.Fatalf("got %d events", len(r.Events))
	}
}

func TestRejectsNonPTX(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.ptx")
	os.WriteFile(p, []byte(strings.Repeat("hello world ", 10)), 0o600)
	if _, err := ExtractTempo(p); err == nil {
		t.Fatal("expected error")
	}
}
