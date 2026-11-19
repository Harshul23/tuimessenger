package messages

import (
	"strings"
	"testing"

	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// A WhatsApp document: the name is in filename and the body is empty when there's no caption. It used to show as
// ": http://127.0.0.1:8008/_matrix/client/v1/media/download/…", a URL that only works with your login.
const whatsappPDF = `{"msgtype":"m.file","body":"","filename":"AI_Engineer_Interview_Questions.pdf",
	"url":"mxc://tuimessenger.localhost/dm-whatsapp-abc","info":{"mimetype":"application/pdf","size":1258291}}`

func TestFileNameFromFilenameField(t *testing.T) {
	fm := parseFileMessage(t, whatsappPDF)
	if got := fm.DisplayName(); got != "AI_Engineer_Interview_Questions.pdf" {
		t.Errorf("DisplayName = %q", got)
	}
	if got, want := fm.fileLine(), "\uf1c1 AI_Engineer_Interview_Questions.pdf · PDF · 1.2 MB"; got != want {
		t.Errorf("fileLine = %q, want %q", got, want)
	}
	if got := fm.PlainText(); got != "AI_Engineer_Interview_Questions.pdf" || strings.Contains(got, "http") {
		t.Errorf("PlainText = %q: the name, without the local download URL", got)
	}

	fm.CalculateBuffer(config.UserPreferences{}, 80, &UIMessage{})
	var drawn strings.Builder
	for _, line := range fm.buffer {
		drawn.WriteString(line.String() + "\n")
	}
	if text := drawn.String(); !strings.Contains(text, "AI_Engineer_Interview_Questions.pdf") || strings.Contains(text, "127.0.0.1") ||
		strings.HasPrefix(text, ":") {
		t.Errorf("drawn as %q", text)
	}
}

func TestFileNameFallbacksAndCaption(t *testing.T) {
	// Older bridges: the name is the body.
	if got := parseFileMessage(t, `{"msgtype":"m.file","body":"notes.txt","url":"mxc://x/a"}`).DisplayName(); got != "notes.txt" {
		t.Errorf("name in body: %q", got)
	}
	// Neither: say what it is.
	if got := parseFileMessage(t, `{"msgtype":"m.video","body":"","url":"mxc://x/a"}`).DisplayName(); got != "Video" {
		t.Errorf("no name: %q", got)
	}
	// filename plus a caption: both, on their own lines.
	fm := parseFileMessage(t, `{"msgtype":"m.file","body":"for tomorrow","filename":"slides.pptx","url":"mxc://x/a"}`)
	if got := fm.PlainText(); got != "slides.pptx\nfor tomorrow" {
		t.Errorf("PlainText = %q", got)
	}
	if got := fm.fileLine(); got != "\uf1c4 slides.pptx · PPTX" {
		t.Errorf("fileLine without a size = %q", got)
	}
}
