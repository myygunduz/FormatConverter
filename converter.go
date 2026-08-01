// converter.go
//
// FormatConverter — a small, dependency-free (standard library only) Go
// program that lets you convert images, audio, and video files to a
// different format from the Windows right-click context menu. The actual
// encoding work is delegated to ffmpeg, which covers nearly every common
// image/audio/video format on its own.
//
// Design notes:
//
//   - The context menu shows a SINGLE entry ("Convert Format"), not one
//     entry per output format. When clicked, the program looks at the
//     input file's extension using its OWN extension list (not Windows'
//     PerceivedType registry classification, which is unreliable for
//     formats like .webp or .mkv on some systems) and prints a short
//     numbered list of applicable target formats in the console.
//
//   - Windows' classic context-menu "command" verb invokes the target
//     program once PER SELECTED FILE when multiple files are selected
//     (there is no registry-only way around this). To convert a batch of
//     files in a single console session, this tool also installs a
//     shortcut in the "Send To" folder: selecting multiple files and
//     using Right-click > Send to > FormatConverter invokes the program
//     exactly once, with all selected files passed as arguments.
//
// Usage:
//
//	converter.exe --install                  install context menu + Send To entry (per-user, no admin needed)
//	converter.exe --uninstall                remove both
//	converter.exe "C:\photo.png"             interactive mode: prompts for target format (what the menu uses)
//	converter.exe "a.jpg" "b.jpg" --to png   direct/scripted conversion, no prompt
//
// Build (produces a Windows .exe; cross-compiling from Linux/macOS works fine):
//
//	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o converter.exe converter.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------
// Category detection: entirely based on our own extension lists.
// We deliberately do NOT rely on Windows' file-type registration
// (PerceivedType), since it's missing or wrong for some extensions
// (.webp, .mkv, ...) on many systems.
// ---------------------------------------------------------------------

var audioExts = map[string]bool{
	"mp3": true, "wav": true, "flac": true, "aac": true, "ogg": true, "m4a": true, "wma": true, "opus": true,
}

var imageExts = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "webp": true, "bmp": true, "gif": true, "tiff": true, "tif": true, "ico": true,
}

var videoExts = map[string]bool{
	"mp4": true, "webm": true, "mkv": true, "avi": true, "mov": true, "wmv": true, "flv": true, "m4v": true, "3gp": true,
}

func category(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if audioExts[ext] {
		return "audio"
	}
	if imageExts[ext] {
		return "image"
	}
	if videoExts[ext] {
		return "video"
	}
	return "unknown"
}

// ---------------------------------------------------------------------
// Target format lists (what to offer, based on the source category)
// ---------------------------------------------------------------------

type target struct {
	label string
	ext   string
}

var imageTargets = []target{
	{"PNG", "png"}, {"JPG", "jpg"}, {"WEBP", "webp"},
	{"BMP", "bmp"}, {"GIF", "gif"}, {"TIFF", "tiff"}, {"ICO", "ico"},
}

var audioOnlyTargets = []target{
	{"MP3", "mp3"}, {"WAV", "wav"}, {"FLAC", "flac"}, {"AAC", "aac"}, {"OGG", "ogg"}, {"M4A", "m4a"},
}

var videoOnlyTargets = []target{
	{"MP4", "mp4"}, {"WEBM", "webm"}, {"MKV", "mkv"}, {"AVI", "avi"}, {"MOV", "mov"},
}

// targetsFor returns the list of offered targets for a given source
// category. Video sources get both video formats and "extract audio"
// options in the same list.
func targetsFor(cat string) []target {
	switch cat {
	case "image":
		return imageTargets
	case "audio":
		return audioOnlyTargets
	case "video":
		return append(append([]target{}, videoOnlyTargets...), audioOnlyTargets...)
	default:
		return nil
	}
}

// ---------------------------------------------------------------------
// Locating ffmpeg
// ---------------------------------------------------------------------

