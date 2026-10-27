package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Tempo positions in a PTX are stored as 5-byte little-endian ticks offset by
// zeroTicks. One beat (quarter note) is 960000 ticks.
const (
	zeroTicks    = 0xE8D4A51000
	ticksPerBeat = 960000
	minBPM       = 5.0
	maxBPM       = 1000.0
)

// TempoEvent is one tempo-map entry.
type TempoEvent struct {
	Ticks uint64  // position from session start, in ticks
	BPM   float64 // tempo at that position
}

// Beats returns the position in beats (quarter notes) from the session start.
func (e TempoEvent) Beats() float64 { return float64(e.Ticks) / ticksPerBeat }

// Result is what ExtractTempo returns.
type Result struct {
	FileName    string
	BlocksFound int          // number of tempo-map blocks located in the file
	Events      []TempoEvent // sorted by position, duplicates removed
}

func genXorDelta(xv, mul int, neg bool) int {
	for i := 0; i < 256; i++ {
		if (i*mul)&0xff == xv {
			if neg {
				return (-i) & 0xff
			}
			return i
		}
	}
	return 0
}

// decryptPTX undoes the byte-wise XOR obfuscation Pro Tools applies to sessions.
func decryptPTX(raw []byte) ([]byte, error) {
	if len(raw) < 0x20 || raw[0] != 0x03 || string(raw[1:14]) != "0010111100101" {
		return nil, errors.New("this does not look like a Pro Tools session (.ptx) file")
	}
	xorType := raw[0x12]
	xv := int(raw[0x13])
	var xd int
	switch xorType {
	case 0x01:
		xd = genXorDelta(xv, 53, false)
	case 0x05:
		xd = genXorDelta(xv, 11, true)
	default:
		return nil, fmt.Errorf("unsupported Pro Tools file version (encoding type 0x%02x)", xorType)
	}
	var table [256]byte
	for i := range table {
		table[i] = byte(i * xd)
	}
	out := make([]byte, len(raw))
	copy(out, raw[:0x14])
	if xorType == 0x01 {
		for i := 0x14; i < len(raw); i++ {
			out[i] = raw[i] ^ table[i&0xff]
		}
	} else {
		for i := 0x14; i < len(raw); i++ {
			out[i] = raw[i] ^ table[(i>>12)&0xff]
		}
	}
	return out, nil
}

// tempoMagic is the content type (0x2028, little endian) followed by the
// "Tempo" tag that opens a tempo-map block.
var tempoMagic = []byte{0x28, 0x20, 'T', 'e', 'm', 'p', 'o'}

// findTempoBlocks returns the payload of every tempo-map block. A block is:
// 0x5a, type(2), size(4), then size bytes of payload starting with the content type.
func findTempoBlocks(d []byte) [][]byte {
	var blocks [][]byte
	i := 0
	for i < len(d) {
		j := bytes.Index(d[i:], tempoMagic)
		if j < 0 {
			break
		}
		p := i + j
		i = p + 1
		h := p - 7
		if h < 0x14 || d[h] != 0x5a {
			continue
		}
		bt := binary.LittleEndian.Uint16(d[h+1:])
		bs := int(binary.LittleEndian.Uint32(d[h+3:]))
		if bt&0xff00 != 0 || bs < len(tempoMagic) || p+bs > len(d) {
			continue
		}
		blocks = append(blocks, d[p:p+bs])
	}
	return blocks
}

// parseTempoBlock scans a tempo-map block for tempo records: a 5-byte tick
// position, 5 bytes of flags, then the tempo as a little-endian float64.
func parseTempoBlock(b []byte) []TempoEvent {
	var ev []TempoEvent
	for off := 0; off+18 <= len(b); off++ {
		if b[off+4] != 0xE8 {
			continue
		}
		v := uint64(b[off]) | uint64(b[off+1])<<8 | uint64(b[off+2])<<16 |
			uint64(b[off+3])<<24 | uint64(b[off+4])<<32
		if v < zeroTicks {
			continue
		}
		bpm := math.Float64frombits(binary.LittleEndian.Uint64(b[off+10:]))
		if !(bpm >= minBPM && bpm <= maxBPM) {
			continue
		}
		ev = append(ev, TempoEvent{Ticks: v - zeroTicks, BPM: bpm})
		off += 17 // skip past this record's fixed part
	}
	return ev
}

