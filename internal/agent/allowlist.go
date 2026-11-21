package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"maunium.net/go/mautrix/id"
)

// DefaultAllowedRooms is hardcoded as the compiled-in default allowlist.
var DefaultAllowedRooms = []string{"!AExLygeLmZHxXo8r5UUUWaxK:tuimessenger.localhost"}

const (
	// AllowlistFileName is stored in dataDir and holds one allowed chat per line.
	AllowlistFileName = "agent-allowlist"
)

var (
	// ErrNoAllowedChats is returned when no allowlist rules are configured (fail-closed).
	ErrNoAllowedChats = errors.New("no allowed chats configured")
	// ErrChatNotAllowed is returned when a room is outside the allowlist.
	ErrChatNotAllowed = errors.New("chat is not in the agent allowlist")
)

// AllowlistPath returns the path to the allowlist file inside dataDir.
func AllowlistPath(dataDir string) string {
	return filepath.Join(dataDir, AllowlistFileName)
}

type allowRule struct {
	raw      string
	wildcard bool
	roomID   id.RoomID
	network  string
	name     string
}

// Allowlist restricts which chats the agent can see, search, or message.
type Allowlist struct {
	rules []allowRule
	path  string
}

var knownNetworks = map[string]bool{
	"whatsapp": true, "telegram": true, "slack": true, "instagram": true,
	"signal": true, "discord": true, "messenger": true, "imessage": true,
	"googlechat": true, "linkedin": true, "twitter": true, "irc": true,
	"matrix": true, "dummy": true,
}

func parseAllowRule(raw string) (allowRule, bool) {
	entry := strings.TrimSpace(raw)
	if entry == "" || strings.HasPrefix(entry, "#") || strings.HasPrefix(entry, "//") {
		return allowRule{}, false
	}
	if entry == "*" {
		return allowRule{raw: entry, wildcard: true}, true
	}
	if strings.HasPrefix(entry, "!") {
		return allowRule{raw: entry, roomID: id.RoomID(entry)}, true
	}
	if before, after, ok := strings.Cut(entry, ":"); ok {
		net := strings.ToLower(strings.TrimSpace(before))
		name := strings.TrimSpace(after)
		if knownNetworks[net] && name != "" {
			return allowRule{raw: entry, network: net, name: name}, true
		}
	}
	return allowRule{raw: entry, name: entry}, true
}

func parseAllowEntries(text string) []allowRule {
	var rules []allowRule
	for _, line := range strings.Split(text, "\n") {
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = line[:idx]
		}
		for _, part := range strings.Split(line, ",") {
			if r, ok := parseAllowRule(part); ok {
				rules = append(rules, r)
			}
		}
	}
	return rules
}

func loadAllowlist(dataDir string) (*Allowlist, error) {
	path := AllowlistPath(dataDir)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Allowlist{
			rules: parseAllowEntries(strings.Join(DefaultAllowedRooms, "\n")),
			path:  path,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading allowlist %s: %w", path, err)
	}
	return &Allowlist{rules: parseAllowEntries(string(raw)), path: path}, nil
}

// IsEmpty reports whether zero allowlist rules are configured.
func (a *Allowlist) IsEmpty() bool {
	return a == nil || len(a.rules) == 0
}

// AllowsAll reports whether wildcard "*" is enabled.
func (a *Allowlist) AllowsAll() bool {
	if a == nil {
		return false
	}
	return slices.ContainsFunc(a.rules, func(r allowRule) bool { return r.wildcard })
}

// Rules returns the active rule strings.
func (a *Allowlist) Rules() []string {
	if a == nil {
		return nil
	}
	out := make([]string, len(a.rules))
	for i, r := range a.rules {
		out[i] = r.raw
	}
	return out
}

func (r allowRule) matchChat(c *Chat) bool {
	if c == nil {
		return false
	}
	if r.wildcard {
		return true
	}
	if r.roomID != "" && c.ID == r.roomID {
		return true
	}
	if r.network != "" && !strings.EqualFold(c.Network, r.network) {
		return false
	}
	if r.name != "" && (strings.EqualFold(c.Name, r.name) || (c.Alias != "" && strings.EqualFold(c.Alias, r.name))) {
		return true
	}
	return false
}

// MatchChat reports whether chat matches at least one allowlist rule.
func (a *Allowlist) MatchChat(c *Chat) bool {
	if a.IsEmpty() || c == nil {
		return false
	}
	for _, r := range a.rules {
		if r.matchChat(c) {
			return true
		}
	}
	return false
}

