package tui

import (
	"maps"
	"slices"
	"sync"
	"time"

	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// sameReaction compares keys ignoring variation selectors (❤ vs ❤️).
func sameReaction(a, b string) bool {
	return variationselector.Remove(a) == variationselector.Remove(b)
}

// OwnReactions maps each of our reaction keys on eventID to its reaction events, as hicli keeps them on the message.
func (view *RoomView) OwnReactions(eventID id.EventID) map[string][]id.EventID {
	if view.Room == nil {
		return nil
	}
	evt := view.Room.GetEventByID(eventID)
	if evt == nil {
		return nil
	}
	return liveOwnReactions(evt.OwnReactions)
}

// liveOwnReactions drops keys whose list is empty: hicli's triggers leave {"👍": []} after a removal.
func liveOwnReactions(own map[string][]id.EventID) map[string][]id.EventID {
	out := make(map[string][]id.EventID, len(own))
	for key, ids := range own {
		if len(ids) > 0 {
			out[key] = ids
		}
	}
	return out
}

// reactionToggles makes a quick second press wait for the first one's result instead of racing it.
var reactionToggles sync.Mutex

// ownReactionsSettle is how long a toggle waits for its change to come back down sync.
const ownReactionsSettle = 5 * time.Second

// ToggleReaction removes our reaction if it's the same emoji, otherwise
// replaces whatever we had: one reaction per message, like the networks.
func (view *RoomView) ToggleReaction(eventID id.EventID, key string) {
	defer debug.Recover()
	// Refuse what the network would reject (e.g. 😂 on Telegram); taking a reaction back always works.
	if why := view.reactionRefused(key); why != "" && !containsReaction(slices.Collect(maps.Keys(view.OwnReactions(eventID))), key) {
		toasts.Show(toastWarning, why)
		return
	}
	reactionToggles.Lock()
	defer reactionToggles.Unlock()
	removedSame := false
	var removed []id.EventID
	for ownKey, reactionIDs := range view.OwnReactions(eventID) {
		if sameReaction(ownKey, key) {
			removedSame = true
		}
		for _, reactionID := range reactionIDs {
			if view.Redact(reactionID, "") == nil {
				removed = append(removed, reactionID)
			}
		}
	}
	var added id.EventID
	if !removedSame {
		added = view.SendReaction(eventID, key)
	}
	view.waitForOwnReactions(eventID, added, removed, ownReactionsSettle)
	view.parent.parent.Render()
}

// waitForOwnReactions blocks until the message's own reactions include added and none of removed, or timeout.
func (view *RoomView) waitForOwnReactions(eventID, added id.EventID, removed []id.EventID, timeout time.Duration) {
	if added == "" && len(removed) == 0 {
		return
	}
	changed := make(chan struct{}, 1)
	stop := view.Room.EventSubs.Listen(eventID, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer stop()
	deadline := time.After(timeout)
	for !ownReactionsSettled(view.OwnReactions(eventID), added, removed) {
		select {
		case <-changed:
		case <-deadline:
			debug.Print("Reaction change on", eventID, "didn't come back from sync in time")
			return
		}
	}
}

func ownReactionsSettled(own map[string][]id.EventID, added id.EventID, removed []id.EventID) bool {
	has := func(want id.EventID) bool {
		for _, ids := range own {
			if slices.Contains(ids, want) {
				return true
			}
		}
		return false
	}
	if added != "" && !has(added) {
		return false
	}
	for _, reactionID := range removed {
		if has(reactionID) {
			return false
		}
	}
	return true
}
