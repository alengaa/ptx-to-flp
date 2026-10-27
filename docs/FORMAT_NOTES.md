# Format notes

Reverse-engineering notes gathered while building this tool. They may be
incomplete or wrong; corrections welcome.

## Pro Tools `.ptx`

- Header byte `0x12` selects an XOR scheme (`0x01` or `0x05`), byte `0x13` is
  its seed; the rest is de-obfuscated block by block (see `genXorDelta`).
- Blocks: `0x5a`, type (u16), size (u32), content type (u16). Payload starts 7
  bytes after the block start.
- Tempo map: content type `0x2028`, 61-byte records: 5-byte little-endian
  ticks (minus `0xE8D4A51000`) at +0, float64 BPM at +10.
- Audio file list: `0x1004` / `0x103a`. Regions: `0x100B`/`0x262A` containing
  `0x1008`/`0x2629`; name at +11, then (source offset, length, start) in samples.
- Playlists: `0x1054` > `0x1052` (one per mono channel) > `0x1050` > `0x104F`
  (region index at +4, start at +5, kind at +13; `0x03` = timeline clip).
- Session timecode origin: block `0x204D`, frame-rate code at +2, start frame
  at +11 (code `0x02` = 25 fps). Region starts include this origin.
- A region in the clip list is not necessarily on a playlist; deleted takes
  stay in the clip list with their stored start.

## FL Studio `.flp`

- `FLhd`/`FLdt`; event ids < 64 carry 1 byte, < 128 2 bytes, < 192 4 bytes,
  >= 192 a varint length. (Event 172 has a 3-byte value in FL 26.)
- 156 tempo (x1000), 64 new channel, 21 channel type, 203 name, 196 sample
  path, 219 levels (first int32 = pan), 233 playlist (88-byte items),
  234 automation points.

## Credit

See `NOTICE.md`.
