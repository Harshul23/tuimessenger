package tui

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Linux counterparts of the macOS-only pieces (Finder dialog, pasteboard, QuickTime). Each takes goos/getenv/have so
// tests can pick a platform.

// systemPlayer names what plays a video when mpv isn't installed.
func systemPlayer() string {
	if runtime.GOOS == "darwin" {
		return "Playing in QuickTime"
	}
	return "Opening in your video player"
}

// systemPickerName is the file_picker "finder" choice as people know it here.
func systemPickerName() string {
	if runtime.GOOS == "darwin" {
		return "Finder"
	}
	return "file dialog"
}

// errNoClipboardTool means neither wl-paste (Wayland) nor xclip (X11) is there to read the clipboard.
var errNoClipboardTool = errors.New("no clipboard tool")

// clipboardCommand is the command that prints the clipboard as mime: wl-paste on Wayland, xclip on X11.
func clipboardCommand(mime string, getenv func(string) string, have func(string) bool) ([]string, error) {
	if getenv("WAYLAND_DISPLAY") != "" && have("wl-paste") {
		return []string{"wl-paste", "--no-newline", "--type", mime}, nil
	}
	if getenv("DISPLAY") != "" && have("xclip") {
		return []string{"xclip", "-selection", "clipboard", "-target", mime, "-out"}, nil
	}
	return nil, errNoClipboardTool
}

// clipboardTool is the package to suggest when errNoClipboardTool: wl-clipboard on Wayland, else xclip.
func clipboardTool(getenv func(string) string) string {
	if getenv("WAYLAND_DISPLAY") != "" {
		return "wl-clipboard"
	}
	return "xclip"
}

// readClipboard returns the clipboard as mime; empty when it holds something else.
func readClipboard(mime string) ([]byte, error) {
	args, err := clipboardCommand(mime, os.Getenv, havePath)
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		// Both tools fail when the clipboard has no such type; that's "nothing there", not an error.
		return nil, nil
	}
	return out, nil
}

// filesFromURIList reads a text/uri-list (files copied in a Linux file manager) into local paths.
func filesFromURIList(list string) []string {
	var paths []string
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || u.Scheme != "file" || u.Path == "" {
			continue
		}
		paths = append(paths, u.Path)
	}
	return paths
}

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// zenityArgs opens zenity's dialog for several files, one path per line.
var zenityArgs = []string{"--file-selection", "--multiple", "--separator=\n", "--title=Send to chat"}

// pickFilesZenity is the Linux file dialog; nil with no error means cancelled.
func pickFilesZenity() ([]string, error) {
	zenity, err := exec.LookPath("zenity")
	if err != nil {
		return nil, fmt.Errorf("no file dialog: install zenity, or switch the file picker to yazi")
	}
	out, err := exec.Command(zenity, zenityArgs...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil // closed or cancelled
		}
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func isPNG(data []byte) bool {
	return bytes.HasPrefix(data, pngMagic)
}
