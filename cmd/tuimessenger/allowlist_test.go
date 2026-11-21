package main

import (
	"os"
	"testing"

	"github.com/whyyor/tuimessenger/internal/agent"
)

func TestAllowlistTTYGuard(t *testing.T) {
	origIsTerminal := isTerminal
	t.Cleanup(func() { isTerminal = origIsTerminal })
	isTerminal = func() bool { return false }

	dir := t.TempDir()

	for _, subcmd := range []string{"lookup", "add", "remove"} {
		code := runAllowlist([]string{"-data", dir, subcmd, "something"})
		if code != 1 {
			t.Errorf("runAllowlist(%s) when !isTerminal() = %d, want 1", subcmd, code)
		}
	}
}

func TestAllowlistCLI(t *testing.T) {
	origIsTerminal := isTerminal
	t.Cleanup(func() { isTerminal = origIsTerminal })
	isTerminal = func() bool { return true }

	dir := t.TempDir()
	allowPath := agent.AllowlistPath(dir)

	// List without client.db will fail agent.Open, returning code 1
	code := runAllowlist([]string{"-data", dir, "list"})
	if code != 1 {
		t.Errorf("runAllowlist(list) on empty dir = %d, want 1", code)
	}

	// Remove default room when file does not exist yet seeds with DefaultAllowedRooms and empties it
	code = runAllowlist([]string{"-data", dir, "remove", agent.DefaultAllowedRooms[0]})
	if code != 0 {
		t.Errorf("runAllowlist(remove) = %d, want 0", code)
	}

	// File should now exist and be 0 bytes
	info, err := os.Stat(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("file size = %d, want 0", info.Size())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 0600", perm)
	}

	// Removing again fails because it is empty
	code = runAllowlist([]string{"-data", dir, "remove", agent.DefaultAllowedRooms[0]})
	if code != 1 {
		t.Errorf("runAllowlist(remove) on empty allowlist = %d, want 1", code)
	}

	// Unknown subcommand
	code = runAllowlist([]string{"-data", dir, "unknown"})
	if code != 1 {
		t.Errorf("runAllowlist(unknown) = %d, want 1", code)
	}
}
