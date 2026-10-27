package flp

// Builds an FLP that contains the tempo (+ automation) and audio clips placed on
// playlist lines. Audio-clip structures are copied from a project saved by FL Studio
// 26.1.6 (audio_base.flp); only the clip positions, sources, names and pan are patched.

import (
	"github.com/alengaa/ptx-to-flp/internal/ptx"

	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

//go:embed assets/audio_base.flp
var flpAudioBase []byte

type flEv struct {
	id   byte
	data []byte
}

// parseFLEvents splits an FLP into its 22-byte header and its events.
// Event 172 carries a 3-byte value in FL 26 (verified by the stream ending exactly at EOF).
func parseFLEvents(b []byte) ([]byte, []flEv, error) {
	if len(b) < 22 || string(b[:4]) != "FLhd" || string(b[14:18]) != "FLdt" {
		return nil, nil, errors.New("not an FLP")
	}
	var evs []flEv
	pos := 22
	for pos < len(b) {
		id := b[pos]
		pos++
		var n int
		switch {
		case id < 64:
			n = 1
		case id < 128:
			n = 2
		case id < 192:
			n = 4
		default:
			shift := 0
			for {
				if pos >= len(b) {
					return nil, nil, errors.New("truncated FLP")
				}
				c := int(b[pos])
				pos++
				n |= (c & 0x7f) << shift
				shift += 7
				if c&0x80 == 0 {
					break
				}
			}
		}
		if id == 172 {
			n = 3
		}
		if pos+n > len(b) {
			return nil, nil, errors.New("truncated FLP")
		}
		evs = append(evs, flEv{id, b[pos : pos+n]})
		pos += n
	}
	return b[:22], evs, nil
}

func serializeFL(head []byte, evs []flEv) []byte {
	out := append([]byte{}, head...)
	for _, e := range evs {
		out = append(out, e.id)
		if e.id >= 192 {
			out = append(out, varLen(len(e.data))...)
		}
		out = append(out, e.data...)
	}
	binary.LittleEndian.PutUint32(out[18:], uint32(len(out)-22))
	return out
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(u)*2+2)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return append(b, 0, 0)
}

func indexOfID(evs []flEv, id byte, from int) int {
	for i := from; i < len(evs); i++ {
		if evs[i].id == id {
			return i
		}
	}
	return -1
}

// ---- tempo map: seconds <-> beats ------------------------------------------------

type tempoMap struct {
	beat, bpm, time []float64 // segment start (beats), tempo, cumulative seconds at start
}

func newTempoMap(ev []ptx.TempoEvent) tempoMap {
	var m tempoMap
	add := func(b, bpm float64) {
		t := 0.0
		if n := len(m.beat); n > 0 {
			t = m.time[n-1] + (b-m.beat[n-1])*60/m.bpm[n-1]
		}
		m.beat = append(m.beat, b)
		m.bpm = append(m.bpm, bpm)
		m.time = append(m.time, t)
	}
	if len(ev) == 0 || ev[0].Ticks != 0 {
		bpm := 120.0
		if len(ev) > 0 {
			bpm = ev[0].BPM
		}
		add(0, bpm)
	}
	for _, e := range ev {
		if n := len(m.beat); n > 0 && e.Beats() <= m.beat[n-1] {
			m.bpm[n-1] = e.BPM // same position: later event wins
			continue
		}
		add(e.Beats(), e.BPM)
	}
	return m
}

func (m tempoMap) timeAtBeat(b float64) float64 {
	i := sort.Search(len(m.beat), func(i int) bool { return m.beat[i] > b }) - 1
	if i < 0 {
		i = 0
	}
	return m.time[i] + (b-m.beat[i])*60/m.bpm[i]
}

func (m tempoMap) beatAtTime(t float64) float64 {
	i := sort.Search(len(m.time), func(i int) bool { return m.time[i] > t }) - 1
	if i < 0 {
		i = 0
	}
	return m.beat[i] + (t-m.time[i])*m.bpm[i]/60
}

// ---- pan ------------------------------------------------------------------------

const (
	panLeft   = 0
	panCenter = 6400
	panRight  = 12800
)

var sideRe = regexp.MustCompile(`(?i)(?:^|[\s._\-(\[])([lr])[)\]]?$`)

// lineSides decides hard-pan sides for the selected lines: two consecutive lines with
// the same track label are a split-stereo pair (first = left); otherwise a trailing
// L / R in the name decides; anything else stays centered.
func lineSides(lines []ptx.AudioLine) []int {
	sides := make([]int, len(lines))
	for i := range sides {
		sides[i] = panCenter
	}
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && lines[i].Name == lines[i+1].Name {
			sides[i], sides[i+1] = panLeft, panRight
			i++
			continue
		}
		if m := sideRe.FindStringSubmatch(lines[i].Name); m != nil {
			if strings.EqualFold(m[1], "l") {
				sides[i] = panLeft
			} else {
				sides[i] = panRight
			}
		}
	}
	return sides
}

