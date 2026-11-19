package messages

import (
	"regexp"
	"strings"
	"time"

	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// kittyQuery asks whether the terminal speaks kitty graphics (a 1×1 image it checks but doesn't keep), then for the
// primary device attributes, which every terminal answers: the reply ends there even where the first is ignored.
const kittyQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c"

// kittyProbeTimeout bounds the wait at startup for a terminal that answers neither.
const kittyProbeTimeout = 300 * time.Millisecond

var deviceAttributes = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

func kittyReplyOK(reply []byte) bool {
	return strings.Contains(string(reply), "\x1b_Gi=31;OK")
}

func kittyReplyDone(reply []byte) bool {
	return deviceAttributes.Match(reply)
}

// DetectKittyGraphics asks the terminal itself when its name doesn't say (e.g. TERM_PROGRAM=herdr: herdr's panes are
// Ghostty terminals that answer while its Kitty graphics are on). Call before the screen takes over the terminal, or
// the answer would arrive as keys. protocol is the image_protocol setting; only "auto" probes.
func DetectKittyGraphics(protocol string) {
	if kittyTerminalSupported || (protocol != "" && protocol != "auto") {
		return
	}
	kittyTerminalSupported = probeKittyGraphics(kittyProbeTimeout)
	debug.Print("Kitty graphics probe:", kittyTerminalSupported)
}
