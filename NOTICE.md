# Acknowledgements

The Pro Tools session format is undocumented. This project builds on the
reverse-engineering work of others:

- **PTX2RPP** by Benjamin Casey (MIT License) — block layout of audio regions
  and playlist placements; the parsing approach was ported to Go.
  https://github.com/bpcrutchfield/PTX2RPP
- **libptformat** by Damien Zammit and contributors (LGPL) — the `.ptx`
  obfuscation scheme and general file structure.
  https://github.com/zamaudio/ptformat
- **PyFLP** (GPL) — reference for the FL Studio `.flp` event layout.
  https://github.com/demberto/PyFLP

No code from these projects is bundled, but their documentation of the
formats made this tool possible.

Pro Tools is a trademark of Avid Technology. FL Studio is a trademark of
Image-Line. This project is not affiliated with or endorsed by either.