// ---- builder -------------------------------------------------------------------

// BuildOptions controls BuildFLPFull.
type BuildOptions struct {
	Events   []ptx.TempoEvent // tempo events as read from the PTX (not shifted)
	StartBar int              // song starts at this bar (1 = session start)
	Audio    *ptx.PTXAudio    // nil = no audio
	Lines    int              // lines with content to use: >0 first N, 0 = all, <0 none
	AudioDir string           // folder holding the session's audio files
	// PathFor, if set, can replace the sample path written for the clip on line index li.
	PathFor func(li int, found string) string
	// RecoverTakes also places the session's unplaced regions (see ptx.PTXAudio.Unplaced) on
	// extra lines below the regular audio, at their stored positions. Best guess only.
	RecoverTakes bool
}

type plItem struct {
	tick       uint32
	ch         int
	lenTicks   uint32
	lane       int
	startMS    float32
	endMS      float32
	isAutoClip bool
}

// BuildFLPFull creates the project and returns it with a human-readable report.
func BuildFLPFull(o BuildOptions) ([]byte, string, error) {
	if len(o.Events) == 0 {
		return nil, "", errors.New("no tempo information")
	}
	if o.StartBar < 1 {
		o.StartBar = 1
	}
	shifted := ShiftToStart(o.Events, o.StartBar)
	distinct := false
	for _, e := range shifted {
		if e.BPM != shifted[0].BPM {
			distinct = true
		}
	}
	bpm, _ := ClampFLTempo(shifted[0].BPM)

	if o.Audio == nil {
		if distinct {
			b, _, err := BuildFLPWithAutomation(shifted)
			return b, "", err
		}
		return BuildFLP(bpm), "", nil
	}

	// choose the base project
	var base []byte
	if distinct {
		b, _, err := BuildFLPWithAutomation(shifted)
		if err != nil {
			return nil, "", err
		}
		base = b
	} else {
		base = append([]byte{}, flpAudioBase...)
	}
	head, evs, err := parseFLEvents(base)
	if err != nil {
		return nil, "", err
	}
	_, aevs, err := parseFLEvents(flpAudioBase)
	if err != nil {
		return nil, "", err
	}

	// the reference's first audio channel record (events 64 .. before the next 64)
	c0 := indexOfID(aevs, 64, 0)
	c1 := indexOfID(aevs, 64, c0+1)
	c2 := indexOfID(aevs, 64, c1+1)
	if c0 < 0 || c1 < 0 || c2 < 0 {
		return nil, "", errors.New("internal template is damaged")
	}
	chanTpl := aevs[c1:c2]
	// the reference's first playlist item (an untouched audio clip)
	pl := aevs[indexOfID(aevs, 233, 0)].data
	if len(pl) < 88 {
		return nil, "", errors.New("internal template is damaged")
	}
	audioItem := append([]byte{}, pl[:88]...)

	// where do channel records go, and what do we drop?
	first64 := indexOfID(evs, 64, 0)
	end99 := indexOfID(evs, 99, first64)
	if first64 < 0 || end99 < 0 {
		return nil, "", errors.New("internal template is damaged")
	}
	insertAt := end99
	if !distinct { // audio-only base: keep its Sampler channel, drop its 2 sample channels
		second64 := indexOfID(evs, 64, first64+1)
		if second64 < 0 {
			return nil, "", errors.New("internal template is damaged")
		}
		evs = append(evs[:second64:second64], evs[end99:]...)
		insertAt = second64
	}

	// select lines
	lines := o.Audio.Lines
	if o.Lines < 0 { // takes only
		lines = nil
	} else if o.Lines > 0 && o.Lines < len(lines) {
		lines = lines[:o.Lines]
	}
	lines = append([]ptx.AudioLine{}, lines...)
	nRegular := len(lines)
	if o.RecoverTakes {
		for i, lane := range ptx.PackLanes(o.Audio.Unplaced) {
			lines = append(lines, ptx.AudioLine{Name: fmt.Sprintf("Recovered takes %d", i+1), Clips: lane})
		}
	}
	sides := lineSides(lines)
	sr := float64(o.Audio.SampleRate)
	origMap := newTempoMap(o.Events)
	t0 := origMap.timeAtBeat(float64(o.StartBar-1) * 4)
	newMap := newTempoMap(shifted)

	var report strings.Builder
	type chanKey struct {
		path string
		pan  int
	}
	chanID := map[chanKey]int{}
	var chanOrder []chanKey
	var items []plItem
	skippedEarly, noFile, missing := 0, 0, 0
	firstClipSec := math.Inf(1)

	for li, line := range lines {
		lane := li + 1
		for _, c := range line.Clips {
			if c.File == "" {
				noFile++
				continue
			}
			path := ptx.FindAudioFile(o.AudioDir, c.File)
			if path == "" {
				missing++
				path = filepath.Join(o.AudioDir, strings.TrimSpace(filepath.Base(strings.ReplaceAll(c.File, "\\", "/"))))
			}
			if o.PathFor != nil {
				path = o.PathFor(li, path)
			}
			key := chanKey{path, sides[li]}
			id, ok := chanID[key]
			if !ok {
				id = len(chanOrder) + 1
				chanID[key] = id
				chanOrder = append(chanOrder, key)
			}
			tStart := float64(c.Start)/sr - t0
			tEnd := float64(c.Start+c.Length)/sr - t0
			if tEnd <= 0 {
				skippedEarly++
				continue
			}
			src := float64(c.SrcOffset)
			if tStart < 0 { // starts before the song: trim the start
				src += -tStart * sr
				tStart = 0
			}
			tick := math.Ceil(newMap.beatAtTime(tStart)*flPPQ - 1e-6)
			tc := newMap.timeAtBeat(tick / flPPQ)
			if d := tc - tStart; d > 0 { // placed up to one tick late: absorb it in the source offset
				src += d * sr
			}
			endTick := math.Round(newMap.beatAtTime(tEnd) * flPPQ)
			if endTick <= tick {
				endTick = tick + 1
			}
			firstClipSec = math.Min(firstClipSec, tStart)
			items = append(items, plItem{
				tick: uint32(tick), ch: id, lenTicks: uint32(endTick - tick), lane: lane,
				startMS: float32(src / sr * 1000),
				endMS:   float32(float64(c.SrcOffset+c.Length) / sr * 1000),
			})
		}
		fmt.Fprintf(&report, "  line %d: %s (%d clips)\n", lane, line.Name, len(line.Clips))
	}

	// channel records
	var chEvs []flEv
	for k, key := range chanOrder {
		stem := strings.TrimSuffix(filepath.Base(key.path), filepath.Ext(key.path))
		for _, e := range chanTpl {
			d := append([]byte{}, e.data...)
			switch e.id {
			case 64:
				binary.LittleEndian.PutUint16(d, uint16(k+1))
			case 212:
				binary.LittleEndian.PutUint32(d[40:], uint32(12+26*k))
				binary.LittleEndian.PutUint32(d[44:], uint32(96+26*k))
			case 203:
				d = utf16z(stem)
			case 196:
				d = utf16z(key.path)
			case 219:
				binary.LittleEndian.PutUint32(d, uint32(key.pan))
			}
			chEvs = append(chEvs, flEv{e.id, d})
		}
	}
	evs = append(evs[:insertAt:insertAt], append(chEvs, evs[insertAt:]...)...)

	// automation clip goes on the line below the audio
	autoLane := len(lines) + 1
	var plData []byte
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].tick != items[b].tick {
			return items[a].tick < items[b].tick
		}
		return items[a].lane < items[b].lane
	})
	for _, it := range items {
		b := append([]byte{}, audioItem...)
		binary.LittleEndian.PutUint32(b[0:], it.tick)
		binary.LittleEndian.PutUint16(b[6:], uint16(it.ch))
		binary.LittleEndian.PutUint32(b[8:], it.lenTicks)
		binary.LittleEndian.PutUint16(b[12:], uint16(500-it.lane))
		binary.LittleEndian.PutUint32(b[24:], math.Float32bits(it.startMS))
		binary.LittleEndian.PutUint32(b[28:], math.Float32bits(it.endMS))
		plData = append(plData, b...)
	}
	pi := indexOfID(evs, 233, 0)
	if pi < 0 {
		return nil, "", errors.New("internal template is damaged")
	}
	if distinct { // keep the template's automation item (first 88 bytes), moved below the audio
		a := append([]byte{}, evs[pi].data[:88]...)
		binary.LittleEndian.PutUint16(a[12:], uint16(500-autoLane))
		plData = append(plData, a...)
	}
	evs[pi] = flEv{233, plData}

	// project tempo
	if ti := indexOfID(evs, 156, 0); ti >= 0 {
		binary.LittleEndian.PutUint32(evs[ti].data, uint32(math.Round(bpm*1000)))
	}

	if o.RecoverTakes {
		fmt.Fprintf(&report, "  Recovered %d unplaced takes on %d extra lines (best-guess positions)\n", len(o.Audio.Unplaced), len(lines)-nRegular)
	}
	fmt.Fprintf(&report, "  %d audio clips on %d lines, %d sample channels\n", len(items), len(lines), len(chanOrder))
	if skippedEarly > 0 {
		fmt.Fprintf(&report, "  %d clips end before the song start and were skipped\n", skippedEarly)
	}
	if noFile > 0 {
		fmt.Fprintf(&report, "  %d clips had no source file recorded and were skipped\n", noFile)
	}
	if missing > 0 {
		fmt.Fprintf(&report, "  WARNING: %d clips point at audio files not found in:\n    %s\n", missing, o.AudioDir)
	}
	if len(items) == 0 {
		return nil, report.String(), errors.New("no audio clips could be placed")
	}
	return serializeFL(head, evs), report.String(), nil
}
