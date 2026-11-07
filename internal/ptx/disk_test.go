package ptx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBWF writes a tiny mono 24-bit WAV with an optional bext TimeReference.
func writeBWF(t *testing.T, path string, timeRef int64, frames int, withBext bool) {
	t.Helper()
	le := binary.LittleEndian
	var body []byte
	chunk := func(id string, data []byte) {
		body = append(body, id...)
		body = le.AppendUint32(body, uint32(len(data)))
		body = append(body, data...)
		if len(data)&1 == 1 {
			body = append(body, 0)
		}
	}
	if withBext {
		b := make([]byte, 602)
		copy(b[256:], "Test")
		le.PutUint32(b[338:], uint32(timeRef))
		le.PutUint32(b[342:], uint32(timeRef>>32))
		chunk("bext", b)
	}
	f := make([]byte, 16)
	le.PutUint16(f[0:], 1)
	le.PutUint16(f[2:], 1)
	le.PutUint32(f[4:], 44100)
	le.PutUint32(f[8:], 44100*3)
	le.PutUint16(f[12:], 3)
	le.PutUint16(f[14:], 24)
	chunk("fmt ", f)
	chunk("data", make([]byte, frames*3))
	out := append([]byte("RIFF"), le.AppendUint32(nil, uint32(4+len(body)))...)
	out = append(out, "WAVE"...)
	out = append(out, body...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadWavInfo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.wav")
	writeBWF(t, p, 159403718, 1000, true)
	i, err := ReadWavInfo(p)
	if err != nil || i.TimeRef != 159403718 || i.Frames != 1000 || i.SampleRate != 44100 || i.Channels != 1 {
		t.Fatalf("%+v %v", i, err)
	}
	writeBWF(t, p, 0, 10, false)
	if i, _ := ReadWavInfo(p); i.TimeRef != -1 {
		t.Fatalf("expected no timestamp, got %d", i.TimeRef)
	}
}

func TestAddDiskTakes(t *testing.T) {
	d := t.TempDir()
	writeBWF(t, filepath.Join(d, "known.wav"), 100, 10, true)
	writeBWF(t, filepath.Join(d, "take 1.wav"), 44100*3600+44100*5, 44100, true)
	writeBWF(t, filepath.Join(d, "take 2.wav"), 0, 10, false)
	os.WriteFile(filepath.Join(d, "._take 1.wav"), []byte("x"), 0o600)
	au := &PTXAudio{SampleRate: 44100, OriginSamples: 44100 * 3600, RegisteredFiles: []string{"Known.WAV"}}
	n, no, err := AddDiskTakes(au, d)
	if err != nil || n != 1 || len(no) != 1 || no[0].Name != "take 2.wav" {
		t.Fatalf("n=%d noStamp=%v err=%v", n, no, err)
	}
	if c := au.DiskTakes[0]; c.File != "take 1.wav" || c.Start != 44100*5 || c.Length != 44100 {
		t.Fatalf("%+v", c)
	}
}

func TestResolveSessionInput(t *testing.T) {
	d := t.TempDir()
	if _, _, err := ResolveSessionInput(d); err == nil {
		t.Fatal("expected error for a folder without .ptx")
	}
	os.WriteFile(filepath.Join(d, "._s.ptx"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(d, "old.ptx"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(d, "new.ptx"), []byte("x"), 0o600)
	old := filepath.Join(d, "old.ptx")
	os.Chtimes(old, timeAgo(), timeAgo())
	p, n, err := ResolveSessionInput(d)
	if err != nil || n != 2 || filepath.Base(p) != "new.ptx" {
		t.Fatalf("%q %d %v", p, n, err)
	}
	if p, n, _ := ResolveSessionInput(old); p != old || n != 1 {
		t.Fatal("a file path must pass through")
	}
}

func timeAgo() time.Time { return time.Now().Add(-48 * time.Hour) }

func TestFindSessionAudioDir(t *testing.T) {
	root := t.TempDir()
	if d, _ := FindSessionAudioDir(root); d != "" {
		t.Fatalf("empty folder: got %q", d)
	}
	os.WriteFile(filepath.Join(root, "x.wav"), []byte("x"), 0o600)
	if d, how := FindSessionAudioDir(root); d != root || how == "" {
		t.Fatalf("WAV in root: got %q %q", d, how)
	}
	sub := filepath.Join(root, "audio files")
	os.Mkdir(sub, 0o755)
	if d, _ := FindSessionAudioDir(root); d != sub {
		t.Fatalf("Audio Files folder must win (case-insensitive): got %q", d)
	}
}

func TestListSessionFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.ptx", "b.PTX", "._a.ptx", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListSessionFiles(dir)
	if err != nil || len(got) != 2 {
		t.Fatalf("want the 2 real .ptx files, got %v (%v)", got, err)
	}
}