func findFFmpeg() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	if exePath, err := os.Executable(); err == nil {
		for _, name := range []string{"ffmpeg.exe", "ffmpeg"} {
			candidate := filepath.Join(filepath.Dir(exePath), name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("ffmpeg not found (not on PATH and not next to the program)")
}

// ---------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------

func uniqueOutputPath(dir, stem, ext string) string {
	out := filepath.Join(dir, stem+"."+ext)
	if _, err := os.Stat(out); err != nil {
		return out
	}
	for i := 1; ; i++ {
		out = filepath.Join(dir, fmt.Sprintf("%s_%d.%s", stem, i, ext))
		if _, err := os.Stat(out); err != nil {
			return out
		}
	}
}

func convertFile(ffmpegPath, input, targetExt string) error {
	targetExt = strings.ToLower(strings.TrimPrefix(targetExt, "."))

	info, err := os.Stat(input)
	if err != nil || info.IsDir() {
		return fmt.Errorf("file not found: %s", input)
	}

	dir := filepath.Dir(input)
	stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	output := uniqueOutputPath(dir, stem, targetExt)

	args := []string{"-y", "-i", input}

	if category(targetExt) == "audio" {
		args = append(args, "-vn") // drop the video stream when extracting audio
	}
	if targetExt == "webp" {
		args = append(args, "-quality", "80")
	}
	if targetExt == "jpg" || targetExt == "jpeg" {
		args = append(args, "-q:v", "3")
	}

	args = append(args, output)

	cmd := exec.Command(ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\n%s", err, lastLines(string(out), 6))
	}
	fmt.Printf("OK: %s -> %s\n", input, output)
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------
// Context menu: a SINGLE entry, shown for every file ("*").
// Format selection happens in the console after clicking it, not in the
// menu itself.
// (Registered under HKEY_CURRENT_USER -> no admin rights required)
// ---------------------------------------------------------------------

const menuKey = `HKCU\Software\Classes\*\shell\ConvertFormatPick`

// Keys/entries that may be left over from earlier versions of this tool.
var legacyKeys = []string{
	`HKCU\Software\Classes\*\shell\ConvertFormat`,
	`HKCU\Software\Classes\SystemFileAssociations\image\shell`,
	`HKCU\Software\Classes\SystemFileAssociations\audio\shell`,
	`HKCU\Software\Classes\SystemFileAssociations\video\shell`,
}

var legacyFlatIDs = []string{
	"01_png", "02_jpg", "03_webp", "04_bmp", "05_gif", "06_tiff",
	"07_mp3", "08_wav", "09_flac", "10_mp4", "11_webm", "12_mkv",
}

func regAdd(key, value, data string) error {
	var args []string
	if value == "" {
		args = []string{"add", key, "/ve", "/t", "REG_SZ", "/d", data, "/f"}
	} else {
		args = []string{"add", key, "/v", value, "/t", "REG_SZ", "/d", data, "/f"}
	}
	cmd := exec.Command("reg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add failed (key=%s value=%s): %v\n%s", key, value, err, string(out))
	}
	return nil
}

func regDeleteQuiet(key string) {
	cmd := exec.Command("reg", "delete", key, "/f")
	_, _ = cmd.CombinedOutput()
}

func cleanupLegacy() {
	for _, k := range legacyKeys {
		regDeleteQuiet(k)
	}
	for _, id := range legacyFlatIDs {
		regDeleteQuiet(`HKCU\Software\Classes\*\shell\ConvertTo_` + id)
	}
	regDeleteQuiet(menuKey)
}

// ---------------------------------------------------------------------
// "Send To" integration: lets the user select MANY files and convert
// them all in a single console session (Right-click > Send to > ...).
// This is the only registry-free, script-free way to get Explorer to
// invoke a program once with every selected file as an argument.
// ---------------------------------------------------------------------

func sendToPath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA environment variable is not set")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "SendTo", "FormatConverter.lnk"), nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func installSendTo(exePath string) error {
	linkPath, err := sendToPath()
	if err != nil {
		return err
	}
	script := fmt.Sprintf(
		`$ws = New-Object -ComObject WScript.Shell; $sc = $ws.CreateShortcut('%s'); $sc.TargetPath = '%s'; $sc.IconLocation = '%s,0'; $sc.Save()`,
		escapePS(linkPath), escapePS(exePath), escapePS(exePath),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create Send To shortcut: %v\n%s", err, string(out))
	}
	return nil
}

func uninstallSendTo() error {
	linkPath, err := sendToPath()
	if err != nil {
		return err
	}
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove Send To shortcut: %v", err)
	}
	return nil
}

