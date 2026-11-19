//go:build darwin || linux

package messages

import (
	"time"

	"golang.org/x/sys/unix"
)

// probeKittyGraphics sends kittyQuery to the terminal and reads its answer. Raw syscalls rather than an os.File: Go
// would hand a tty to its poller (on Linux), which ignores VTIME and could wait forever on a silent terminal.
func probeKittyGraphics(timeout time.Duration) bool {
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	old, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		return false
	}
	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1 // a read returns after 0.1 s without input
	if err = unix.IoctlSetTermios(fd, setTermios, &raw); err != nil {
		return false
	}
	defer func() { _ = unix.IoctlSetTermios(fd, setTermios, old) }()
	return kittyReplyOK(kittyExchange(fd, timeout))
}

// kittyExchange writes kittyQuery to fd and reads until the device attributes arrive or the time is up.
func kittyExchange(fd int, timeout time.Duration) []byte {
	if _, err := unix.Write(fd, []byte(kittyQuery)); err != nil {
		return nil
	}
	var reply []byte
	buf := make([]byte, 256)
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline) && !kittyReplyDone(reply); {
		if n, err := unix.Read(fd, buf); err == nil && n > 0 {
			reply = append(reply, buf[:n]...)
		}
	}
	return reply
}