func extractFromData(d []byte) *Result {
	res := &Result{}
	blocks := findTempoBlocks(d)
	res.BlocksFound = len(blocks)
	seen := map[TempoEvent]bool{}
	for _, b := range blocks {
		for _, e := range parseTempoBlock(b) {
			if !seen[e] {
				seen[e] = true
				res.Events = append(res.Events, e)
			}
		}
	}
	sort.SliceStable(res.Events, func(a, b int) bool { return res.Events[a].Ticks < res.Events[b].Ticks })
	return res
}

// ExtractTempo reads a .ptx file and returns its tempo information.
func ExtractTempo(path string) (*Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := decryptPTX(raw)
	if err != nil {
		return nil, err
	}
	res := extractFromData(d)
	res.FileName = filepath.Base(path)
	return res, nil
}

func formatBPM(v float64) string {
	s := strconv.FormatFloat(math.Round(v*10000)/10000, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func formatClock(sec float64) string {
	ms := int64(math.Round(sec * 1000))
	return fmt.Sprintf("%d:%02d.%03d", ms/60000, (ms/1000)%60, ms%1000)
}

// Format renders a Result as plain text for display.
func (r *Result) Format() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "File: %s\n\n", r.FileName)

	if len(r.Events) == 0 {
		sb.WriteString("No tempo information found in this file.\n")
		if r.BlocksFound == 0 {
			sb.WriteString("(No tempo-map block located. The session may come from a Pro Tools version this tool does not recognise.)\n")
		} else {
			sb.WriteString("(A tempo-map block was found but no tempo record could be read from it.)\n")
		}
		return sb.String()
	}

	first := r.Events[0]
	minB, maxB := first.BPM, first.BPM
	distinct := map[float64]bool{}
	for _, e := range r.Events {
		distinct[e.BPM] = true
		minB = math.Min(minB, e.BPM)
		maxB = math.Max(maxB, e.BPM)
	}

	fmt.Fprintf(&sb, "Project tempo: %s BPM", formatBPM(first.BPM))
	if first.Ticks != 0 {
		sb.WriteString("  (first tempo event is not at the very start of the session)")
	}
	sb.WriteString("\n")

	if len(distinct) == 1 {
		sb.WriteString("Tempo automation: none (constant tempo)\n")
		if len(r.Events) > 1 {
			fmt.Fprintf(&sb, "(%d tempo events found, all with the same BPM)\n", len(r.Events))
		}
		return sb.String()
	}

	fmt.Fprintf(&sb, "Tempo automation: YES - %d tempo events, %s to %s BPM\n\n",
		len(r.Events), formatBPM(minB), formatBPM(maxB))

	fmt.Fprintf(&sb, "%5s  %-11s  %10s  %-10s  %s\n", "#", "Bar|Beat*", "Beats", "Time", "BPM")
	t := 0.0
	var prev TempoEvent
	for i, e := range r.Events {
		if i == 0 {
			t = e.Beats() * 60 / e.BPM
		} else {
			t += (e.Beats() - prev.Beats()) * 60 / prev.BPM
		}
		beats := e.Beats()
		bar := int(math.Floor(beats/4)) + 1
		beatInBar := math.Mod(beats, 4) + 1
		fmt.Fprintf(&sb, "%5d  %-11s  %10.3f  %-10s  %s\n", i+1,
			fmt.Sprintf("%d|%.3f", bar, beatInBar), beats, formatClock(t), formatBPM(e.BPM))
		prev = e
	}
	sb.WriteString("\n* Bar|Beat assumes 4/4 throughout. Beats = quarter notes from session start.\n")
	return sb.String()
}
