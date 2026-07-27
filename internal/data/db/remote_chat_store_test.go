package db

import (
	"fmt"
	"testing"
)

func newChatStore(t *testing.T) *RemoteChatStore {
	t.Helper()
	return openChatStoreAt(t, t.TempDir())
}

// openChatStoreAt opens the shared database in dir and returns its chat store.
// Two calls with the same dir give two independent connections to one database,
// which is what the concurrency test needs.
func openChatStoreAt(t *testing.T, dir string) *RemoteChatStore {
	t.Helper()
	sm, err := NewStoreManager(dir)
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm.RemoteChats()
}

// TestConcurrentStoresDoNotClobber is the point of the whole migration. Two
// stores over the same database — a CLI command and a running server, say —
// must both see each other's writes. The JSON files this replaces failed
// exactly here: each holder rewrote the file whole from a snapshot taken at
// open, so the second writer erased the first one's chats.
func TestConcurrentStoresDoNotClobber(t *testing.T) {
	dir := t.TempDir()

	first := openChatStoreAt(t, dir)
	second := openChatStoreAt(t, dir)

	if err := first.BindProject("chat-a", "telegram", "/proj/a", "owner-a"); err != nil {
		t.Fatalf("bind on first: %v", err)
	}
	if err := second.BindProject("chat-b", "feishu", "/proj/b", "owner-b"); err != nil {
		t.Fatalf("bind on second: %v", err)
	}

	// Both writes survive, seen from either connection.
	for _, probe := range []struct {
		store  *RemoteChatStore
		chatID string
		want   string
	}{
		{first, "chat-a", "/proj/a"},
		{first, "chat-b", "/proj/b"},
		{second, "chat-a", "/proj/a"},
		{second, "chat-b", "/proj/b"},
	} {
		got, ok, err := probe.store.GetProjectPath(probe.chatID)
		if err != nil {
			t.Fatalf("get %s: %v", probe.chatID, err)
		}
		if !ok || got != probe.want {
			t.Errorf("GetProjectPath(%s) = (%q, %v), want (%q, true)",
				probe.chatID, got, ok, probe.want)
		}
	}
}

// TestClearingFlagsPersists guards the GORM trap this store had to be written
// around: struct-based Updates silently skips zero values, so turning a flag
// back off would have looked like it worked and changed nothing.
func TestClearingFlagsPersists(t *testing.T) {
	store := newChatStore(t)

	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}
	if !store.IsWhitelisted("chat-1") {
		t.Fatal("chat should be whitelisted")
	}
	if err := store.RemoveFromWhitelist("chat-1"); err != nil {
		t.Fatalf("unwhitelist: %v", err)
	}
	if store.IsWhitelisted("chat-1") {
		t.Error("whitelist flag was not cleared")
	}

	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender-1"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if !store.IsChatPaired("chat-1", "bot-uuid") {
		t.Fatal("chat should be paired")
	}
	if err := store.ClearPaired("chat-1"); err != nil {
		t.Fatalf("clear pairing: %v", err)
	}
	if store.IsChatPaired("chat-1", "bot-uuid") {
		t.Error("pairing was not cleared")
	}
	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat.PairedBotUUID != "" || chat.PairedSenderID != "" {
		t.Errorf("pairing identifiers survived the clear: %+v", chat)
	}
}

// TestClearPairedPreservesOtherState checks unpairing is scoped: the chat's
// project binding and whitelist status must survive it.
func TestClearPairedPreservesOtherState(t *testing.T) {
	store := newChatStore(t)

	if err := store.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}
	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := store.ClearPaired("chat-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat.ProjectPath != "/proj" {
		t.Errorf("ProjectPath = %q, want /proj", chat.ProjectPath)
	}
	if !chat.IsWhitelisted {
		t.Error("whitelist was cleared by unpairing")
	}
}

// TestUpdateChatMissingIsNoop keeps the contract the JSON store had: updating
// a chat that doesn't exist is silently ignored, not an error.
func TestUpdateChatMissingIsNoop(t *testing.T) {
	store := newChatStore(t)

	called := false
	if err := store.UpdateChat("nope", func(c *Chat) { called = true }); err != nil {
		t.Fatalf("update missing: %v", err)
	}
	if called {
		t.Error("update function ran for a chat that does not exist")
	}
}

