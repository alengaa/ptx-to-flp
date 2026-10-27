package main

// Audio track / clip reader for Pro Tools sessions.
//
// The block layout and field offsets follow the reverse-engineering work in
// libptformat (zamaudio/ptformat) and PTX2RPP (bpcrutchfield/PTX2RPP, MIT).

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pblock struct {
	bt   uint16
	ct   uint16
	size int // payload size (the 4-byte size field)
	off  int // offset of the payload (the 2-byte content type) = header + 7
	kids []*pblock
}

func rd2(d []byte, p int) int { return int(binary.LittleEndian.Uint16(d[p:])) }
func rd4(d []byte, p int) int { return int(binary.LittleEndian.Uint32(d[p:])) }
func rd5(d []byte, p int) uint64 {
	return uint64(d[p]) | uint64(d[p+1])<<8 | uint64(d[p+2])<<16 | uint64(d[p+3])<<24 | uint64(d[p+4])<<32
}

// parseBlockAt parses a block at pos. A child must fit inside its parent, which
// keeps false 0x5a matches inside payload data from exploding.
func parseBlockAt(d []byte, pos, end int) *pblock {
	if pos < 0 || pos >= len(d) || d[pos] != 0x5a || pos+9 > end {
		return nil
	}
	bt := rd2(d, pos+1)
	bs := rd4(d, pos+3)
	ct := rd2(d, pos+7)
	if bt&0xff00 != 0 || bs > 0x4000000 {
		return nil
	}
	be := pos + 7 + bs
	if be > end {
		return nil
	}
	b := &pblock{bt: uint16(bt), ct: uint16(ct), size: bs, off: pos + 7}
	for i := 1; i < bs; {
		c := parseBlockAt(d, pos+i, be)
		if c != nil {
			b.kids = append(b.kids, c)
			i += c.size + 7
		} else {
			i++
		}
	}
	return b
}

func findTopBlocks(d []byte) []*pblock {
	var out []*pblock
	for pos := 0x14; pos < len(d); {
		if b := parseBlockAt(d, pos, len(d)); b != nil {
			out = append(out, b)
			pos += b.size + 7
		} else {
			pos++
		}
	}
	return out
}

func findByCT(blocks []*pblock, ct uint16) []*pblock {
	var out []*pblock
	var walk func(bs []*pblock)
	walk = func(bs []*pblock) {
		for _, b := range bs {
			if b.ct == ct {
				out = append(out, b)
			}
			walk(b.kids)
		}
	}
	walk(blocks)
	return out
}

func readPTString(d []byte, pos int) (string, int) {
	if pos < 0 || pos+4 > len(d) {
		return "", pos
	}
	n := rd4(d, pos)
	pos += 4
	if n < 0 || n > 4096 || pos+n > len(d) {
		return "", pos
	}
	return strings.TrimRight(string(d[pos:pos+n]), "\x00"), pos + n
}

// parseThreePoint reads (source offset, length, start) in samples.
func parseThreePoint(d []byte, j int) (srcOff, length, start int64) {
	if j+10 >= len(d) {
		return 0, 0, 0
	}
	ob := int(d[j+1]&0xf0) >> 4
	lb := int(d[j+2]&0xf0) >> 4
	sb := int(d[j+3]&0xf0) >> 4
	base := j + 5
	rle := func(pos, n int) int64 {
		if n == 0 || pos+n > len(d) {
			return 0
		}
		var v int64
		for k := 0; k < n; k++ {
			v |= int64(d[pos+k]) << (8 * k)
		}
		return v
	}
	return rle(base, ob), rle(base+ob, lb), rle(base+ob+lb, sb)
}

type ptRegion struct {
	index     int
	name      string
	start     int64
	length    int64
	srcOffset int64
	fileIndex int
}

// AudioClip is one active audio clip on the Pro Tools timeline.
type AudioClip struct {
	RegionName string
	File       string // registered media file name (as stored in the PTX)
	Start      int64  // timeline start, samples from session start
	Length     int64  // samples
	SrcOffset  int64  // samples trimmed from the start of the source file
}

// AudioLine is one Pro Tools playlist line (one mono channel) holding clips.
type AudioLine struct {
	Name    string
	Channel int // 0 = left/first channel of the track, 1 = right
	Clips   []AudioClip
}

// PTXAudio is everything the FLP builder needs from the PTX.
type PTXAudio struct {
	SampleRate int
	Lines      []AudioLine // only lines that contain clips, in PTX order
	EmptyLines int
	// OverlapsTrimmed counts clips shortened, split or removed because a later clip covers them.
	OverlapsTrimmed int
}

