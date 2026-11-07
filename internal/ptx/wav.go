package ptx

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// WavInfo is what the converter needs from a WAV / Broadcast WAV file.
type WavInfo struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
	Frames        int64 // length in sample frames
	TimeRef       int64 // BWF "TimeReference": sample position of the first sample, -1 if absent
}

// ReadWavInfo reads the header chunks of a WAV file without loading its audio.
// Pro Tools writes the position where a take was recorded into the BWF bext chunk.
func ReadWavInfo(path string) (WavInfo, error) {
	info := WavInfo{TimeRef: -1}
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()
	var hdr [12]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return info, err
	}
	if string(hdr[0:4]) != "RIFF" && string(hdr[0:4]) != "RF64" || string(hdr[8:12]) != "WAVE" {
		return info, errors.New("not a WAV file")
	}
	st, err := f.Stat()
	if err != nil {
		return info, err
	}
	var dataBytes int64 = -1
	pos := int64(12)
	for pos+8 <= st.Size() {
		var ch [8]byte
		if _, err := f.ReadAt(ch[:], pos); err != nil {
			break
		}
		id := string(ch[0:4])
		size := int64(binary.LittleEndian.Uint32(ch[4:]))
		body := pos + 8
		switch id {
		case "fmt ":
			var b [16]byte
			if _, err := f.ReadAt(b[:], body); err == nil {
				info.Channels = int(binary.LittleEndian.Uint16(b[2:]))
				info.SampleRate = int(binary.LittleEndian.Uint32(b[4:]))
				info.BitsPerSample = int(binary.LittleEndian.Uint16(b[14:]))
			}
		case "bext":
			// description(256) originator(32) ref(32) date(10) time(8) TimeReference low/high (u32 each)
			var b [8]byte
			if size >= 346 {
				if _, err := f.ReadAt(b[:], body+338); err == nil {
					lo := int64(binary.LittleEndian.Uint32(b[0:]))
					hi := int64(binary.LittleEndian.Uint32(b[4:]))
					info.TimeRef = lo | hi<<32
				}
			}
		case "data":
			dataBytes = size
			if size == 0xFFFFFFFF || body+size > st.Size() {
				dataBytes = st.Size() - body
			}
		}
		pos = body + size + size&1
	}
	if info.Channels > 0 && info.BitsPerSample > 0 && dataBytes >= 0 {
		info.Frames = dataBytes / int64(info.Channels*((info.BitsPerSample+7)/8))
	}
	if info.SampleRate == 0 {
		return info, errors.New("WAV has no fmt chunk")
	}
	return info, nil
}
