package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

const me = id.UserID("@me:tuimessenger.localhost")

// fixture writes a client.db like the TUI's, then opens it read-only through Open.
type fixture struct {
	t    *testing.T
	ctx  context.Context
	dir  string
	db   *database.Database
	next time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AllowlistFileName), []byte("*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tui"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := dbutil.NewFromConfig("test", dbutil.Config{PoolConfig: dbutil.PoolConfig{
		Type: "sqlite3-fk-wal", URI: "file:" + filepath.Join(dir, "tui", "client.db") + "?_txlock=immediate", MaxOpenConns: 1,
	}}, dbutil.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: context.Background(), dir: dir, db: database.New(raw), next: time.UnixMilli(1_700_000_000_000)}
	if err = f.db.Upgrade(f.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	if err = f.db.Account.Put(f.ctx, &database.Account{UserID: me, DeviceID: "TMTUI", AccessToken: "x", HomeserverURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	return f
}

// room makes a chat with members (localpart → display name) and messages from those members ("" is you).
func (f *fixture) room(roomID id.RoomID, name string, members map[string]string, msgs ...[2]string) []id.EventID {
	f.t.Helper()
	if err := f.db.Room.CreateRow(f.ctx, roomID); err != nil {
		f.t.Fatal(err)
	}
	var heroes []id.UserID
	for localpart, display := range members {
		userID := id.NewUserID(localpart, "tuimessenger.localhost")
		heroes = append(heroes, userID)
		f.state(roomID, userID, &event.MemberEventContent{Membership: event.MembershipJoin, Displayname: display})
	}
	var ids []id.EventID
	var rowIDs []database.EventRowID
	for _, m := range msgs {
		sender := me
		if m[0] != "" {
			sender = id.NewUserID(m[0], "tuimessenger.localhost")
		}
		rowID, evtID := f.insert(roomID, sender, event.EventMessage.Type, &event.MessageEventContent{MsgType: event.MsgText, Body: m[1]})
		ids, rowIDs = append(ids, evtID), append(rowIDs, rowID)
	}
	if _, err := f.db.Timeline.Append(f.ctx, roomID, rowIDs); err != nil {
		f.t.Fatal(err)
	}
	room := &database.Room{ID: roomID, Name: ptr.Ptr(name), NameQuality: database.NameQualityExplicit,
		LazyLoadSummary: &mautrix.LazyLoadSummary{Heroes: heroes, JoinedMemberCount: ptr.Ptr(len(heroes) + 2)}, SortingTimestamp: jsontime.UM(f.next),
		UnreadCounts: database.UnreadCounts{UnreadMessages: 1}}
	if len(rowIDs) > 0 {
		room.PreviewEventRowID = rowIDs[len(rowIDs)-1]
	}
	if err := f.db.Room.Update(f.ctx, room); err != nil {
		f.t.Fatal(err)
	}
	return ids
}

func (f *fixture) insert(roomID id.RoomID, sender id.UserID, evtType string, content any) (database.EventRowID, id.EventID) {
	f.t.Helper()
	raw, _ := json.Marshal(content)
	f.next = f.next.Add(time.Minute)
	evtID := id.EventID("$" + f.next.Format("150405") + string(roomID[1:4]))
	evt := &database.Event{RoomID: roomID, ID: evtID, Sender: sender, Type: evtType, Timestamp: jsontime.UM(f.next), Content: raw, Unsigned: json.RawMessage("{}")}
	var rel struct {
		RelatesTo *event.RelatesTo `json:"m.relates_to"`
	}
	_ = json.Unmarshal(raw, &rel)
	if rel.RelatesTo != nil && rel.RelatesTo.Type != "" {
		evt.RelatesTo, evt.RelationType = rel.RelatesTo.EventID, rel.RelatesTo.Type
	}
	rowID, err := f.db.Event.Insert(f.ctx, evt)
	if err != nil {
		f.t.Fatal(err)
	}
	return rowID, evtID
}

func (f *fixture) state(roomID id.RoomID, userID id.UserID, member *event.MemberEventContent) {
	f.t.Helper()
	raw, _ := json.Marshal(member)
	f.next = f.next.Add(time.Second)
	rowID, err := f.db.Event.Insert(f.ctx, &database.Event{RoomID: roomID, ID: id.EventID("$m" + userID.Localpart() + string(roomID[1:4])),
		Sender: userID, Type: event.StateMember.Type, StateKey: ptr.Ptr(userID.String()), Timestamp: jsontime.UM(f.next), Content: raw, Unsigned: json.RawMessage("{}")})
	if err != nil {
		f.t.Fatal(err)
	}
	if err = f.db.CurrentState.Set(f.ctx, roomID, event.StateMember, userID.String(), rowID, member.Membership); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) open() *Reader {
	f.t.Helper()
	r, err := Open(f.ctx, f.dir)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestChatsMessagesSearch(t *testing.T) {
	f := newFixture(t)
	maya := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"},
		[2]string{"whatsapp_15550100011", "dinner at 8?"}, [2]string{"", "sounds good"}, [2]string{"whatsapp_15550100011", "see you at the lake"})
	f.room("!mayaig:x", "maya.draws", map[string]string{"instagram_100000000031": "maya.draws"},
		[2]string{"instagram_100000000031", "new sketch"})
	f.room("!design:x", "#design", map[string]string{"slackgo_t0000000000-u0000000041": "Priya"},
		[2]string{"slackgo_t0000000000-u0000000041", "mockups are up"})
	// An edit replaces the text; the edit itself isn't a message.
	editRow, _ := f.insert("!maya:x", me, event.EventMessage.Type, map[string]any{
		"msgtype": "m.text", "body": "* sounds great", "m.new_content": map[string]any{"msgtype": "m.text", "body": "sounds great"},
		"m.relates_to": map[string]any{"rel_type": "m.replace", "event_id": maya[1]},
	})
	if _, err := f.db.Exec(f.ctx, "UPDATE event SET last_edit_rowid=$1 WHERE event_id=$2", editRow, maya[1]); err != nil {
		t.Fatal(err)
	}
	r := f.open()
	ctx := f.ctx

	chats, err := r.Chats(ctx, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 3 || chats[0].Name != "#design" || chats[0].Network != "slack" || chats[2].Network != "whatsapp" || !chats[2].Direct {
		t.Fatalf("chats = %+v", chats)
	}
	if last := chats[2].LastMessage; last == nil || last.Sender != "Maya" || last.Text != "see you at the lake" {
		t.Fatalf("last message = %+v", last)
	}

	msgs, err := r.Messages(ctx, "!maya:x", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].Text != "dinner at 8?" || msgs[1].Text != "sounds great" || !msgs[1].Edited || !msgs[1].FromMe || msgs[1].Sender != "you" {
		t.Fatalf("messages = %+v %+v", msgs[0], msgs[1])
	}
	if msgs, _ = r.Messages(ctx, "!maya:x", 1); len(msgs) != 1 || msgs[0].Text != "see you at the lake" {
		t.Fatalf("limit 1 = %+v", msgs)
	}

	found, err := r.Search(ctx, "lak", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Chat != "!maya:x" {
		t.Fatalf("search = %+v", found)
	}

	for query, want := range map[string]id.RoomID{"Maya": "!maya:x", "instagram:maya": "!mayaig:x", "design": "!design:x", "!mayaig:x": "!mayaig:x"} {
		if chat, err := r.Resolve(ctx, query); err != nil || chat.ID != want {
			t.Errorf("Resolve(%q) = %v, %v; want %s", query, chat, err, want)
		}
	}
	var amb *AmbiguousError
	if _, err = r.Resolve(ctx, "may"); !errors.As(err, &amb) || len(amb.Matches) != 2 {
		t.Errorf("Resolve(may) = %v, want two matches", err)
	}
	if _, err = r.Resolve(ctx, "nobody"); err == nil {
		t.Error("Resolve(nobody) found something")
	}
}

// A message still on its way ("not sent") isn't reported as failed; a real send error is.
func TestPendingIsNotFailed(t *testing.T) {
	f := newFixture(t)
	ids := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"},
		[2]string{"", "on its way"}, [2]string{"", "refused"})
	for evtID, sendError := range map[id.EventID]string{ids[0]: "not sent", ids[1]: "M_FORBIDDEN: no power"} {
		if _, err := f.db.Exec(f.ctx, "UPDATE event SET send_error=$1 WHERE event_id=$2", sendError, evtID); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := f.open().Messages(f.ctx, "!maya:x", 10)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages = %+v, %v", msgs, err)
	}
	if msgs[0].Failed != "" || msgs[1].Failed != "M_FORBIDDEN: no power" {
		t.Errorf("failed = %q, %q; want only the refused one", msgs[0].Failed, msgs[1].Failed)
	}
}

// Chats read in tuimessenger only (/seen off) count as read, like in the TUI; /clear'd chats are hidden.
func TestLocalSeenAndCleared(t *testing.T) {
	f := newFixture(t)
	maya := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"}, [2]string{"whatsapp_15550100011", "hi"})
	f.room("!old:x", "Old", map[string]string{"telegram_10000000021": "Old"}, [2]string{"telegram_10000000021", "bye"})
	put := func(roomID id.RoomID, evtType event.Type, content any) {
		raw, _ := json.Marshal(content)
		if _, err := f.db.AccountData.PutRoom(f.ctx, me, roomID, evtType, raw); err != nil {
			t.Fatal(err)
		}
	}
	put("!maya:x", accountDataSeen, map[string]any{"event_id": maya[0]})
	put("!old:x", accountDataCleared, map[string]any{"ts": f.next.UnixMilli()})
	r := f.open()
	all, err := r.Chats(f.ctx, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "!maya:x" || all[0].Unread != 0 {
		t.Fatalf("chats = %+v", all)
	}
	if unread, _ := r.Chats(f.ctx, true, 0); len(unread) != 0 {
		t.Fatalf("unread = %+v", unread)
	}
}

func TestOpenWithoutClient(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); !errors.Is(err, ErrNoClient) {
		t.Fatalf("err = %v", err)
	}
}

// A WhatsApp document has its name in filename and an empty body: the agent gets the name as file.
func TestFileName(t *testing.T) {
	f := newFixture(t)
	ids := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"}, [2]string{"whatsapp_15550100011", "x"})
	pdf := `{"msgtype":"m.file","body":"","filename":"AI_Engineer_Interview_Questions.pdf","url":"mxc://x/a"}`
	if _, err := f.db.Exec(f.ctx, "UPDATE event SET content=$1 WHERE event_id=$2", pdf, ids[0]); err != nil {
		t.Fatal(err)
	}
	msgs, err := f.open().Messages(f.ctx, "!maya:x", 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("messages = %+v, %v", msgs, err)
	}
	if m := msgs[0]; m.Kind != "file" || m.File != "AI_Engineer_Interview_Questions.pdf" || m.Text != "" {
		t.Errorf("got kind=%q file=%q text=%q", m.Kind, m.File, m.Text)
	}
}

func TestAllowlistDefaultsAndAdmin(t *testing.T) {
	f := newFixture(t)
	// Remove the wildcard allowlist file created by newFixture so dataDir is uninitialized
	allowPath := AllowlistPath(f.dir)
	if err := os.Remove(allowPath); err != nil {
		t.Fatal(err)
	}

	defaultID := id.RoomID(DefaultAllowedRooms[0])
	f.room(defaultID, "VST 25-29 Official", map[string]string{"whatsapp_15550100011": "Admin"},
		[2]string{"whatsapp_15550100011", "official announcement"})
	f.room("!wa_group:x", "Project Alpha", map[string]string{"whatsapp_15550100022": "Alice"},
		[2]string{"whatsapp_15550100022", "deploy is green"})
	f.room("!private_dm:x", "Secret DM", map[string]string{"whatsapp_15550100033": "Bob"},
		[2]string{"whatsapp_15550100033", "bank password is 1234"})
	f.room("!ig_group:x", "Project Alpha", map[string]string{"instagram_100000000031": "Carol"},
		[2]string{"instagram_100000000031", "wrong network same name"})

	// 1. Uninitialized dataDir: defaults strictly to DefaultAllowedRooms
	r := f.open()
	if !slices.Equal(r.Allowlist().Rules(), DefaultAllowedRooms) {
		t.Fatalf("rules = %v, want DefaultAllowedRooms %v", r.Allowlist().Rules(), DefaultAllowedRooms)
	}
	chats, err := r.Chats(f.ctx, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].ID != defaultID {
		t.Fatalf("Chats() with default allowlist = %+v, want only %s", chats, defaultID)
	}
	if _, err := r.Messages(f.ctx, "!private_dm:x", 10); !errors.Is(err, ErrChatNotAllowed) {
		t.Fatalf("Messages(private_dm) = %v, want ErrChatNotAllowed", err)
	}

	// 2. LookupUnfiltered discovers chats without allowlist restriction
	allFound, err := LookupUnfiltered(f.ctx, f.dir, "")
	if err != nil || len(allFound) != 4 {
		t.Fatalf("LookupUnfiltered(empty) = %+v, err=%v; want 4 chats", allFound, err)
	}

	lookupWa, err := LookupUnfiltered(f.ctx, f.dir, "!wa_group:x")
	if err != nil || len(lookupWa) != 1 || lookupWa[0].ID != "!wa_group:x" {
		t.Fatalf("LookupUnfiltered(!wa_group:x) = %+v, %v", lookupWa, err)
	}

	lookupSub, err := LookupUnfiltered(f.ctx, f.dir, "Secret")
	if err != nil || len(lookupSub) != 1 || lookupSub[0].ID != "!private_dm:x" {
		t.Fatalf("LookupUnfiltered(Secret) = %+v, %v", lookupSub, err)
	}

	// 3. AddAllowlistRoom:
	// 3a. Ambiguous match fails
	if _, err := AddAllowlistRoom(f.ctx, f.dir, "Project Alpha"); err == nil {
		t.Fatal("AddAllowlistRoom(Project Alpha) succeeded, want AmbiguousError")
	} else {
		var amb *AmbiguousError
		if !errors.As(err, &amb) || len(amb.Matches) != 2 {
			t.Fatalf("expected AmbiguousError with 2 matches, got: %v", err)
		}
	}

	// 3b. Not found fails
	if _, err := AddAllowlistRoom(f.ctx, f.dir, "NonExistentRoom"); err == nil {
		t.Fatal("AddAllowlistRoom(NonExistentRoom) succeeded, want error")
	}

	// 3c. Unambiguous match adds room and materializes file with 0600 permissions
	added, err := AddAllowlistRoom(f.ctx, f.dir, "whatsapp:Project Alpha")
	if err != nil || added.ID != "!wa_group:x" {
		t.Fatalf("AddAllowlistRoom(whatsapp:Project Alpha) = %v, %v; want !wa_group:x", added, err)
	}

	info, err := os.Stat(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("allowlist file mode = %o, want 0600", perm)
	}

	// Verify deduplication: adding again doesn't duplicate
	if _, err := AddAllowlistRoom(f.ctx, f.dir, "!wa_group:x"); err != nil {
		t.Fatal(err)
	}
	rulesRaw, _ := os.ReadFile(allowPath)
	lines := strings.Split(strings.TrimSpace(string(rulesRaw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in allowlist after dedup, got %d (%v)", len(lines), lines)
	}

	// Reopen and verify newly allowed room is visible
	r2 := f.open()
	chats2, err := r2.Chats(f.ctx, false, 0)
	if err != nil || len(chats2) != 2 {
		t.Fatalf("Chats after add = %+v, err=%v", chats2, err)
	}

	// 4. RemoveAllowlistEntry:
	// Remove !wa_group:x by query
	removed, err := RemoveAllowlistEntry(f.ctx, f.dir, "whatsapp:Project Alpha")
	if err != nil || len(removed) != 1 || removed[0] != "!wa_group:x" {
		t.Fatalf("RemoveAllowlistEntry = %v, %v; want [!wa_group:x]", removed, err)
	}

	// Remove default room to reach empty state
	removedDefault, err := RemoveAllowlistEntry(f.ctx, f.dir, string(defaultID))
	if err != nil || len(removedDefault) != 1 || removedDefault[0] != string(defaultID) {
		t.Fatalf("RemoveAllowlistEntry(defaultID) = %v, %v; want [%s]", removedDefault, err, defaultID)
	}

	// File still exists on disk with 0600 permissions but is empty
	infoEmpty, err := os.Stat(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := infoEmpty.Mode().Perm(); perm != 0o600 {
		t.Fatalf("empty allowlist file mode = %o, want 0600", perm)
	}
	if infoEmpty.Size() != 0 {
		t.Fatalf("empty allowlist file size = %d, want 0", infoEmpty.Size())
	}

	// 5. Fail-closed: 0 rules configured
	rEmpty := f.open()
	if _, err := rEmpty.Chats(f.ctx, false, 0); !errors.Is(err, ErrNoAllowedChats) {
		t.Fatalf("Chats on empty allowlist = %v, want ErrNoAllowedChats", err)
	}
	if err := rEmpty.CheckAllowed(f.ctx, defaultID); !errors.Is(err, ErrChatNotAllowed) && !errors.Is(err, ErrNoAllowedChats) {
		t.Fatalf("CheckAllowed on empty allowlist = %v, want ErrNoAllowedChats or ErrChatNotAllowed", err)
	}
}
