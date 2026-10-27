# Architecture

```
cmd/ptx-to-flp/      GUI (Windows, raw Win32) and a CLI stand-in for other OSes
internal/ptx/        .ptx reader: de-obfuscation, tempo map, audio regions/placements
internal/flp/        .flp writer: events, templates, playlist, automation
internal/flp/assets/ embedded template projects (generic, no user data)
docs/                format notes
```

`internal/ptx` knows nothing about FL Studio; `internal/flp` only consumes the
types `ptx` exports (`TempoEvent`, `PTXAudio`, `AudioLine`, `AudioClip`).

## Pipeline

1. `ptx.ExtractTempo` / `ptx.ReadPTXAudio` decrypt the file and return tempo
   events and audio lines.
2. `flp.BuildFLPFull` shifts everything to the chosen start bar, converts
   sample positions to FL ticks through the tempo map, and clones sample
   channel records from an embedded template.
3. The GUI writes the result and shows a report.

## Positions

- PTX tempo positions are ticks (960000 per quarter note). Audio positions are
  samples. They are connected through the tempo map (seconds <-> beats).
- FL Studio uses 96 PPQ. Clips are placed on the next tick at or after the
  true position and the remainder is absorbed in the clip's source offset, so
  the audio stays sample-accurate.

## Adding a feature

Put format reading in `internal/ptx`, format writing in `internal/flp`, UI in
`cmd/ptx-to-flp`. Tests must use synthetic data; never commit real sessions.
