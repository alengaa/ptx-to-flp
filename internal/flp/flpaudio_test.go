package flp

import (
	"bytes"
	"encoding/binary"
	"github.com/alengaa/ptx-to-flp/internal/ptx"
	"math"
	"testing"
)

func TestFLEventsRoundTrip(t *testing.T) {
	for name, b := range map[string][]byte{"audio_base": flpAudioBase, "template": flpTemplate} {
		h, ev, err := parseFLEvents(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(serializeFL(h, ev), b) {
			t.Fatalf("%s: round trip differs", name)
		}
	}
}

func decodeItems(t *testing.T, b []byte) (items [][]byte, chans map[int]string, pans map[int]uint32) {
	_, ev, err := parseFLEvents(b)
	if err != nil {
		t.Fatal(err)
	}
	chans, pans = map[int]string{}, map[int]uint32{}
	cur := -1
	for _, e := range ev {
		switch e.id {
		case 64:
			cur = int(binary.LittleEndian.Uint16(e.data))
		case 196:
			chans[cur] = decodeU16(e.data)
		case 219:
			if _, ok := chans[cur]; !ok {
				pans[cur] = binary.LittleEndian.Uint32(e.data)
			}
		case 233:
			for k := 0; k+88 <= len(e.data); k += 88 {
				items = append(items, e.data[k:k+88])
			}
		}
	}
	return
}

func decodeU16(d []byte) string {
	var r []rune
	for i := 0; i+1 < len(d); i += 2 {
		c := rune(d[i]) | rune(d[i+1])<<8
		if c == 0 {
			break
		}
		r = append(r, c)
	}
	return string(r)
}

func TestSidesAndMap(t *testing.T) {
	s := lineSides([]ptx.AudioLine{{Name: "inst"}, {Name: "inst"}, {Name: "vox"}, {Name: "kick R"}})
	if s[0] != panLeft || s[1] != panRight || s[2] != panCenter || s[3] != panRight {
		t.Fatalf("%v", s)
	}
	m := newTempoMap([]ptx.TempoEvent{{Ticks: 0, BPM: 120}, {Ticks: 4 * ptx.TicksPerBeat, BPM: 60}})
	if got := m.timeAtBeat(6); math.Abs(got-4) > 1e-9 { // 4 beats @120 = 2s, 2 beats @60 = 2s
		t.Fatalf("time %v", got)
	}
	if got := m.beatAtTime(4); math.Abs(got-6) > 1e-9 {
		t.Fatalf("beat %v", got)
	}
}

func TestBuildFromSyntheticAudio(t *testing.T) {
	ev := []ptx.TempoEvent{{Ticks: 0, BPM: 120}}
	mk := func(name string) ptx.AudioLine {
		return ptx.AudioLine{Name: name, Clips: []ptx.AudioClip{
			{RegionName: "r1", File: "a.wav", Start: 44100, Length: 44100, SrcOffset: 0},
			{RegionName: "r2", File: "a.wav", Start: 132300, Length: 22050, SrcOffset: 1000},
		}}
	}
	au := &ptx.PTXAudio{SampleRate: 44100, Lines: []ptx.AudioLine{mk("x"), mk("x"), mk("y")}}
	b, _, err := BuildFLPFull(BuildOptions{Events: ev, StartBar: 1, Audio: au, AudioDir: "/aud"})
	if err != nil {
		t.Fatal(err)
	}
	items, chans, _ := decodeItems(t, b)
	if len(items) != 6 || len(chans) != 3 {
		t.Fatalf("items %d channels %v", len(items), chans)
	}
}

func TestRecoverTakesAddsLanes(t *testing.T) {
	ev := []ptx.TempoEvent{{Ticks: 0, BPM: 120}}
	clip := func(f string, start, n int64) ptx.AudioClip {
		return ptx.AudioClip{RegionName: f, File: f, Start: start, Length: n}
	}
	au := &ptx.PTXAudio{SampleRate: 44100,
		Lines:    []ptx.AudioLine{{Name: "inst", Clips: []ptx.AudioClip{clip("a.wav", 0, 44100)}}},
		Unplaced: []ptx.AudioClip{clip("t1.wav", 0, 88200), clip("t2.wav", 44100, 88200), clip("t3.wav", 100000, 10000)},
	}
	if lanes := ptx.PackLanes(au.Unplaced); len(lanes) != 2 || len(lanes[0]) != 2 {
		t.Fatalf("lanes %+v", lanes)
	}
	b, _, err := BuildFLPFull(BuildOptions{Events: ev, StartBar: 1, Audio: au, AudioDir: "/aud", RecoverTakes: true})
	if err != nil {
		t.Fatal(err)
	}
	items, chans, _ := decodeItems(t, b)
	if len(items) != 4 || len(chans) != 4 {
		t.Fatalf("items %d channels %v", len(items), chans)
	}
	b, _, _ = BuildFLPFull(BuildOptions{Events: ev, StartBar: 1, Audio: au, AudioDir: "/aud"})
	if items, _, _ := decodeItems(t, b); len(items) != 1 {
		t.Fatalf("without recovery: %d items", len(items))
	}
}
