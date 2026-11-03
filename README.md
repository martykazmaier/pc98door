# pc98door

pc98door is a PC-9801 door for EleBBS and SyncTERM. It lists the disk images in `roms/`, and Enter starts the highlighted disk in [Neko Project II kai](https://github.com/libretro/NP2kai). Ctrl-Q leaves the game and returns to the menu.

Copyright (C) 2026 Martin Kazmaier. Licensed under the GNU General Public License, version 2 or any later version. See [LICENSE](LICENSE).

## Run

The build that plays disks is the 32-bit Windows program. Put these beside `pc98door.exe`:

- `np2kai_libretro.dll`, included in the Windows release
- `bios/` with `bios.rom`, `font.rom`, `itf.rom`, and `sound.rom`
- `roms/` with Anex86 `.hdi` disk images
- `save/` is created per caller

`door32.sys` is read from the BBS working directory. The caller's save folder is the real name on line 6 of that file. `-L` forces the local console instead of the inherited socket.

```
pc98door [-L]
```

Firmware and disk images are not part of this repository.

Linux and ARM builds compile the menu and the local terminal. They do not include a Neko Project core, so starting a disk reports that the emulator runs as a 32-bit Windows program.

## Releases

| File | System |
| --- | --- |
| `pc98door-windows-386.zip` | 32-bit Windows, with the Neko Project core |
| `pc98door-linux-amd64.tar.gz` | 64-bit Linux |
| `pc98door-linux-386.tar.gz` | 32-bit Linux |
| `pc98door-linux-arm64.tar.gz` | 64-bit ARM Linux |

## Other programs

The Windows core is Neko Project II kai built by libretro (`np2kai_libretro.dll`). That program is GPL-2.0, and its source is [libretro/NP2kai](https://github.com/libretro/NP2kai). The nightly this door was linked against is published at `https://buildbot.libretro.com/nightly/windows/x86/latest/np2kai_libretro.dll.zip`.

Tests decode JPEG XL with [jxl-go](third_party/jxl-go), which is MIT, copyright Ken Faulkner.
