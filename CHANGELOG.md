# Changelog

Versioning follows [Semantic Versioning](https://semver.org). While the major
version is 0, anything may change. Pre-releases are tagged `-alpha.N`, then
`-beta.N`, then stable.

## [0.1.0-alpha.1]

First public alpha.

- Extract project BPM and tempo automation from a `.ptx`.
- Create an `.flp` with the tempo and a linked tempo-automation clip.
- Copy audio clip placements of the first N playlist lines (or all) into the FLP
  playlist, sample-accurate, hard-panned L/R for split stereo material.
- "Song starts at bar" offset.
- **New:** optional recovery of unplaced takes. Regions that exist in the
  session's clip list but are on no playlist are placed on extra lines at the
  position stored in the region (best guess).
- Handles Pro Tools file names that start with a space, which FL Studio trims.
- Project restructured into `cmd/` and `internal/` packages.

### Known limitations

- Muted clips are not muted in the FLP.
- Only 25 fps session timecode origins are verified.
- Tested with Pro Tools 12-format sessions and FL Studio 26.
