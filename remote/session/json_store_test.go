package session

import (
	"path/filepath"
	"testing"
	"time"
)

// reopen returns a second store over the same file. Because the JSON store
// loads once at open, a fresh store sees exactly what is on disk — which is
// what makes it a durability probe.
func reopen(t *testing.T, path string) *SessionStoreJSON {
	t.Helper()
	s, err := NewSessionStoreJSON(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	return s
}

// TestSetPersistsImmediately pins the P0-2 fix: Set must reach disk on its
// own. Before it, jsonstore.Set only flipped a dirty flag, so a session's
// status transitions, response and messages lived in memory until a clean
// shutdown — a crash lost them all and left sessions stuck in "pending".
func TestSetPersistsImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	store, err := NewSessionStoreJSON(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	sess := &Session{
		ID:        "sess-1",
		ChatID:    "chat-1",
		Agent:     "claude",
		Project:   "/tmp/proj",
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Mutate the way Manager.Update does, and store it again.
	sess.Status = StatusCompleted
	sess.Response = "done"
	sess.Messages = append(sess.Messages, Message{Role: "user", Content: "hi"})
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set after update: %v", err)
	}

	// Deliberately do NOT Close: the point is that the update survives
	// without a graceful shutdown.
	got, err := reopen(t, path).Get(sess.ID)
	if err != nil {
		t.Fatalf("get from reopened store: %v", err)
	}
	if got == nil {
		t.Fatal("session did not reach disk")
	}
	if got.Status != StatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, StatusCompleted)
	}
	if got.Response != "done" {
		t.Errorf("response = %q, want %q", got.Response, "done")
	}
	if len(got.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(got.Messages))
	}
}

// TestDeletePersistsImmediately covers the same durability contract for removal.
func TestDeletePersistsImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	store, err := NewSessionStoreJSON(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := store.Set("sess-1", &Session{ID: "sess-1", Status: StatusPending}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Delete("sess-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := reopen(t, path).Get("sess-1")
	if err != nil {
		t.Fatalf("get from reopened store: %v", err)
	}
	if got != nil {
		t.Error("deleted session still on disk")
	}
}

// TestManagerUpdatePersistsWithoutStop checks the fix end-to-end through the
// Manager, which is where the old *SessionStoreJSON type assertion used to
// decide whether anything was flushed at all.
func TestManagerUpdatePersistsWithoutStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	store, err := NewSessionStoreJSON(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	mgr := NewManager(Config{Timeout: 30 * time.Minute}, store)
	defer mgr.Stop()

	sess := mgr.CreateWith("chat-1", "claude", "/tmp/proj")
	if !mgr.SetCompleted(sess.ID, "all good") {
		t.Fatal("SetCompleted returned false")
	}

	// No mgr.Stop() before reading: simulate a crash after the update.
	got, err := reopen(t, path).Get(sess.ID)
	if err != nil {
		t.Fatalf("get from reopened store: %v", err)
	}
	if got == nil {
		t.Fatal("session did not reach disk")
	}
	if got.Status != StatusCompleted || got.Response != "all good" {
		t.Errorf("persisted session = {%s, %q}, want {%s, %q}",
			got.Status, got.Response, StatusCompleted, "all good")
	}
}

// TestManagerStopIsIdempotent guards the sync.Once added to Stop. Shutdown
// paths can overlap, and closing stopCh twice panics.
func TestManagerStopIsIdempotent(t *testing.T) {
	mgr := NewManager(Config{Timeout: time.Minute}, nil)
	mgr.Stop()
	mgr.Stop() // must not panic
}
