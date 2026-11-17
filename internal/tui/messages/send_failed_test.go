package messages

import (
	"testing"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

func TestSendFailed(t *testing.T) {
	cases := map[string]bool{
		"":                      false, // confirmed by the server
		"not sent":              false, // on its way, or sent and waiting for the sync echo
		"M_FORBIDDEN: no power": true,
	}
	for sendError, want := range cases {
		if got := SendFailed(&database.Event{SendError: sendError}); got != want {
			t.Errorf("SendError %q: got %v, want %v", sendError, got, want)
		}
	}
	if SendFailed(nil) {
		t.Error("no event is not a failure")
	}
}
