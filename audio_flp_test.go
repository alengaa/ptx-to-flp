package main

import (
	"bytes"
	"encoding/binary"
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
	s := lineSides([]AudioLine{{Name: "inst"}, {Name: "inst"}, {Name: "vox"}, {Name: "kick R"}})
	if s[0] != panLeft || s[1] != panRight || s[2] != panCenter || s[3] != panRight {
		t.Fatalf("%v", s)
	}
	m := newTempoMap([]TempoEvent{{0, 120}, {4 * ticksPerBeat, 60}})
	if got := m.timeAtBeat(6); math.Abs(got-4) > 1e-9 { // 4 beats @120 = 2s, 2 beats @60 = 2s
		t.Fatalf("time %v", got)
	}
	if got := m.beatAtTime(4); math.Abs(got-6) > 1e-9 {
		t.Fatalf("beat %v", got)
	}
}

func TestBuildFromSyntheticAudio(t *testing.T) {
	ev := []TempoEvent{{Ticks: 0, BPM: 120}}
	mk := func(name string) AudioLine {
		return AudioLine{Name: name, Clips: []AudioClip{
			{RegionName: "r1", File: "a.wav", Start: 44100, Length: 44100, SrcOffset: 0},
			{RegionName: "r2", File: "a.wav", Start: 132300, Length: 22050, SrcOffset: 1000},
		}}
	}
	au := &PTXAudio{SampleRate: 44100, Lines: []AudioLine{mk("x"), mk("x"), mk("y")}}
	b, _, err := BuildFLPFull(BuildOptions{Events: ev, StartBar: 1, Audio: au, AudioDir: "/aud"})
	if err != nil {
		t.Fatal(err)
	}
	items, chans, _ := decodeItems(t, b)
	if len(items) != 6 || len(chans) != 3 {
		t.Fatalf("items %d channels %v", len(items), chans)
	}
}
