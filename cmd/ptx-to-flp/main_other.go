//go:build !windows

package main

import (
	"github.com/alengaa/ptx-to-flp/internal/flp"
	"github.com/alengaa/ptx-to-flp/internal/ptx"

	"fmt"
	"os"
	"strconv"
)

// Command-line stand-in so the logic can be tested off Windows.
// usage: ptx-to-flp file.ptx [bar N] [lines N] [recover 1] [out file.flp] [audio dir]
func main() {
	if len(os.Args) < 2 {
		fmt.Println("ptx-to-flp", version)
		fmt.Println("usage: ptx-to-flp file.ptx [bar N] [lines N] [recover 1] [out file.flp] [audio dir]")
		os.Exit(2)
	}
	res, err := ptx.ExtractTempo(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Print(res.Format())
	bar, lines, out, dir, rec := 1, 0, "", "", false
	for i := 2; i+1 < len(os.Args); i += 2 {
		v := os.Args[i+1]
		switch os.Args[i] {
		case "bar":
			bar, _ = strconv.Atoi(v)
		case "lines":
			lines, _ = strconv.Atoi(v)
		case "recover":
			rec = v == "1"
		case "out":
			out = v
		case "audio":
			dir = v
		}
	}
	if out == "" {
		return
	}
	o := flp.BuildOptions{Events: res.Events, StartBar: bar, Lines: lines, AudioDir: dir, RecoverTakes: rec}
	if dir != "" {
		au, err := ptx.ReadPTXAudio(os.Args[1])
		if err != nil {
			fmt.Println("Audio:", err)
			os.Exit(1)
		}
		o.Audio = au
	}
	b, rep, err := flp.BuildFLPFull(o)
	fmt.Print(rep)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	os.WriteFile(out, b, 0o644)
	fmt.Println("wrote", out, len(b), "bytes")
}
