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
// usage: ptx-to-flp <file.ptx | project folder> [bar N] [lines N] [recover 1] [fades 1] [disk 1] [out file.flp] [audio dir]
func main() {
	if len(os.Args) < 2 {
		fmt.Println("ptx-to-flp", version)
		fmt.Println("usage: ptx-to-flp <file.ptx | project folder> [bar N] [lines N] [recover 1] [fades 1] [disk 1] [out file.flp] [audio dir]")
		os.Exit(2)
	}
	ptxPath, _, err := ptx.ResolveSessionInput(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	folder := ""
	if st, e := os.Stat(os.Args[1]); e == nil && st.IsDir() {
		folder = os.Args[1]
		fmt.Println("Project folder mode: using", ptxPath)
	}
	res, err := ptx.ExtractTempo(ptxPath)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Print(res.Format())
	bar, lines, out, dir := 1, 0, "", ""
	var rec, fades, disk bool
	for i := 2; i+1 < len(os.Args); i += 2 {
		v := os.Args[i+1]
		switch os.Args[i] {
		case "bar":
			bar, _ = strconv.Atoi(v)
		case "lines":
			lines, _ = strconv.Atoi(v)
		case "recover":
			rec = v == "1"
		case "fades":
			fades = v == "1"
		case "disk":
			disk = v == "1"
		case "out":
			out = v
		case "audio":
			dir = v
		}
	}
	if out == "" {
		return
	}
	if dir == "" && folder != "" {
		dir, _ = ptx.FindSessionAudioDir(folder)
	}
	o := flp.BuildOptions{Events: res.Events, StartBar: bar, Lines: lines, AudioDir: dir, RecoverTakes: rec}
	if dir != "" {
		au, err := ptx.ReadPTXAudioOpts(ptxPath, ptx.ReadOptions{IncludeFades: fades})
		if err != nil && (au == nil || !(disk && folder != "")) {
			fmt.Println("Audio:", err)
			os.Exit(1)
		}
		if disk && folder != "" {
			added, skipped, e := ptx.AddDiskTakes(au, dir)
			fmt.Printf("WAVs not in the .ptx: %d added, %d skipped (%v)\n", added, len(skipped), e)
			for _, s := range skipped {
				fmt.Println("  skipped", s.Name+":", s.Reason)
			}
			o.AddDiskTakes = true
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
