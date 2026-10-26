<p align="center">
  <img src="assets/logo.svg" alt="Dustblow" width="128" height="128" />
</p>

<h1 align="center">Dustblow</h1>

<p align="center">
  A minimal Nintendo Entertainment System and Famicom emulator.<br/>
  Blow the dust off the cartridge, press power, and play.
</p>

---

## Why the name

Anyone who grew up with an 8-bit console knows the routine. You pushed the gray cartridge into the slot, hit the power switch, and got nothing but a blinking gray screen. So you pulled the game back out, blew across the brass pins, pushed it down with a click, and suddenly the title music filled the room.

Dustblow takes its name from that ritual—the small moment right before a game starts.

## Design philosophy

Over the years, many emulators have started to look more like flight simulators than game consoles. Before playing a five-minute platformer, you often have to dig through nested menu bars, tweak audio buffer sliders, pick shader passes, and run a button-mapping wizard.

When you sat down in front of a real NES, there was no settings screen. You plugged in the controller and played. Dustblow is built around that same idea: keep the hardware emulation accurate under the hood, and get the software out of the way.

That is also why several features common in other emulators are intentionally missing here. There is no keybinding menu and no configuration file to edit. On the keyboard, both the classic arrow-key layout (`Arrows` + `Z`/`X`) and a laptop-friendly layout (`WASD` + `J`/`K`) are always active at the same time. If you plug in a USB or Bluetooth controller—whether it is an 8BitDo pad, an Xbox controller, or a PlayStation DualSense—it is picked up automatically, with the action buttons arranged around natural thumb placement so holding **B** to run and rolling your thumb onto **A** to jump feels right immediately.

Video options follow the same approach. Instead of exposing dozens of scaling parameters, pressing `F4` cycles through three display modes: a clean 4:3 interpolation (*Sharp Bilinear*), a warm tube-TV look with scanlines and an aperture grille mask (*CRT*), and raw square pixels (*Pixel Perfect*). All three crop the outer 8 pixels of overscan so the flickering seams at the edges of old games stay hidden, just as they did on a real television.

Even small desktop annoyances are handled quietly in the background. If you minimize the window or switch to another workspace, the game pauses and mutes itself automatically until you come back.

## Game compatibility and sound

Despite the stripped-down interface, Dustblow runs a cycle-accurate core that plays over 99% of the commercial NES and Japanese Famicom catalog out of the box—from timing-sensitive games like *Battletoads* and *Mike Tyson's Punch-Out!!* to long RPGs like *Mother* and *Fire Emblem*.

It also emulates the extra sound chips found inside Japanese Famicom cartridges, including the Konami VRC6 (*Akumajou Densetsu*), Sunsoft 5B (*Gimmick!*), Namco 163 (*Rolling Thunder*, *Megami Tensei II*), and Nintendo MMC5. Audio is routed through a model of the original console's analog filter circuit, preserving the warm low-end and rounded treble of real hardware.

## Controls

Player 1 always responds to both keyboard layouts and the primary gamepad at the same time.

| NES Button | Keyboard | Gamepad |
|:----------:|:--------:|:-------:|
| **D-Pad** | `Arrows` / `W` `A` `S` `D` | D-Pad |
| **B** *(Run / Attack)* | `Z` / `J` | South (`A` Xbox / `B` 8BitDo / `Cross`) or West (`X` Xbox / `Y` 8BitDo / `Square`) |
| **A** *(Jump / Confirm)* | `X` / `K` | East (`B` Xbox / `A` 8BitDo / `Circle`) or North (`Y` Xbox / `X` 8BitDo / `Triangle`) |
| **Select** | `Backspace` / `Right Shift` | Select / Back / Share |
| **Start** | `Enter` / `Return` | Start / Options |

For two-player games, plugging in a second controller assigns it to Player 2 automatically. If you want to play two-player with one person on the keyboard and the other on a single gamepad—or swap which controller belongs to whom—press `F6`. The game will pause and show a small overlay where you can assign ports with the directional keys.

### Shortcuts

| Key | Action |
|:---:|:-------|
| `Tab` *(hold)* | Fast-forward (3x speed) |
| `F` | Toggle fullscreen |
| `P` | Pause / Resume |
| `M` | Mute / Unmute audio |
| `F4` | Cycle display mode (*Sharp Bilinear* / *CRT Scanlines* / *Pixel Perfect*) |
| `0` – `9` | Select save state slot |
| `F5` / `F8` | Save state / Load state |
| `F6` | Controller assignment menu |
| `F12` | Save screenshot (`~/Pictures/Dustblow`) |
| `F2` / `Ctrl+R` / `Cmd+R` | Reset console |
| `Esc` | Quit |

## Saves and screenshots

Games that originally shipped with battery-backed memory or onboard EEPROM save their progress automatically while you play and when you close the window. For everything else, you have ten save state slots (`0`–`9`) that can be written with `F5` and restored with `F8`.

All save files and snapshots are kept inside `~/.dustblow` (or the path set in `DUSTBLOW_HOME`), while screenshots taken with `F12` are written as lossless PNGs to `~/Pictures/Dustblow` (or `DUSTBLOW_SCREENSHOTS_DIR`).

## Building and running

You only need a standard Go toolchain (Go 1.27+):

```bash
git clone https://github.com/aoliveti/dustblow.git
cd dustblow
go build -o bin/dustblow ./cmd/dustblow
```

Pass any `.nes` file to launch it, adding `-f` if you want to start directly in fullscreen:

```bash
bin/dustblow path/to/game.nes
bin/dustblow -f path/to/game.nes
```

## License

Released under the [BSD 3-Clause License](LICENSE).
