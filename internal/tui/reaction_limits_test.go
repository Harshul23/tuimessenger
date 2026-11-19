package tui

import (
	"encoding/json"
	"slices"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

// telegramReactions is the start of what Telegram sends for an account without Premium, as the bridge stores it
// (some keys carry a variation selector: "👍️").
var telegramReactions = []string{"❤️", "👍️", "👎️", "🔥", "🥰", "👏", "😁", "🤔", "🤯", "😱", "😢", "🎉", "🙏", "🤣"}

func featuresJSON(t *testing.T, f event.RoomFeatures) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseReactionLimits(t *testing.T) {
	if parseReactionLimits(nil) != nil || parseReactionLimits(featuresJSON(t, event.RoomFeatures{MaxTextLength: 10})) != nil {
		t.Error("no allowed_reactions (WhatsApp, Meta, Slack) means any emoji")
	}
	if parseReactionLimits(featuresJSON(t, event.RoomFeatures{AllowedReactions: telegramReactions, CustomEmojiReactions: true})) != nil {
		t.Error("with custom emoji reactions (Telegram Premium) the bridge can send any emoji")
	}
	off := parseReactionLimits(featuresJSON(t, event.RoomFeatures{Reaction: event.CapLevelRejected}))
	if !off.none() || off.allows("👍") {
		t.Error("reactions rejected: nothing is allowed")
	}

	l := parseReactionLimits(featuresJSON(t, event.RoomFeatures{AllowedReactions: telegramReactions}))
	if l == nil || l.none() {
		t.Fatal("Telegram's list should limit reactions")
	}
	for key, want := range map[string]bool{"👍": true, "👍️": true, "❤": true, "🤣": true, "😂": false, "😮": false} {
		if got := l.allows(key); got != want {
			t.Errorf("allows(%q) = %v, want %v (variation selectors don't matter)", key, got, want)
		}
	}
	if got := l.filter([]string{"😂", "🔥", "😮", "👍"}); !slices.Equal(got, []string{"🔥", "👍"}) {
		t.Errorf("filter = %v", got)
	}
}

// Each 1–6 key keeps its meaning: 😂 becomes 🤣 and 😮 becomes 😱 where they aren't allowed.
func TestQuickReactionsKeepTheirPlace(t *testing.T) {
	l := parseReactionLimits(featuresJSON(t, event.RoomFeatures{AllowedReactions: telegramReactions}))
	if got, want := l.quick(quickReactions), []string{"👍", "❤️", "🤣", "😱", "😢", "🙏"}; !slices.Equal(got, want) {
		t.Errorf("Telegram quick row = %v, want %v", got, want)
	}
	var none *reactionLimits
	if got := none.quick(quickReactions); !slices.Equal(got, quickReactions) {
		t.Errorf("no limits: %v", got)
	}
	// Neither 😂 nor its stand-ins: the next allowed emoji not in the row yet.
	few := parseReactionLimits(featuresJSON(t, event.RoomFeatures{AllowedReactions: []string{"👍", "🔥", "🎉"}}))
	if got, want := few.quick(quickReactions), []string{"👍", "🔥", "🎉"}; !slices.Equal(got, want) {
		t.Errorf("a group allowing three = %v, want %v", got, want)
	}
}

func telegramRoom(t *testing.T, allowed []string) (*RoomView, *store.RoomStore) {
	t.Helper()
	_, room := bubbleView(t)
	key := ""
	room.ApplyState(&database.Event{RowID: 100, ID: "$features", RoomID: room.ID, Sender: "@telegrambot:beeper.local",
		Type: event.StateBeeperRoomFeatures.Type, StateKey: &key, Content: featuresJSON(t, event.RoomFeatures{AllowedReactions: allowed})})
	return &RoomView{Room: room}, room
}

// Whatever the path (/react 😂, a key, the picker), a reaction the network rejects isn't sent; a toast says why.
func TestRefusedReactionNotSent(t *testing.T) {
	oldToasts := toasts
	t.Cleanup(func() { toasts = oldToasts })
	toasts = &toaster{}

	rv, room := telegramRoom(t, telegramReactions)
	reactedMsg(room, nil)
	if why := rv.reactionRefused("😂"); why == "" {
		t.Fatal("😂 isn't a Telegram reaction")
	}
	if rv.reactionRefused("🤣") != "" || rv.reactionRefused("👍") != "" {
		t.Error("🤣 and 👍 are")
	}
	rv.ToggleReaction("$msg", "😂") // returns before sending: there's no backend here to send to
	toasts.mu.Lock()
	shown := toasts.current
	toasts.mu.Unlock()
	if shown == nil || shown.kind != toastWarning || shown.text != rv.reactionRefused("😂") {
		t.Errorf("want a warning toast saying why, got %+v", shown)
	}
	if got := rv.quickReactions(); got[2] != "🤣" {
		t.Errorf("key 3 in this chat = %q, want 🤣", got[2])
	}
}

// The picker in a limited chat: the quick row, the grid's "allowed" tab and search only offer what the chat takes.
func TestReactPickerShowsOnlyAllowed(t *testing.T) {
	rv, _ := telegramRoom(t, telegramReactions)
	rv.OpenReactPicker(id.EventID("$msg"))
	p := rv.picker
	if p == nil {
		t.Fatal("picker didn't open")
	}
	if !slices.Equal(p.quick, []string{"👍", "❤️", "🤣", "😱", "😢", "🙏"}) {
		t.Errorf("quick row %v", p.quick)
	}
	if icons := p.tabIcons(); len(icons) != 2 {
		t.Errorf("tabs %v, want recent + allowed", icons)
	}
	p.cat = 1
	if items := p.gridItems(); len(items) != len(telegramReactions) {
		t.Errorf("allowed tab has %d emoji, want %d", len(items), len(telegramReactions))
	}
	p.setQuery("joy")
	for _, m := range p.results {
		if !p.limits.allows(m.Emoji) {
			t.Errorf("search offered %s", m.Emoji)
		}
	}

	// Ctrl+e (into the composer) isn't a reaction: any emoji.
	rv.OpenEmojiPicker()
	if e := rv.picker; e.reactLimits() != nil || len(e.tabIcons()) < 3 {
		t.Error("the composer's emoji picker shouldn't be limited")
	}

	// Reactions turned off: the picker doesn't open.
	off, _ := telegramRoom(t, nil)
	key := ""
	off.Room.ApplyState(&database.Event{RowID: 101, ID: "$features2", RoomID: off.Room.ID, Type: event.StateBeeperRoomFeatures.Type,
		StateKey: &key, Content: featuresJSON(t, event.RoomFeatures{Reaction: event.CapLevelRejected})})
	off.OpenReactPicker("$msg")
	if off.picker != nil {
		t.Error("no picker when the chat takes no reactions")
	}
}
