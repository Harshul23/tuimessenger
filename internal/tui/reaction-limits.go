package tui

import (
	"encoding/json"

	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/event"
)

// reactionLimits are the reactions a chat's network accepts, from the room's com.beeper.room_features: e.g.
// Telegram without Premium only takes its own reaction emoji (and a group may allow fewer). nil allows any emoji.
type reactionLimits struct {
	allowed []string        // in the network's order, as it sends them
	set     map[string]bool // the same, without variation selectors
}

// parseReactionLimits reads room features; nil when they don't limit reactions.
func parseReactionLimits(content json.RawMessage) *reactionLimits {
	var features event.RoomFeatures
	if len(content) == 0 || json.Unmarshal(content, &features) != nil || features.CustomEmojiReactions {
		return nil // custom emoji reactions (Telegram Premium): the bridge can send any emoji
	}
	if features.Reaction == event.CapLevelRejected {
		return &reactionLimits{set: map[string]bool{}} // reactions are off in this chat
	}
	if len(features.AllowedReactions) == 0 {
		return nil
	}
	limits := &reactionLimits{set: make(map[string]bool, len(features.AllowedReactions))}
	for _, key := range features.AllowedReactions {
		if bare := variationselector.Remove(key); !limits.set[bare] {
			limits.set[bare] = true
			limits.allowed = append(limits.allowed, key)
		}
	}
	return limits
}

func (l *reactionLimits) allows(key string) bool {
	return l == nil || l.set[variationselector.Remove(key)]
}

func (l *reactionLimits) none() bool {
	return l != nil && len(l.allowed) == 0
}

// filter keeps the allowed emoji of keys, in their order.
func (l *reactionLimits) filter(keys []string) []string {
	if l == nil {
		return keys
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if l.allows(key) {
			out = append(out, key)
		}
	}
	return out
}

// quickStandIns replace a quick reaction a network doesn't take with the nearest one it might (Telegram has 🤣, 😱).
var quickStandIns = map[string][]string{"😂": {"🤣", "😁"}, "😮": {"😱", "🤯"}}

// quick is the 1–6 row for this chat. Each key keeps its place: one the chat doesn't allow becomes its stand-in,
// else the next allowed emoji not in the row yet, so key 3 is still "laughing" on Telegram (🤣).
func (l *reactionLimits) quick(defaults []string) []string {
	if l == nil {
		return defaults
	}
	out := make([]string, 0, len(defaults))
	taken := func(key string) bool { return containsReaction(out, key) || containsReaction(defaults, key) }
	for _, key := range defaults {
		pick := ""
		if l.allows(key) {
			pick = key
		}
		for _, alt := range quickStandIns[key] {
			if pick == "" && l.allows(alt) && !taken(alt) {
				pick = alt
			}
		}
		for _, alt := range l.allowed {
			if pick == "" && !taken(alt) {
				pick = alt
			}
		}
		if pick != "" {
			out = append(out, pick)
		}
	}
	return out
}

func containsReaction(keys []string, key string) bool {
	for _, k := range keys {
		if sameReaction(k, key) {
			return true
		}
	}
	return false
}

// reactionLimits reads the open chat's limits now, so changes by the network (or a group's admins) apply.
func (view *RoomView) reactionLimits() *reactionLimits {
	if view.Room == nil {
		return nil
	}
	evt := view.Room.GetStateEvent(event.StateBeeperRoomFeatures, "")
	if evt == nil {
		return nil
	}
	return parseReactionLimits(evt.Content)
}

// quickReactions are the 1–6 reactions in this chat.
func (view *RoomView) quickReactions() []string {
	return view.reactionLimits().quick(quickReactions)
}

// reactionRefused says why an emoji can't be a reaction here, or "" if it can.
func (view *RoomView) reactionRefused(key string) string {
	limits := view.reactionLimits()
	if limits.allows(key) {
		return ""
	}
	network := "This chat"
	if view.Room != nil {
		if id := view.Room.BridgeID(); id != "" {
			network = networkName(id)
		}
	}
	if limits.none() {
		return "Reactions are turned off in this chat"
	}
	return network + " doesn't allow " + key + " as a reaction here"
}
