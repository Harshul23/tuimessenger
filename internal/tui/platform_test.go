package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
func haveOnly(names ...string) func(string) bool {
	return func(n string) bool { return slices.Contains(names, n) }
}

func TestClipboardCommand(t *testing.T) {
	wayland := map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}
	if args, err := clipboardCommand("image/png", envOf(wayland), haveOnly("wl-paste", "xclip")); err != nil || args[0] != "wl-paste" || args[len(args)-1] != "image/png" {
		t.Errorf("Wayland with wl-paste: %v %v", args, err)
	}
	// Wayland without wl-clipboard but with XWayland's xclip still works.
	if args, err := clipboardCommand("image/png", envOf(wayland), haveOnly("xclip")); err != nil || args[0] != "xclip" {
		t.Errorf("XWayland with xclip: %v %v", args, err)
	}
	if args, err := clipboardCommand("text/uri-list", envOf(map[string]string{"DISPLAY": ":0"}), haveOnly("xclip")); err != nil ||
		strings.Join(args, " ") != "xclip -selection clipboard -target text/uri-list -out" {
		t.Errorf("X11 with xclip: %v %v", args, err)
	}
	if _, err := clipboardCommand("image/png", envOf(map[string]string{"DISPLAY": ":0"}), haveOnly()); err != errNoClipboardTool {
		t.Errorf("no tool: got %v, want errNoClipboardTool", err)
	}
	if clipboardTool(envOf(wayland)) != "wl-clipboard" || clipboardTool(envOf(map[string]string{"DISPLAY": ":0"})) != "xclip" {
		t.Error("the suggested package should match the display server")
	}
}

func TestFilesFromURIList(t *testing.T) {
	list := "# copied by Nautilus\r\nfile:///home/me/My%20Photo.png\r\nhttps://example.com/x\r\n\r\nfile:///tmp/notes.txt\n"
	got := filesFromURIList(list)
	if want := []string{"/home/me/My Photo.png", "/tmp/notes.txt"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// readClipboard runs the real tool: a fake wl-paste on PATH answers like the Wayland one.
func TestReadClipboardRunsTool(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
*text/uri-list*) printf 'file://%s/a.txt\n' "$CLIPDIR" ;;
*image/png*) printf '\211PNG\r\n\032\nrest' ;;
*) echo "No suitable type of content copied" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "wl-paste"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("CLIPDIR", dir)

	list, err := readClipboard("text/uri-list")
	if files := filesFromURIList(string(list)); err != nil || len(files) != 1 || files[0] != filepath.Join(dir, "a.txt") {
		t.Errorf("uri-list: %q %v", files, err)
	}
	if img, err := readClipboard("image/png"); err != nil || !isPNG(img) {
		t.Errorf("image: %q %v", img, err)
	}
	if other, err := readClipboard("text/html"); err != nil || other != nil {
		t.Errorf("a type the clipboard doesn't have is nothing, not an error: %q %v", other, err)
	}
}

// Opening goes through internal/tui/lib/open (xdg-open on Linux); macOS's open command broke links, files and video
// fallbacks on Linux.
func TestNoMacOpenCommand(t *testing.T) {
	bare := regexp.MustCompile(`exec\.Command(Context)?\([^,)]*"open"`)
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil && bare.Match(data) {
			t.Errorf("%s runs the macOS-only open command; use open.Open", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
