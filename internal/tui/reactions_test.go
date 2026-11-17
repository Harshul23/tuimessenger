package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/lib/emoji"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

// reactedMsg puts a message with the given own_reactions into the room's store, as a sync would.
func reactedMsg(room *store.RoomStore, own map[string][]id.EventID) {
	room.ApplySync(&jsoncmd.SyncRoom{
		Events: []*database.Event{{RowID: 1, TimelineRowID: 1, ID: "$msg", RoomID: room.ID,
			Sender: "@whatsapp_lid-1:beeper.local", Type: "m.room.message", Timestamp: jsontime.UM(time.Now()),
			OwnReactions: own}},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}},
	})
}

func TestOwnReactionsFromStore(t *testing.T) {
	_, room := bubbleView(t)
	rv := &RoomView{Room: room}
	reactedMsg(room, map[string][]id.EventID{"👍": {"$r1"}, "❤️": {}})
	got := rv.OwnReactions("$msg")
	if len(got) != 1 || len(got["👍"]) != 1 || got["👍"][0] != "$r1" {
		t.Errorf("got %v, want only 👍 → [$r1] (an emptied ❤️ list is not a reaction)", got)
	}
	if rv.OwnReactions("$unknown") != nil {
		t.Error("a message not in the store has no reactions")
	}
}

// A second press waits for the first one's reaction to come back down sync, so it sees it and removes it (a toggle)
// instead of sending a duplicate.
func TestWaitForOwnReactionsSeesSync(t *testing.T) {
	_, room := bubbleView(t)
	rv := &RoomView{Room: room}
	reactedMsg(room, nil)
	done := make(chan struct{})
	go func() {
		rv.waitForOwnReactions("$msg", "$r1", nil, 5*time.Second)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	reactedMsg(room, map[string][]id.EventID{"👍": {"$r1"}})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the wait should end as soon as the reaction comes back down sync")
	}
	if own := rv.OwnReactions("$msg"); len(own["👍"]) != 1 {
		t.Errorf("after the wait the next toggle sees our 👍: %v", own)
	}
}

func TestWaitForOwnReactionsGivesUp(t *testing.T) {
	_, room := bubbleView(t)
	rv := &RoomView{Room: room}
	reactedMsg(room, map[string][]id.EventID{"👍": {"$r1"}})
	start := time.Now()
	rv.waitForOwnReactions("$msg", "", []id.EventID{"$r1"}, 50*time.Millisecond)
	if time.Since(start) > time.Second {
		t.Error("a removal that never comes back must not block the next press for long")
	}
}

func TestOwnReactionsSettled(t *testing.T) {
	own := map[string][]id.EventID{"👍": {"$r1"}}
	cases := []struct {
		added   id.EventID
		removed []id.EventID
		want    bool
	}{
		{"$r1", nil, true},
		{"$r2", nil, false},
		{"", []id.EventID{"$r1"}, false},
		{"", []id.EventID{"$old"}, true},
		{"$r1", []id.EventID{"$old"}, true},
	}
	for _, c := range cases {
		if got := ownReactionsSettled(own, c.added, c.removed); got != c.want {
			t.Errorf("added %q removed %v: got %v, want %v", c.added, c.removed, got, c.want)
		}
	}
}

func TestSameReaction(t *testing.T) {
	if !sameReaction("❤", "❤️") || sameReaction("👍", "❤️") {
		t.Error("variation selectors must not matter, different emoji must")
	}
}

func TestFormatReactions(t *testing.T) {
	got := messages.FormatReactions(map[string]int{"❤️": 1, "👍": 2, "😂": 0})
	if got != "👍 2  ❤️" {
		t.Errorf("got %q", got)
	}
}

func TestEmojiSearchRanksPrefixFirst(t *testing.T) {
	res := emoji.Search("heart", 50)
	if len(res) == 0 || res[0].Shortcode[:5] != "heart" {
		t.Fatalf("prefix match should lead: %v", res)
	}
	seen := map[string]bool{}
	for _, m := range res {
		if seen[m.Emoji] {
			t.Fatalf("duplicate emoji %s", m.Emoji)
		}
		seen[m.Emoji] = true
	}
	if emoji.Search("", 10) != nil {
		t.Error("empty query should give nothing")
	}
}

func TestReactPickerPlacement(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(80, 20)
	p := &ReactPicker{}
	// Room above the message: sits directly over it.
	p.Draw(sim, 5, 10, 11)
	if p.y != 7 || p.h != 3 || p.x != 5 {
		t.Errorf("above: x=%d y=%d h=%d", p.x, p.y, p.h)
	}
	// No room above: goes below.
	p.Draw(sim, 5, 1, 2)
	if p.y != 3 {
		t.Errorf("below: y=%d", p.y)
	}
	// Anchor near the right edge is pulled back inside.
	p.Draw(sim, 79, 10, 11)
	if p.x+p.w > 80 {
		t.Errorf("overflows: x=%d w=%d", p.x, p.w)
	}
}

func TestReactPickerKeysSwitchToSearch(t *testing.T) {
	p := &ReactPicker{}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if p.sel != 1 {
		t.Fatalf("→ should move to slot 2, got %d", p.sel)
	}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone))
	if !p.searching || p.query != "f" || len(p.results) == 0 {
		t.Fatalf("a letter should start a search: %+v", p)
	}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone))
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone))
	if p.searching {
		t.Error("backspace on an empty query should return to the quick row")
	}
}

func TestBareReactParses(t *testing.T) {
	for _, cmd := range LocalCommands {
		if cmd.Command == CmdReact {
			parsed, err := cmd.ParseInput(cmdspec.FakeSender, cmdSigils, "/react")
			if err != nil || parsed == nil {
				t.Fatalf("bare /react: %v", err)
			}
			return
		}
	}
}
