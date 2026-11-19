//go:build darwin || linux

package messages

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// fakeTerminal answers kittyQuery on the other end of a socket pair, like a terminal would.
func fakeTerminal(t *testing.T, answer string) int {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fds[0]); unix.Close(fds[1]) })
	go func() {
		buf := make([]byte, 256)
		var got string
		for !strings.HasSuffix(got, "\x1b[c") {
			n, err := unix.Read(fds[1], buf)
			if err != nil || n <= 0 {
				return
			}
			got += string(buf[:n])
		}
		if got != kittyQuery {
			t.Errorf("the terminal got %q", got)
		}
		// Answered in two pieces, as a terminal may.
		half := len(answer) / 2
		_, _ = unix.Write(fds[1], []byte(answer[:half]))
		time.Sleep(10 * time.Millisecond)
		_, _ = unix.Write(fds[1], []byte(answer[half:]))
	}()
	return fds[0]
}

func TestKittyProbe(t *testing.T) {
	cases := map[string]struct {
		answer string
		want   bool
	}{
		// herdr panes are Ghostty terminals: graphics on → OK, then the device attributes.
		"herdr, graphics on":  {"\x1b_Gi=31;OK\x1b\\\x1b[?62;22;52c", true},
		"herdr, graphics off": {"\x1b[?62;22c", false},
		"error reply":         {"\x1b_Gi=31;ENOTSUPPORTED:no\x1b\\\x1b[?1;2c", false},
	}
	for name, c := range cases {
		start := time.Now()
		reply := kittyExchange(fakeTerminal(t, c.answer), time.Second)
		if got := kittyReplyOK(reply); got != c.want {
			t.Errorf("%s: got %v from %q", name, got, reply)
		}
		if time.Since(start) > 500*time.Millisecond {
			t.Errorf("%s: should stop at the device attributes, not wait for the timeout", name)
		}
	}
}

func TestDetectKittyGraphicsRespectsSetting(t *testing.T) {
	orig := kittyTerminalSupported
	t.Cleanup(func() { kittyTerminalSupported = orig })
	kittyTerminalSupported = false
	DetectKittyGraphics("ansi") // must not touch the terminal
	if kittyTerminalSupported {
		t.Error(`image_protocol "ansi" must not probe`)
	}
	kittyTerminalSupported = true
	DetectKittyGraphics("") // already known by name: nothing to ask
	if !kittyTerminalSupported {
		t.Error("a terminal known by name stays supported")
	}
}