// LookupUnfiltered opens client.db read-only without allowlist filtering and returns chats matching query
// by !roomid, network:name, alias, or case-insensitive substring so the admin can discover room IDs without
// temporarily disabling the filter.
func LookupUnfiltered(ctx context.Context, dataDir, query string) ([]*Chat, error) {
	allowAll := &Allowlist{rules: []allowRule{{raw: "*", wildcard: true}}}
	r, err := openWithAllowlist(ctx, dataDir, allowAll)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	allChats, err := r.Chats(ctx, false, 0)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || query == "*" {
		for _, c := range allChats {
			c.LastMessage = nil
		}
		return allChats, nil
	}

	if strings.HasPrefix(query, "!") {
		for _, c := range allChats {
			if string(c.ID) == query {
				c.LastMessage = nil
				return []*Chat{c}, nil
			}
		}
		var matches []*Chat
		qLower := strings.ToLower(query)
		for _, c := range allChats {
			if strings.Contains(strings.ToLower(string(c.ID)), qLower) {
				c.LastMessage = nil
				matches = append(matches, c)
			}
		}
		return matches, nil
	}

	network, name := "", query
	if before, after, ok := strings.Cut(query, ":"); ok && before != "" && !strings.Contains(before, " ") {
		network, name = strings.ToLower(strings.TrimSpace(before)), strings.TrimSpace(after)
	}
	nameLower := strings.ToLower(name)

	var matches []*Chat
	for _, c := range allChats {
		if network != "" && !strings.EqualFold(c.Network, network) {
			continue
		}
		chatNameLower := strings.ToLower(c.Name)
		chatAliasLower := strings.ToLower(c.Alias)
		if nameLower == "" ||
			chatNameLower == nameLower ||
			(chatAliasLower != "" && chatAliasLower == nameLower) ||
			strings.Contains(chatNameLower, nameLower) ||
			(chatAliasLower != "" && strings.Contains(chatAliasLower, nameLower)) {
			c.LastMessage = nil
			matches = append(matches, c)
		}
	}
	return matches, nil
}

// AddAllowlistRoom resolves query via LookupUnfiltered (failing with an AmbiguousError if >1 chat matches,
// or not-found error if 0 match), materializes <dataDir>/agent-allowlist seeded with DefaultAllowedRooms
// if the file didn't exist yet, appends the resolved immutable !roomid (deduplicated) with 0600 permissions,
// and returns the matched *Chat.
func AddAllowlistRoom(ctx context.Context, dataDir, query string) (*Chat, error) {
	matches, err := LookupUnfiltered(ctx, dataDir, query)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no chat matches %q", query)
	}
	if len(matches) > 1 {
		return nil, &AmbiguousError{Query: query, Matches: matches}
	}
	chat := matches[0]

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := AllowlistPath(dataDir)

	var rules []string
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		rules = append(rules, DefaultAllowedRooms...)
	} else if err != nil {
		return nil, err
	} else {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				rules = append(rules, line)
			}
		}
	}

	targetID := string(chat.ID)
	alreadyPresent := false
	for _, r := range rules {
		if r == targetID {
			alreadyPresent = true
			break
		}
		if rule, ok := parseAllowRule(r); ok && string(rule.roomID) == targetID {
			alreadyPresent = true
			break
		}
	}
	if !alreadyPresent {
		rules = append(rules, targetID)
	}

	content := strings.Join(rules, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return nil, err
	}
	return chat, nil
}

// RemoveAllowlistEntry materializes <dataDir>/agent-allowlist seeded with DefaultAllowedRooms if the file didn't exist yet,
// removes any rule matching query directly (by raw rule / !roomid) or matching a chat resolved via LookupUnfiltered,
// rewrites the file (0600, even if 0 rules remain), and returns the removed rule(s).
func RemoveAllowlistEntry(ctx context.Context, dataDir, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty query")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := AllowlistPath(dataDir)

	var rules []string
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		rules = append(rules, DefaultAllowedRooms...)
	} else if err != nil {
		return nil, err
	} else {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				rules = append(rules, line)
			}
		}
	}

	var resolvedChats []*Chat
	if chats, err := LookupUnfiltered(ctx, dataDir, query); err == nil {
		resolvedChats = chats
	}

	matchesQuery := func(line string) bool {
		if strings.EqualFold(line, query) {
			return true
		}
		rule, ok := parseAllowRule(line)
		if ok {
			if strings.EqualFold(rule.raw, query) {
				return true
			}
			if rule.roomID != "" && string(rule.roomID) == query {
				return true
			}
			if rule.name != "" {
				if strings.EqualFold(rule.name, query) {
					return true
				}
				if rule.network != "" && strings.EqualFold(rule.network+":"+rule.name, query) {
					return true
				}
			}
		}

		for _, c := range resolvedChats {
			if string(c.ID) == line {
				return true
			}
			if ok && !rule.wildcard {
				if rule.roomID != "" && rule.roomID == c.ID {
					return true
				}
				if rule.matchChat(c) {
					return true
				}
			}
		}
		return false
	}

	var kept []string
	var removed []string
	for _, r := range rules {
		if matchesQuery(r) {
			removed = append(removed, r)
		} else {
			kept = append(kept, r)
		}
	}

	if len(removed) == 0 {
		return nil, fmt.Errorf("no allowlist entry matches %q", query)
	}

	var content []byte
	if len(kept) > 0 {
		content = []byte(strings.Join(kept, "\n") + "\n")
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return nil, err
	}
	return removed, nil
}
