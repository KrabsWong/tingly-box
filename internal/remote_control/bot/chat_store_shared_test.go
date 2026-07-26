package bot

import (
	"path/filepath"
	"testing"
)

// TestConcurrentStoresClobber characterizes the hazard the shared-store fix
// exists to avoid. It is not testing desired behavior — it pins WHY a chat
// store must never be opened twice over one file, so that anyone tempted to
// reintroduce a per-bot store sees the cost spelled out.
//
// The JSON store loads the file once at open and thereafter rewrites it whole
// from its own in-memory map. Two stores over one path therefore hold
// divergent snapshots, and the second one to write silently erases the first
// one's rows.
func TestConcurrentStoresClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")

	// Two stores opened over the same file, as two bots used to do.
	botA, err := NewChatStoreJSON(path)
	if err != nil {
		t.Fatalf("open store A: %v", err)
	}
	botB, err := NewChatStoreJSON(path)
	if err != nil {
		t.Fatalf("open store B: %v", err)
	}

	if err := botA.BindProject("chat-a", "telegram", "/proj/a", "owner-a"); err != nil {
		t.Fatalf("bind on A: %v", err)
	}
	// B never saw A's write, so its own write restores B's startup snapshot.
	if err := botB.BindProject("chat-b", "feishu", "/proj/b", "owner-b"); err != nil {
		t.Fatalf("bind on B: %v", err)
	}

	fresh, err := NewChatStoreJSON(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	chat, err := fresh.GetChat("chat-a")
	if err != nil {
		t.Fatalf("get chat-a: %v", err)
	}
	if chat != nil {
		t.Fatal("expected the documented clobber; two stores over one file " +
			"no longer lose writes, so Manager may not need to share one — " +
			"re-read .design/remote-storage.md P0-1 before changing this")
	}
}

// TestManagerChatStoreIsShared pins the P0-1 fix: every caller — and so every
// bot the manager runs — gets the same store instance, not a fresh one per
// call. Before it, runBotWithSettings opened a store per bot over a single
// shared path and concurrent bots erased each other's chats.
func TestManagerChatStoreIsShared(t *testing.T) {
	m := NewManager(nil)
	m.SetDataPath(filepath.Join(t.TempDir(), "chats.json"))

	first, err := m.ChatStore()
	if err != nil {
		t.Fatalf("first ChatStore: %v", err)
	}
	second, err := m.ChatStore()
	if err != nil {
		t.Fatalf("second ChatStore: %v", err)
	}
	if first != second {
		t.Fatal("ChatStore returned distinct instances; bots would clobber each other")
	}

	// A write through one handle is visible through the other, because they
	// are the same store.
	if err := first.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	path, ok, err := second.GetProjectPath("chat-1")
	if err != nil {
		t.Fatalf("get project path: %v", err)
	}
	if !ok || path != "/proj" {
		t.Errorf("GetProjectPath = (%q, %v), want (%q, true)", path, ok, "/proj")
	}

	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestManagerChatStoreRequiresDataPath keeps the "not configured" error path
// intact — Start relies on it to fail loudly rather than run a bot with no
// persistence.
func TestManagerChatStoreRequiresDataPath(t *testing.T) {
	m := NewManager(nil)
	if _, err := m.ChatStore(); err == nil {
		t.Fatal("expected an error when no data path is configured")
	}
	// Close on a manager that never opened a store is a no-op, not a panic.
	if err := m.Close(); err != nil {
		t.Errorf("Close on unopened store: %v", err)
	}
}