// resolveOverlaps makes the clips of one line non-overlapping, like a Pro Tools playlist
// shows them: where clips overlap, the one that starts later is on top. Clips must be
// sorted by Start. Hidden parts are cut away (a clip can be split in two).
func resolveOverlaps(clips []AudioClip, sr int) ([]AudioClip, int) {
	minLen := int64(sr) / 500 // ignore slivers under 2 ms
	var out []AudioClip
	changed := 0
	for i, c := range clips {
		pieces := [][2]int64{{c.Start, c.Start + c.Length}}
		for _, o := range clips[i+1:] {
			os, oe := o.Start, o.Start+o.Length
			var next [][2]int64
			for _, p := range pieces {
				if oe <= p[0] || os >= p[1] {
					next = append(next, p)
					continue
				}
				if os > p[0] {
					next = append(next, [2]int64{p[0], os})
				}
				if oe < p[1] {
					next = append(next, [2]int64{oe, p[1]})
				}
			}
			pieces = next
			if len(pieces) == 0 {
				break
			}
		}
		if len(pieces) != 1 || pieces[0][0] != c.Start || pieces[0][1] != c.Start+c.Length {
			changed++
		}
		for _, p := range pieces {
			if p[1]-p[0] < minLen {
				continue
			}
			n := c
			n.Start = p[0]
			n.Length = p[1] - p[0]
			n.SrcOffset = c.SrcOffset + (p[0] - c.Start)
			out = append(out, n)
		}
	}
	return out, changed
}

func extractAudioFileList(d []byte, top []*pblock) []string {
	var files []string
	for _, b1004 := range findByCT(top, 0x1004) {
		if b1004.off+6 > len(d) {
			continue
		}
		nwavs := rd4(d, b1004.off+2)
		for _, c := range b1004.kids {
			if c.ct != 0x103a {
				continue
			}
			pos := c.off + 11
			valid := 0
			for pos < c.off+c.size && valid < nwavs {
				if pos+4 > len(d) {
					break
				}
				nl := rd4(d, pos)
				if nl == 0 || nl > 512 || pos+4+nl > len(d) {
					pos += 4
					continue
				}
				fname := strings.TrimRight(string(d[pos+4:pos+4+nl]), "\x00")
				pos += 4 + nl
				if pos+9 > len(d) {
					break
				}
				wt := string(d[pos : pos+4])
				pos += 9
				if strings.Contains(fname, ".grp") || strings.Contains(fname, "Audio Files") || strings.Contains(fname, "Fade Files") {
					continue
				}
				typeOK := false
				for _, x := range []string{"WAVE", "EVAW", "AIFF", "FFIA", "fFXM", "MXFf"} {
					if strings.Contains(wt, x) {
						typeOK = true
					}
				}
				l := strings.ToLower(fname)
				extOK := false
				for _, x := range []string{".wav", ".wave", ".aif", ".aiff", ".mxf"} {
					if strings.HasSuffix(l, x) {
						extOK = true
					}
				}
				if !typeOK && !extOK {
					continue
				}
				files = append(files, fname)
				valid++
			}
		}
	}
	return files
}

type ptTrack struct {
	name string
	chs  []int
}

func extractAudioTrackDefs(d []byte, top []*pblock) []ptTrack {
	var tracks []ptTrack
	seen := map[int]bool{}
	for _, cont := range findByCT(top, 0x1015) {
		for _, b := range cont.kids {
			if b.ct != 0x1014 || seen[b.off] {
				continue
			}
			seen[b.off] = true
			name, after := readPTString(d, b.off+2)
			if name == "" {
				continue
			}
			j := after + 1
			if j+4 > len(d) {
				continue
			}
			nch := rd4(d, j)
			j += 4
			if nch < 1 || nch > 8 {
				continue
			}
			var ids []int
			for k := 0; k < nch && j+2 <= len(d); k++ {
				ids = append(ids, rd2(d, j))
				j += 2
			}
			if len(ids) > 0 {
				tracks = append(tracks, ptTrack{name, ids})
			}
		}
	}
	return tracks
}

func extractRegionDefs(d []byte, top []*pblock) []ptRegion {
	var regs []ptRegion
	for _, rl := range top {
		if rl.ct != 0x100B && rl.ct != 0x262A {
			continue
		}
		for _, b := range rl.kids {
			if b.ct != 0x1008 && b.ct != 0x2629 {
				continue
			}
			j := b.off + 11
			if j+4 > len(d) {
				continue
			}
			name, after := readPTString(d, j)
			if name == "" {
				continue
			}
			src, ln, st := parseThreePoint(d, after)
			if ln <= 0 {
				continue
			}
			fi := -1
			if len(b.kids) > 0 {
				p := b.kids[0].off + b.kids[0].size
				if p+4 <= len(d) {
					fi = rd4(d, p)
				}
			}
			regs = append(regs, ptRegion{len(regs), name, st, ln, src, fi})
		}
	}
	return regs
}

