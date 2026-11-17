package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"go.mau.fi/mauview"
)

// Found by fuzzing mauview's InputArea: bytes that aren't UTF-8, wrapped at width 13.
const crashingText = " word \xff\xfeword \xe2\x82word \x80abcabcword \xe2\x82abc\xff\xfe\x80abcword \xe2\x82abcabc" +
	"\xe9\xe2\x82\xe9\xff\xfe\xff\xfe\xff\xfe\xe2\x82\xe2\x82\xff\xfe\xff\xfe\xc3\xe2\x82\xc3"

func prepareDraw(input *mauview.InputArea, width int) (panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprint(r)
		}
	}()
	input.PrepareDraw(width)
	return ""
}

// A paste on Linux put raw clipboard bytes in the message box; mauview crashed drawing them, and again on every
// start because the text was saved as a draft. validInputText runs before every draw.
func TestInvalidUTF8InMessageBox(t *testing.T) {
	raw := mauview.NewInputArea()
	raw.SetText(crashingText)
	if prepareDraw(raw, 13) == "" {
		t.Fatal("mauview no longer crashes on this text; the test needs a new example")
	}

	input := mauview.NewInputArea()
	input.SetText(crashingText)
	validInputText(input)
	if p := prepareDraw(input, 13); p != "" {
		t.Fatalf("still crashes after validInputText: %s", p)
	}
	if text := input.GetText(); !utf8.ValidString(text) || !strings.Contains(text, "\uFFFD") || !strings.Contains(text, "word") {
		t.Errorf("want the text kept with � for the bad bytes, got %q", text)
	}

	ok := mauview.NewInputArea()
	ok.SetText("héllo 👍")
	validInputText(ok)
	if ok.GetText() != "héllo 👍" {
		t.Errorf("valid text must be left alone, got %q", ok.GetText())
	}
}

// The paste fix-up swaps in the clipboard's text when it matches the paste; the clipboard's bad bytes must not
// come along.
func TestRestoredPasteIsValidUTF8(t *testing.T) {
	clip := strings.ToValidUTF8("caf\xe9\n\tbar", "\uFFFD") // what readClipboardText returns
	got, ok := restorePasteWhitespace("caf\uFFFDbar", clip)
	if !ok || got != "caf\uFFFD\n\tbar" || !utf8.ValidString(got) {
		t.Errorf("got %q %v", got, ok)
	}
}
