package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/agent"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

func TestTUIAllowlistDefaultsAndFiltering(t *testing.T) {
	td := t.TempDir()
	t.Setenv("HOME", td)
	t.Setenv("XDG_CONFIG_HOME", td)
	t.Setenv("TUIMESSENGER_ALLOWED_CHATS", "*") // Must be ignored!

	// 1. Without agent-allowlist on disk: defaults to agent.DefaultAllowedRooms
	raw, active := loadTUIAllowlistRaw()
	if !active {
		t.Fatal("expected loadTUIAllowlistRaw to be active by default")
	}
	if raw != strings.Join(agent.DefaultAllowedRooms, "\n") {
		t.Fatalf("loadTUIAllowlistRaw() = %q, want default %q", raw, strings.Join(agent.DefaultAllowedRooms, "\n"))
	}

	defaultRoomID := id.RoomID(agent.DefaultAllowedRooms[0])
	rooms := []*store.RoomListEntry{
		{RoomID: defaultRoomID, Name: "VST 25-29 Official", Bridge: "whatsapp"},
		{RoomID: "!other:x", Name: "Other Chat", Bridge: "whatsapp"},
	}

	filtered := filterAllowedRoomEntries(rooms)
	if len(filtered) != 1 || filtered[0].RoomID != defaultRoomID {
		t.Fatalf("filterAllowedRoomEntries() = %+v, want only %s", filtered, defaultRoomID)
	}

	// 2. With agent-allowlist on disk
	cfgDir := filepath.Join(td, "Library", "Application Support", "tuimessenger")
	_ = os.MkdirAll(cfgDir, 0o700)
	allowPath := filepath.Join(cfgDir, "agent-allowlist")

	// Empty file: fail-closed (0 rooms)
	if err := os.WriteFile(allowPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	filteredEmpty := filterAllowedRoomEntries(rooms)
	if len(filteredEmpty) != 0 {
		t.Fatalf("filterAllowedRoomEntries() with empty file = %+v, want 0", filteredEmpty)
	}

	// Custom rule
	if err := os.WriteFile(allowPath, []byte("!other:x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	filteredCustom := filterAllowedRoomEntries(rooms)
	if len(filteredCustom) != 1 || filteredCustom[0].RoomID != "!other:x" {
		t.Fatalf("filterAllowedRoomEntries() with custom rule = %+v, want only !other:x", filteredCustom)
	}
}