// TestSetCurrentAgentCreatesChat covers the auto-create: handoff state must
// persist on chats that were never bound or paired first.
func TestSetCurrentAgentCreatesChat(t *testing.T) {
	store := newChatStore(t)

	if err := store.SetCurrentAgent("fresh-chat", "telegram", "claude"); err != nil {
		t.Fatalf("set agent: %v", err)
	}
	got, err := store.GetCurrentAgent("fresh-chat")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got != "claude" {
		t.Errorf("current agent = %q, want claude", got)
	}
}

// TestGetCurrentAgentDefaults checks Smart Guide stays the entry point for an
// unknown chat.
func TestGetCurrentAgentDefaults(t *testing.T) {
	store := newChatStore(t)

	got, err := store.GetCurrentAgent("unknown")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got != DefaultChatAgent {
		t.Errorf("current agent = %q, want %q", got, DefaultChatAgent)
	}
}

// TestListChatsByOwnerFiltersUnbound checks the owner listing only surfaces
// chats that actually have a project bound.
func TestListChatsByOwnerFiltersUnbound(t *testing.T) {
	store := newChatStore(t)

	if err := store.BindProject("bound", "telegram", "/proj", "owner-1"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := store.GetOrCreateChat("unbound", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.BindProject("other-owner", "telegram", "/proj", "owner-2"); err != nil {
		t.Fatalf("bind other: %v", err)
	}

	chats, err := store.ListChatsByOwner("owner-1", "telegram")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "bound" {
		t.Errorf("chats = %+v, want just the bound chat for owner-1", chats)
	}
}

// TestUpsertRequiresChatID keeps the validation the store inherited.
func TestUpsertRequiresChatID(t *testing.T) {
	store := newChatStore(t)

	if err := store.UpsertChat(&Chat{}); err == nil {
		t.Error("expected an error upserting a chat with no ID")
	}
}

// ---------- project-history MRU ----------
//
// These moved here with PushProjectHistory itself when chats became rows.

func TestPushProjectHistory_PrependsAndDedupes(t *testing.T) {
	chat := &Chat{}
	PushProjectHistory(chat, "/a")
	PushProjectHistory(chat, "/b")
	PushProjectHistory(chat, "/c")
	PushProjectHistory(chat, "/a") // dedupe — should move to front, not duplicate
	want := []string{"/a", "/c", "/b"}
	if len(chat.ProjectHistory) != len(want) {
		t.Fatalf("history length %d, want %d (%v)", len(chat.ProjectHistory), len(want), chat.ProjectHistory)
	}
	for i, w := range want {
		if chat.ProjectHistory[i] != w {
			t.Errorf("history[%d] = %q, want %q (%v)", i, chat.ProjectHistory[i], w, chat.ProjectHistory)
		}
	}
	if chat.ProjectPath != "/a" {
		t.Errorf("ProjectPath = %q, want /a", chat.ProjectPath)
	}
}

func TestPushProjectHistory_SeedsLegacyProjectPath(t *testing.T) {
	chat := &Chat{ProjectPath: "/legacy"} // pre-existing binding from before history
	PushProjectHistory(chat, "/new")
	want := []string{"/new", "/legacy"}
	if len(chat.ProjectHistory) != 2 || chat.ProjectHistory[0] != want[0] || chat.ProjectHistory[1] != want[1] {
		t.Errorf("history = %v, want %v", chat.ProjectHistory, want)
	}
}

func TestPushProjectHistory_EmptyPathIsNoOp(t *testing.T) {
	chat := &Chat{ProjectPath: "/x", ProjectHistory: []string{"/x"}}
	PushProjectHistory(chat, "")
	if chat.ProjectPath != "/x" || len(chat.ProjectHistory) != 1 {
		t.Errorf("empty path should not mutate state: path=%q history=%v", chat.ProjectPath, chat.ProjectHistory)
	}
}

func TestPushProjectHistory_Caps(t *testing.T) {
	chat := &Chat{}
	for i := 0; i < ProjectHistoryCap+5; i++ {
		PushProjectHistory(chat, fmt.Sprintf("/p%d", i))
	}
	if len(chat.ProjectHistory) != ProjectHistoryCap {
		t.Errorf("history not capped: got %d, want %d", len(chat.ProjectHistory), ProjectHistoryCap)
	}
}