func sessionSampleRate(d []byte, top []*pblock) int {
	sr := 44100
	for _, b := range findByCT(top, 0x1028) {
		if b.off+8 <= len(d) {
			if c := rd4(d, b.off+4); c >= 8000 && c <= 768000 {
				sr = c
			}
		}
	}
	return sr
}

// ReadPTXAudio extracts the audio lines (mono channels) that hold active clips.
func ReadPTXAudio(path string) (*PTXAudio, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := decryptPTX(raw)
	if err != nil {
		return nil, err
	}
	top := findTopBlocks(d)
	res := &PTXAudio{SampleRate: sessionSampleRate(d, top)}
	files := extractAudioFileList(d, top)
	defs := extractAudioTrackDefs(d, top)
	regs := extractRegionDefs(d, top)
	byIdx := map[int]ptRegion{}
	for _, r := range regs {
		byIdx[r.index] = r
	}
	if len(regs) == 0 {
		return res, errors.New("no audio regions found in this session")
	}

	type chInfo struct {
		name string
		num  int
	}
	var chs []chInfo
	for _, t := range defs {
		for n := range t.chs {
			chs = append(chs, chInfo{t.name, n})
		}
	}

	var maps []*pblock
	for _, b := range top {
		if b.ct == 0x1054 {
			maps = append(maps, b)
		}
	}
	if len(maps) == 0 {
		seen := map[int]bool{}
		for _, b := range findByCT(top, 0x1054) {
			if !seen[b.off] {
				seen[b.off] = true
				maps = append(maps, b)
			}
		}
	}

	for _, m := range maps {
		count := 0
		for _, e := range m.kids {
			if e.ct != 0x1052 {
				continue
			}
			label, _ := readPTString(d, e.off+2)
			label = strings.TrimSpace(label)
			ci := chInfo{label, count}
			if count < len(chs) {
				ci = chs[count]
				if label != "" {
					ci.name = label
				}
			}
			if ci.name == "" {
				ci.name = fmt.Sprintf("Track %d", count+1)
			}
			count++

			line := AudioLine{Name: ci.name, Channel: ci.num}
			for _, pl := range e.kids {
				if pl.ct != 0x1050 {
					continue
				}
				for _, ref := range pl.kids {
					if ref.ct != 0x104F {
						continue
					}
					j := ref.off + 4
					if j+16 > len(d) {
						continue
					}
					ri := rd4(d, j)
					start5 := rd5(d, j+5)
					start := int64(start5)
					if start5 >= zeroTicks {
						start = int64(start5 - zeroTicks)
					}
					reg, ok := byIdx[ri]
					if !ok {
						continue
					}
					if d[j+13] != 0x03 { // auxiliary reference, not a timeline clip
						continue
					}
					// Older sessions mark stale references with a trailing 0x01; the newer
					// 48-byte placement blocks always end in 0x01, so the test does not apply.
					if sp := pl.off + pl.size - 1; pl.size != 48 && sp < len(d) && d[sp] == 0x01 {
						continue
					}
					file := ""
					if reg.fileIndex >= 0 && reg.fileIndex < len(files) {
						file = files[reg.fileIndex]
					}
					line.Clips = append(line.Clips, AudioClip{
						RegionName: reg.name, File: file, Start: start,
						Length: reg.length, SrcOffset: reg.srcOffset,
					})
				}
			}
			sort.SliceStable(line.Clips, func(a, b int) bool { return line.Clips[a].Start < line.Clips[b].Start })
			var cut int
			line.Clips, cut = resolveOverlaps(line.Clips, res.SampleRate)
			res.OverlapsTrimmed += cut
			if len(line.Clips) == 0 {
				res.EmptyLines++
				continue
			}
			res.Lines = append(res.Lines, line)
		}
		if len(res.Lines) > 0 {
			break // the first map with content is the real one
		}
	}
	if len(res.Lines) == 0 {
		return res, errors.New("no audio clips found on the timeline")
	}
	return res, nil
}

// FindAudioFile looks for a media file by name inside dir (case-insensitive,
// extension-insensitive), returning the full path or "".
func FindAudioFile(dir, registered string) string {
	if dir == "" || registered == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(registered, "\\", "/"))
	if p := filepath.Join(dir, base); fileExists(p) {
		return p
	}
	// Pro Tools registers some names with a leading space (" .0001_0001.wav")
	// while the file on disk has none; FL Studio also trims it, so match trimmed.
	if p := filepath.Join(dir, strings.TrimSpace(base)); fileExists(p) {
		return p
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	stem := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base))))
	for _, e := range ents {
		n := e.Name()
		switch strings.ToLower(filepath.Ext(n)) {
		case ".wav", ".aif", ".aiff":
		default:
			continue
		}
		if strings.ToLower(strings.TrimSpace(strings.TrimSuffix(n, filepath.Ext(n)))) == stem {
			return filepath.Join(dir, n)
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
