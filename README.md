# FormatConverter

🖱️ Right-click context menu tool for Windows to convert images, audio, and video files (PNG, JPG, WEBP, MP3, MP4, and more) via ffmpeg. Single dependency-free Go binary


A lightweight Windows right-click tool for converting images, audio, and video files to a different format

Right-click a file → **Convert Format** → pick a target format from a short list → done. Select many files and use **Send to → FormatConverter** to convert them all in one go.

## Why

The typical "convert to X" context-menu tools either ship one menu entry per format (cluttering your right-click menu) or rely on Windows' own file-type registration, which is often missing or wrong for formats like `.webp` or `.mkv`. FormatConverter avoids both problems:

- **One menu entry**, not a dozen. Clicking it opens a tiny console prompt listing only the formats that make sense for the file you clicked (image, audio, or video).
- **Its own extension list** decides the file's category — not Windows' registry, so `.webp`, `.mkv`, and similar formats are recognized reliably regardless of what's installed on your system.
- **Batch-friendly**: select multiple files and use "Send to" to convert them all with a single prompt, instead of Windows spawning one console per file (which is what happens with plain context-menu verbs on multi-select — a Windows limitation, not this tool's).
- **A single, dependency-free Go binary.** The actual encoding is delegated to [ffmpeg](https://ffmpeg.org/), which covers nearly every common image/audio/video format on its own.

## Features

- Supported source types and the formats offered for each:

  | Source type | Extensions recognized | Offered targets |
  |---|---|---|
  | Image | `png jpg jpeg webp bmp gif tiff tif ico` | PNG, JPG, WEBP, BMP, GIF, TIFF, ICO |
  | Audio | `mp3 wav flac aac ogg m4a wma opus` | MP3, WAV, FLAC, AAC, OGG, M4A |
  | Video | `mp4 webm mkv avi mov wmv flv m4v 3gp` | MP4, WEBM, MKV, AVI, MOV **+** MP3, WAV, FLAC, AAC, OGG, M4A (extract audio) |

- Per-user install (`HKEY_CURRENT_USER`) — **no administrator rights required**.
- Output files are written next to the input, with a `_1`, `_2`, ... suffix added automatically if a file with that name already exists (nothing is ever silently overwritten).
- `--install` / `--uninstall` clean up after themselves, including entries left by earlier versions of this tool.

## Requirements

- Windows 10/11
- [ffmpeg](https://www.gyan.dev/ffmpeg/builds/) — either on your `PATH`, or `ffmpeg.exe` copied into the same folder as `converter.exe`.

`converter.exe` itself has no runtime dependencies — it's a single static binary.

## Installation

1. Download `converter.exe` (or build it yourself, see below) into a permanent folder, e.g. `C:\Tools\FormatConverter\converter.exe`.
   Don't move it after installing — the registry entry points at this exact path. If you do move it, run `--uninstall` then `--install` again from the new location.
2. Make sure `ffmpeg.exe` is on `PATH`, or copy it into the same folder.
3. Open a terminal (cmd or PowerShell) in that folder and run:
   ```bat
   converter.exe --install
   ```
4. Right-click any file → **Convert Format**. A console window opens with a numbered list of target formats for that file; type a number and press Enter.
   - On Windows 11 this may be tucked under **"Show more options"** (or `Shift + Right-click`).
   - For multiple files at once: select them, right-click → **Send to → FormatConverter**. This runs the whole batch through a single prompt instead of opening one console per file.
   - If nothing shows up, restart Explorer or log off/on.

### Uninstall

```bat
converter.exe --uninstall
```

## Usage from the command line

```bat
REM Interactive (what the context menu / Send To use) — prompts for a format:
converter.exe "C:\photo.png"

REM Direct / scripted — no prompt:
converter.exe "C:\photo.png" --to webp
converter.exe "a.jpg" "b.jpg" "c.jpg" --to png
converter.exe "video.mp4" --to mp3
```

If the target extension you picked isn't supported, or the input file doesn't exist, FormatConverter prints a clear error and leaves your files untouched.

## Building from source

Only the Go standard library is used — no external modules, no `go.sum`, no internet access needed at build time.

### Option A — build on Windows directly

1. **Install Go.** Download the Windows installer from <https://go.dev/dl/> (the `.msi` file, e.g. `go1.23.x.windows-amd64.msi`) and run it — it adds `go` to your `PATH` automatically.
2. Open a **new** Command Prompt or PowerShell window (must be opened *after* installing Go, so it picks up the updated `PATH`) and confirm it worked:
   ```bat
   go version
   ```
   You should see something like `go version go1.23.x windows/amd64`.
3. Put `converter.go` and `go.mod` in a folder, e.g. `C:\...\convertor`, and `cd` into it:
   ```bat
   cd C:\...\convertor
   ```
4. Build:
   ```bat
   go build -trimpath -o converter.exe converter.go
   ```
   This produces `converter.exe` in the same folder. That's it — no other steps, no linking, no separate runtime to ship.

### Option B — cross-compile from Linux/macOS

If you're building on Linux or macOS (e.g. in CI, or if that's your dev machine) and just want to produce the Windows `.exe`:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o converter.exe converter.go
```

Go's cross-compilation "just works" here because the program only uses the standard library — no cgo, no platform-specific dependencies.

### Notes on the build flags

- `-trimpath` strips local filesystem paths (like `C:\...\convertor`) from the compiled binary, so they don't leak into the `.exe`.
- We deliberately **don't** add `-s -w` (symbol/debug-info stripping) or run the result through a packer like UPX — see [Antivirus false positives](#antivirus-false-positives) below for why.
- No `-ldflags`, no external linker needed; `CGO_ENABLED=0` (default when cross-compiling, and safe to set explicitly on Windows too) keeps the build fully static.

### After building

Rebuilding doesn't automatically update the installed context menu / Send To shortcut — if you already ran `--install` before, and `converter.exe` stayed at the same path, you don't need to do anything. If you moved or renamed the file, re-run:

```bat
converter.exe --uninstall
converter.exe --install
```

### Verifying the build

```bat
converter.exe --help
```

should print the usage text shown in [Usage from the command line](#usage-from-the-command-line) above. If it does, the binary is good.

## Adding a new format

To offer a new **target** format, add a line to the relevant list in `converter.go`:

```go
var audioOnlyTargets = []target{
    {"MP3", "mp3"}, {"WAV", "wav"}, {"FLAC", "flac"},
    {"AAC", "aac"}, {"OGG", "ogg"}, {"M4A", "m4a"},
    {"OPUS", "opus"}, // new
}
```

To recognize a new **source** extension (so the menu appears for it at all), add it to `imageExts` / `audioExts` / `videoExts`. Since the menu is registered for `"*"` (all files), no registry changes are needed — just rebuild.

## Antivirus false positives

Unsigned `.exe` files — especially small Go binaries that write to the registry — are sometimes flagged by antivirus heuristics even when they do nothing malicious. There's no way to guarantee this never happens, but a few things reduce the risk:

1. **Don't pack the binary** (e.g. with UPX). Packing is exactly what malware droppers do, and it's one of the more reliable heuristic triggers.
2. **Don't strip symbols** (`-s -w`). Stripped, minified binaries look more "hidden" to some scanners.
3. **Code-sign the binary.** This is the actual fix. Paid options include DigiCert, Sectigo, and SSL.com; [SignPath.io](https://signpath.io) offers free signing for open-source projects.
4. **Report false positives.** For Microsoft Defender: <https://www.microsoft.com/wdsi/filesubmission>. Most vendors have a similar submission page.
5. Registry writes only happen when you explicitly run `--install` — the binary does nothing on its own when double-clicked without arguments.

## How it works internally

- `converter.exe --install` registers one context-menu verb under `HKCU\Software\Classes\*\shell\ConvertFormatPick` and creates a shortcut in the current user's `Send To` folder (`%APPDATA%\Microsoft\Windows\SendTo`) via a short PowerShell/WScript.Shell call — this is the standard, script-free way to create a `.lnk` file.
- When invoked (by either entry point), the program looks at the extension of the first selected file, classifies it as image/audio/video using its own maps, and prints the matching list of target formats.
- Conversion itself just shells out to `ffmpeg -i <input> [flags] <output>`, adding `-vn` when extracting audio from video, and reasonable quality flags for WebP/JPEG.

## Limitations

- Requires ffmpeg to be installed/reachable; if it isn't, you get a clear error message telling you where to get it.
- Large video files can take a while to convert; the console window stays open until the job finishes.
- The context menu appears on every file type. Using it on an unsupported file (e.g. `.txt`) prints "unsupported file type" and doesn't touch the file.
- Multi-select via the plain right-click verb still opens one console per file — this is a Windows shell limitation for registry-only context-menu entries. Use **Send To** for true single-console batch conversion.

## License

MIT — do whatever you want with this.