func installMenu() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.Abs(exePath)

	cleanupLegacy() // remove keys left over from earlier attempts

	if err := regAdd(menuKey, "", "Convert Format"); err != nil {
		return err
	}
	if err := regAdd(menuKey, "Icon", "shell32.dll,-16769"); err != nil {
		return err
	}
	cmdVal := fmt.Sprintf(`"%s" "%%1"`, exePath)
	if err := regAdd(menuKey+`\command`, "", cmdVal); err != nil {
		return err
	}

	sendToErr := installSendTo(exePath)

	fmt.Println("Context menu installed: single 'Convert Format' entry.")
	fmt.Println("Click it on a file and you'll be asked which format to convert to")
	fmt.Println("(the list adapts automatically to whether it's an image, audio, or video file).")
	if sendToErr != nil {
		fmt.Println("Note: could not set up the 'Send To' entry for batch conversions:", sendToErr)
	} else {
		fmt.Println("For converting MANY files at once in a single console window, use:")
		fmt.Println("  select the files -> right-click -> Send to -> FormatConverter")
	}
	fmt.Println("On Windows 11 the entry may be hidden under 'Show more options' (or Shift+Right-click).")
	fmt.Println("(If it doesn't show up, restart Explorer or log off/on.)")
	return nil
}

func uninstallMenu() error {
	cleanupLegacy()
	if err := uninstallSendTo(); err != nil {
		fmt.Println("Note: could not remove the 'Send To' shortcut:", err)
	}
	fmt.Println("Context menu and Send To entry removed.")
	return nil
}

// ---------------------------------------------------------------------
// Interactive format picker (this is what runs when the menu item, or
// the Send To shortcut, is clicked)
// ---------------------------------------------------------------------

func pickTarget(files []string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no file specified")
	}
	ext := strings.TrimPrefix(filepath.Ext(files[0]), ".")
	cat := category(ext)
	targets := targetsFor(cat)
	if targets == nil {
		return "", fmt.Errorf("unsupported file type: .%s (must be an image, audio, or video file)", ext)
	}

	fmt.Printf("File: %s\n", filepath.Base(files[0]))
	if len(files) > 1 {
		fmt.Printf("(+%d more file(s), all will be converted to the same format)\n", len(files)-1)
	}
	fmt.Println("Convert to which format?")
	for i, t := range targets {
		fmt.Printf("  %2d) %s\n", i+1, t.label)
	}
	fmt.Print("Choice (number), or q to cancel: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "q" || line == "Q" || line == "" {
		return "", fmt.Errorf("cancelled")
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(targets) {
		return "", fmt.Errorf("invalid choice: %s", line)
	}
	return targets[n-1].ext, nil
}

// ---------------------------------------------------------------------
// main
// ---------------------------------------------------------------------

func printUsage() {
	fmt.Println(`FormatConverter - usage:
  converter.exe --install                  Install context menu + Send To entry
  converter.exe --uninstall                Remove both
  converter.exe <file> [<file2> ...]       Interactive: prompts for target format (used by the menu)
  converter.exe <file> [...] --to <fmt>    Direct conversion, no prompt (for scripting)

Examples:
  converter.exe "C:\photo.png"
  converter.exe "C:\photo.png" --to webp
  converter.exe "video.mp4" --to mp3`)
}

func pauseIfInteractive() {
	fmt.Println("\nPress Enter to close...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "--install":
		if err := installMenu(); err != nil {
			fmt.Println("Error:", err)
			pauseIfInteractive()
			os.Exit(1)
		}
		pauseIfInteractive()
		return
	case "--uninstall":
		if err := uninstallMenu(); err != nil {
			fmt.Println("Error:", err)
			pauseIfInteractive()
			os.Exit(1)
		}
		pauseIfInteractive()
		return
	case "-h", "--help", "/?":
		printUsage()
		return
	}

	var files []string
	var target string
	for i := 0; i < len(args); i++ {
		if args[i] == "--to" && i+1 < len(args) {
			target = args[i+1]
			i++
			continue
		}
		files = append(files, args[i])
	}

	if len(files) == 0 {
		printUsage()
		return
	}

	// No --to given (the normal path when invoked from the context menu
	// or the Send To shortcut): ask interactively.
	if target == "" {
		t, err := pickTarget(files)
		if err != nil {
			fmt.Println("Error:", err)
			pauseIfInteractive()
			os.Exit(1)
		}
		target = t
	}

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		fmt.Println("Error:", err)
		fmt.Println("Download ffmpeg.exe from https://www.gyan.dev/ffmpeg/builds/ and add it to PATH,")
		fmt.Println("or copy it next to converter.exe.")
		pauseIfInteractive()
		os.Exit(1)
	}

	failed := 0
	for _, f := range files {
		if err := convertFile(ffmpegPath, f, target); err != nil {
			fmt.Println("ERROR:", err)
			failed++
		}
	}

	if failed > 0 {
		fmt.Printf("\n%d/%d file(s) failed.\n", failed, len(files))
		pauseIfInteractive()
		os.Exit(1)
	}
	fmt.Printf("\n%d file(s) converted successfully.\n", len(files))
	pauseIfInteractive()
}