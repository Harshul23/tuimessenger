package messages

import (
	"bytes"
	"image"
	"image/png"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// Without kitty graphics, the first half-block image says so once; not when you chose ansi, nor for reply previews.
func TestLowResImageNote(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	oldSupported, oldHook := kittyTerminalSupported, OnLowResImages
	t.Cleanup(func() { kittyTerminalSupported, OnLowResImages, lowResOnce = oldSupported, oldHook, sync.Once{} })
	kittyTerminalSupported = false

	var notes atomic.Int32
	draw := func(protocol string, reply bool) int32 {
		lowResOnce = sync.Once{}
		notes.Store(0)
		OnLowResImages = func() { notes.Add(1) }
		for range 3 {
			fm := &FileMessage{Type: "m.image", kittyPNG: buf.Bytes(), imgW: 8, imgH: 8}
			fm.CalculateBuffer(config.UserPreferences{ImageProtocol: protocol}, 40, &UIMessage{IsReplyBubble: reply})
		}
		time.Sleep(20 * time.Millisecond) // the note runs in a goroutine
		return notes.Load()
	}
	if got := draw("", false); got != 1 {
		t.Errorf("auto, no kitty graphics: %d notes, want 1 for three images", got)
	}
	if got := draw("ansi", false); got != 0 {
		t.Errorf("image_protocol ansi: %d notes, want none", got)
	}
	if got := draw("", true); got != 0 {
		t.Errorf("reply previews: %d notes, want none", got)
	}
}
