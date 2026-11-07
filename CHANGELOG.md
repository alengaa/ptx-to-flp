# Changelog

Versioning follows [Semantic Versioning](https://semver.org). While the major
version is 0, anything may change. Pre-releases are tagged `-alpha.N`, then
`-beta.N`, then stable.

## [0.1.0-alpha.9]

- New dark interface. Main window: session box, the two select buttons, Create FLP and the result box.
- Options moved to a hamburger menu (top right): song start bar, number of lines / all lines, recover takes, fade pieces, WAVs not in the .ptx, open folder when done.
- Explorer opens with the new .flp selected when a conversion finishes (on by default, can be switched off in the menu).
- Changing an option re-reads the session summary.

## [0.1.0-alpha.8]

- Fix: "Select project folder..." now really opens the Explorer-style dialog (a typo in the dialog's interface ID made it fall back to the old tree). The fallback is now the regular file window.
- Clips muted in Pro Tools are placed muted in the FLP (the flag lives in the placement reference).
- A project folder with several .ptx files asks "Which PTX project would you like to use?".

## [0.1.0-alpha.7]

- "Select project folder..." now opens the same Explorer-style dialog as the .ptx picker (in folder mode); the old tree dialog is only a fallback.

## [0.1.0-alpha.6]

- Fix: pieces of the instrumental no longer show up on take lines (fade-flagged pieces of a file that is also placed normally on another line are dropped, even with "Include fade pieces" on).
- Fix: renders derived from a placed file (e.g. `<beat>-VariFi_01.wav`) are no longer recovered as takes.

## [0.1.0-alpha.5]

- Clips on playlists that are not on any track of the session (Pro Tools does not play them) are now placed muted, like recovered takes. Nothing is muted if that would mute every line.

## [0.1.0-alpha.4]

- Fix: "Select project folder..." did nothing (the button was never wired to its handler).
- Fix: overlapping clips no longer lose Pro Tools' real structure; kind 2 placements sit underneath their neighbours instead of "later start wins".
- Recovered and found takes are now placed muted.

## [0.1.0-alpha.3]

- Fix: "Select project folder..." did nothing (the folder dialog needs COM initialised).
- WAVs that cannot be placed in folder mode are now listed with the reason.

## [0.1.0-alpha.2]

- **Fix: missing clips.** Placements of kind 1 and 2 are real timeline clips and
  were being ignored (e.g. alternate takes, and the first section of an
  instrumental). All kinds are read now; the result matches the reference parser's
  clip count on every test session.
- Placements flagged as fade pieces are skipped by default; new checkbox to include them.
- **Two input modes.** "Select .ptx file" (limited: only what the `.ptx` contains)
  and "Select project folder". Folder mode finds the `.ptx` (most recently saved if
  there are several), then `Audio Files/`; if that is missing it looks for WAVs in the
  folder itself, and otherwise asks you for the folder. Dropping a file or a folder
  on the window picks the mode.
- Folder mode warns when the audio folder has WAVs the `.ptx` does not know (older
  save) and can optionally place them from the recording timestamp stored in the WAV
  (Broadcast WAV `bext` chunk).

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
