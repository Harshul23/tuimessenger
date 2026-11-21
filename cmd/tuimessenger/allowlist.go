package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/agent"
)

const allowlistUsage = `tuimessenger allowlist: manage allowed chats for the AI agent. Output is JSON.

  tuimessenger allowlist list                       active rules and their resolved chats
  tuimessenger allowlist lookup QUERY               search client.db for chats (interactive terminal only)
  tuimessenger allowlist add QUERY                  resolve and add a chat to allowlist (interactive terminal only)
  tuimessenger allowlist remove QUERY               remove a rule or chat from allowlist (interactive terminal only)

Every command takes -data DIR (default: tuimessenger's data folder).
QUERY can be a room id (!…), a chat name ("Family"), or network:name ("whatsapp:Family").`

func runAllowlist(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(allowlistUsage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fs := flag.NewFlagSet("tuimessenger allowlist", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data", defaultDataDir(), "tuimessenger's data folder")

	pos, err := parseAnywhere(fs, args)
	if err != nil {
		return allowlistFail(fmt.Errorf("%w (see tuimessenger allowlist help)", err))
	}
	if len(pos) == 0 {
		fmt.Println(allowlistUsage)
		return 0
	}

	cmd := pos[0]
	subArgs := pos[1:]

	switch cmd {
	case "list":
		if len(subArgs) > 0 {
			return allowlistFail(errors.New("usage: tuimessenger allowlist list"))
		}
		r, err := agent.Open(ctx, *dataDir)
		if err != nil {
			return allowlistFail(err)
		}
		defer r.Close()

		matched, err := r.Chats(ctx, false, 0)
		if err != nil && !errors.Is(err, agent.ErrNoAllowedChats) {
			return allowlistFail(err)
		}
		for _, c := range matched {
			c.LastMessage = nil
		}
		return allowlistOut(map[string]any{
			"allowlist_file": r.AllowlistPath(),
			"rules":          nonNil(r.Allowlist().Rules()),
			"matched_chats":  nonNil(matched),
		})

	case "lookup":
		if !isTerminal() {
			return allowlistFail(errors.New("tuimessenger allowlist lookup must be run interactively in a terminal"))
		}
		if len(subArgs) == 0 {
			return allowlistFail(errors.New("usage: tuimessenger allowlist lookup QUERY"))
		}
		query := strings.Join(subArgs, " ")
		matches, err := agent.LookupUnfiltered(ctx, *dataDir, query)
		if err != nil {
			return allowlistFail(err)
		}
		type lookupEntry struct {
			ID      id.RoomID `json:"id"`
			Name    string    `json:"name"`
			Network string    `json:"network,omitempty"`
			Direct  bool      `json:"direct"`
		}
		entries := make([]lookupEntry, 0, len(matches))
		for _, m := range matches {
			entries = append(entries, lookupEntry{
				ID:      m.ID,
				Name:    m.Name,
				Network: m.Network,
				Direct:  m.Direct,
			})
		}
		return allowlistOut(map[string]any{
			"matches": entries,
		})

	case "add":
		if !isTerminal() {
			return allowlistFail(errors.New("tuimessenger allowlist add must be run interactively in a terminal"))
		}
		if len(subArgs) == 0 {
			return allowlistFail(errors.New("usage: tuimessenger allowlist add QUERY"))
		}
		query := strings.Join(subArgs, " ")
		chat, err := agent.AddAllowlistRoom(ctx, *dataDir, query)
		if err != nil {
			return allowlistFail(err)
		}
		return allowlistOut(map[string]any{
			"added": chat.ID,
			"chat": map[string]any{
				"id":      chat.ID,
				"name":    chat.Name,
				"network": chat.Network,
				"direct":  chat.Direct,
			},
			"allowlist_file": agent.AllowlistPath(*dataDir),
		})

	case "remove":
		if !isTerminal() {
			return allowlistFail(errors.New("tuimessenger allowlist remove must be run interactively in a terminal"))
		}
		if len(subArgs) == 0 {
			return allowlistFail(errors.New("usage: tuimessenger allowlist remove QUERY"))
		}
		query := strings.Join(subArgs, " ")
		removed, err := agent.RemoveAllowlistEntry(ctx, *dataDir, query)
		if err != nil {
			return allowlistFail(err)
		}
		return allowlistOut(map[string]any{
			"removed":        removed,
			"allowlist_file": agent.AllowlistPath(*dataDir),
		})

	default:
		return allowlistFail(fmt.Errorf("unknown allowlist command %q (see tuimessenger allowlist help)", cmd))
	}
}

func allowlistOut(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return 0
}

func allowlistFail(err error) int {
	out := map[string]any{"error": err.Error()}
	var amb *agent.AmbiguousError
	if errors.As(err, &amb) {
		for _, c := range amb.Matches {
			c.LastMessage = nil
		}
		out["matches"] = amb.Matches
	}
	allowlistOut(out)
	return 1
}
