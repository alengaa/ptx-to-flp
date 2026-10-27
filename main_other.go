//go:build !windows

package main

import (
	"fmt"
	"os"
	"strconv"
)

// Command-line stand-in so the logic can be tested off Windows.
// usage: ptxtempo file.ptx [bar N] [lines N] [out file.flp] [audio dir]
func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: ptxtempo file.ptx [bar N] [lines N] [out file.flp] [audio dir]")
		os.Exit(2)
	}
	res, err := ExtractTempo(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Print(res.Format())
	bar, lines, out, dir := 1, 0, "", ""
	for i := 2; i+1 < len(os.Args); i += 2 {
		v := os.Args[i+1]
		switch os.Args[i] {
		case "bar":
			bar, _ = strconv.Atoi(v)
		case "lines":
			lines, _ = strconv.Atoi(v)
		case "out":
			out = v
		case "audio":
			dir = v
		}
	}
	if out == "" {
		return
	}
	o := BuildOptions{Events: res.Events, StartBar: bar, Lines: lines, AudioDir: dir}
	if dir != "" {
		au, err := ReadPTXAudio(os.Args[1])
		if err != nil {
			fmt.Println("Audio:", err)
			os.Exit(1)
		}
		o.Audio = au
	}
	b, rep, err := BuildFLPFull(o)
	fmt.Print(rep)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	os.WriteFile(out, b, 0o644)
	fmt.Println("wrote", out, len(b), "bytes")
}
