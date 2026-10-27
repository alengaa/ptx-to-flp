package ptx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpaceName(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, ".0001_0001.wav"), []byte("x"), 0o644)
	if p := FindAudioFile(d, " .0001_0001.wav"); filepath.Base(p) != ".0001_0001.wav" {
		t.Fatalf("got %q", p)
	}
}
