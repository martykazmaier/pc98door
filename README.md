# pc98door

pc98door is a PC-9801 door for EleBBS and SyncTERM. It lists the disk images in `roms/`, and Enter starts the highlighted disk in [Neko Project II kai](https://github.com/libretro/NP2kai). Ctrl-Q leaves the game and returns to the menu.

Copyright (C) 2026 Martin Kazmaier. Licensed under the GNU General Public License, version 2 or any later version. See [LICENSE](LICENSE).

## Run

The build that plays disks is the 32-bit Windows program. Unpack it so `pc98door.exe` and `np2kai_libretro.dll` are in the same directory. Linux builds show the menu on a local terminal. Starting a disk on Linux reports that the emulator runs as a 32-bit Windows program.

`door32.sys` is read from the BBS working directory, the directory the node was in when it started the door. It is not read from beside the program. Line 6 of that file is the caller's real name. `-L` forces the local console instead of the inherited socket.

```
pc98door [-L]
```

Firmware and disk images are not part of this repository. The program creates `bios`, `roms`, and `save` beside itself when they are missing.

## Directories

These directories sit beside `pc98door.exe` on Windows and beside `pc98door` on Linux.

### bios

Shared firmware for every caller. Put these files here. Names are matched without regard to case.

- `bios.rom`
- `font.rom`
- `itf.rom`
- `sound.rom`

On Windows, the first time a disk starts, every file in `bios/` is copied to `np2sys/np2kai/`. Leave the originals in `bios/`. Do not put disk images here.

### roms

Shared disk images, opened read-only. The menu lists files in this directory whose names end in `.hdi`, `.nhd`, `.hdm`, `.hdn`, `.d88`, `.fdi`, `.tfd`, `.nfd`, `.dup`, `.img`, `.dsk`, `.hdd`, `.2hd`, `.flp`, or `.xdf`. The usual image is an Anex86 `.hdi`. Subdirectories are not listed. The first time a caller writes to a disk, that image is copied into their save directory. A path cannot leave `roms/`.

### save

One directory per caller. The folder name is the real name from line 6 of `door32.sys`. Spaces stay. `\ / : * ? " < > |` and control characters become `_`. You do not create these folders yourself. A private disk keeps the same file name it had in `roms/`. With `-L`, or when no name is available, the folder is `local`.

### np2sys

Created by the Windows program for Neko Project II kai. `np2sys/np2kai/` holds the copied firmware and the core's `np2kai.cfg`. Do not put firmware or disks here yourself. The core uses this copy so it can write its config without changing `bios/`.

`pc98door.log` is written beside the program.

## Releases

Each archive contains `file_id.diz` for the BBS file listing. The description for each build is also stored under `release/`.

| File | System |
| --- | --- |
| `pc98door-windows-386.zip` | 32-bit Windows, with the Neko Project core |
| `pc98door-linux-amd64.tar.gz` | 64-bit Linux |
| `pc98door-linux-386.tar.gz` | 32-bit Linux |
| `pc98door-linux-arm64.tar.gz` | 64-bit ARM Linux |

## Other programs

The Windows core is Neko Project II kai built by libretro. The 32-bit DLL is checked in at `third_party/np2kai_libretro.dll`. That program is GPL-2.0, and its source is [libretro/NP2kai](https://github.com/libretro/NP2kai). The nightly this copy came from is `https://buildbot.libretro.com/nightly/windows/x86/latest/np2kai_libretro.dll.zip`.

Tests decode JPEG XL with [jxl-go](third_party/jxl-go), which is MIT, copyright Ken Faulkner.
