# PTX to FLP

A small, free Windows program that converts a **Pro Tools session (`.ptx`)**
into an **FL Studio project (`.flp`)**. No installation, no Python: a single
`.exe` with a simple window.

> Early version. Always check the result in FL Studio. Tested with FL Studio 26.

## What it converts

- **Project tempo** (BPM).
- **Tempo automation** as an FL Studio automation clip linked to the project
  tempo, placed to the sample.
- **Audio clip placement** for the first *N* playlist lines that contain clips
  (default 2, or all lines). Clips land at the exact same position, with
  sample-accurate start/end trimming, and are loaded from the session's
  `Audio Files` folder.
  - Stereo-split material (`.L` / `.R`) is hard-panned left/right.
  - Overlapping clips are resolved so they don't play twice.
- **Song start bar**: shift everything so the song starts at a chosen bar.

## Not supported yet

- Muted clips are not muted in the FLP.
- Fades, clip gain, plugins/inserts, MIDI, volume/pan automation.
- Only tested with 44.1 kHz sessions and FL Studio 26.

## Usage

1. Download `ptx-to-flp.exe` from the [Releases](../../releases) page.
2. Run it, then pick (or drag in) your `.ptx` file.
3. **Extract tempo** shows the BPM and any tempo automation.
4. Set *Song starts at bar* and the number of audio lines (or *All lines*).
5. **Create FLP...**, choose where to save. The `Audio Files` folder next to
   the `.ptx` is used automatically; otherwise you'll be asked for it.

The WAV files must be where the session registered them (the `Audio Files`
folder). Your `.ptx` is never modified.

## Build from source

Requires Go 1.24+. No third-party modules.

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui" -o ptx-to-flp.exe .
go test ./...
```

On other systems a command-line stand-in is built for testing:
`go run . file.ptx [bar N] [lines N] [out file.flp] [audio dir]`.

## How it works

The `.ptx` is de-obfuscated, tempo and audio blocks are parsed, and the FLP is
generated from small embedded templates. See [NOTICE.md](NOTICE.md) for the
projects whose format research this builds on.

## License

[MIT](LICENSE). Not affiliated with Avid or Image-Line.
