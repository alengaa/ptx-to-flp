package ptx

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ResolveSessionInput accepts either a .ptx file or a session folder. For a folder it
// picks the most recently modified top-level .ptx (ignoring macOS "._" stubs) and returns
// how many candidates there were.
func ResolveSessionInput(p string) (ptxPath string, candidates int, err error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", 0, err
	}
	if !st.IsDir() {
		return p, 1, nil
	}
	ents, err := os.ReadDir(p)
	if err != nil {
		return "", 0, err
	}
	var best string
	var bestT int64
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || !strings.EqualFold(filepath.Ext(n), ".ptx") {
			continue
		}
		candidates++
		if info, err := e.Info(); err == nil && (best == "" || info.ModTime().UnixNano() > bestT) {
			best, bestT = n, info.ModTime().UnixNano()
		}
	}
	if best == "" {
		return "", 0, errors.New("no .ptx file found in that folder")
	}
	return filepath.Join(p, best), candidates, nil
}

// FindAudioDir returns the session's "Audio Files" folder next to the .ptx ("" if absent).
func FindAudioDir(ptxPath string) string {
	dir := filepath.Dir(ptxPath)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() && strings.EqualFold(e.Name(), "Audio Files") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(filepath.Base(strings.ReplaceAll(s, "\\", "/"))))
	return strings.TrimSpace(strings.TrimSuffix(s, filepath.Ext(s)))
}

// DiskScan compares an audio folder with the files the session registers.
type DiskScan struct {
	OnDisk        int      // audio files in the folder
	NotRegistered []string // file names on disk that the .ptx does not know
}

// ScanAudioDir lists audio files in dir that the session does not register.
func ScanAudioDir(dir string, au *PTXAudio) (DiskScan, error) {
	var sc DiskScan
	ents, err := os.ReadDir(dir)
	if err != nil {
		return sc, err
	}
	reg := map[string]bool{}
	for _, f := range au.RegisteredFiles {
		reg[normName(f)] = true
	}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(n), ".wav") {
			continue
		}
		sc.OnDisk++
		if !reg[normName(n)] {
			sc.NotRegistered = append(sc.NotRegistered, n)
		}
	}
	sort.Strings(sc.NotRegistered)
	return sc, nil
}

// SkippedFile is a WAV that AddDiskTakes could not position, with the reason.
type SkippedFile struct{ Name, Reason string }

// AddDiskTakes adds audio files that exist in dir but are unknown to the session to
// au.DiskTakes, positioned from their Broadcast WAV timestamp (where Pro Tools recorded
// them) minus the session's timecode origin. Files without a timestamp are returned in
// noStamp. Best guess only: it is where the take was recorded, not an edit.
func AddDiskTakes(au *PTXAudio, dir string) (added int, skipped []SkippedFile, err error) {
	sc, err := ScanAudioDir(dir, au)
	if err != nil {
		return 0, nil, err
	}
	for _, n := range sc.NotRegistered {
		info, e := ReadWavInfo(filepath.Join(dir, n))
		switch {
		case e != nil:
			skipped = append(skipped, SkippedFile{n, "unreadable: " + e.Error()})
			continue
		case info.TimeRef < 0:
			skipped = append(skipped, SkippedFile{n, "no recording time stored in the file (no Broadcast WAV chunk)"})
			continue
		case info.TimeRef == 0:
			skipped = append(skipped, SkippedFile{n, "recording time is 0 (not set, e.g. imported, bounced or consolidated audio)"})
			continue
		case info.Frames <= 0:
			skipped = append(skipped, SkippedFile{n, "no audio data"})
			continue
		}
		pos, length := info.TimeRef, info.Frames
		if info.SampleRate != au.SampleRate && info.SampleRate > 0 { // convert to session samples
			pos = pos * int64(au.SampleRate) / int64(info.SampleRate)
			length = length * int64(au.SampleRate) / int64(info.SampleRate)
		}
		if pos >= au.OriginSamples {
			pos -= au.OriginSamples
		}
		au.DiskTakes = append(au.DiskTakes, AudioClip{
			RegionName: strings.TrimSuffix(n, filepath.Ext(n)), File: n, Start: pos, Length: length,
		})
		added++
	}
	sort.SliceStable(au.DiskTakes, func(a, b int) bool { return au.DiskTakes[a].Start < au.DiskTakes[b].Start })
	return added, skipped, nil
}

// FindSessionAudioDir locates the audio files of a project folder: its "Audio Files"
// sub-folder first, then the folder itself if it directly contains WAVs. how describes
// the choice; dir is "" when nothing was found and the user must pick the folder.
func FindSessionAudioDir(root string) (dir, how string) {
	if d := FindAudioDir(filepath.Join(root, "x.ptx")); d != "" {
		return d, "Audio Files folder"
	}
	if ents, err := os.ReadDir(root); err == nil {
		for _, e := range ents {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") && strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
				return root, "project folder itself (no Audio Files folder)"
			}
		}
	}
	return "", ""
}

// ListSessionFiles returns the names of the top-level .ptx files in a project folder
// (ignoring macOS "._" stubs), most recently modified first.
func ListSessionFiles(folder string) ([]string, error) {
	ents, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	type f struct {
		name string
		t    int64
	}
	var fs []f
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || !strings.EqualFold(filepath.Ext(n), ".ptx") {
			continue
		}
		var t int64
		if info, err := e.Info(); err == nil {
			t = info.ModTime().UnixNano()
		}
		fs = append(fs, f{n, t})
	}
	sort.SliceStable(fs, func(a, b int) bool { return fs[a].t > fs[b].t })
	out := make([]string, len(fs))
	for i, x := range fs {
		out[i] = x.name
	}
	return out, nil
}
