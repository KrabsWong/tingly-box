package smart_guide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestFileKeyContainsTraversal pins the P0-3 fix. Chat IDs arrive straight
// from the IM platform: Feishu IDs carry punctuation, WhatsApp JIDs contain
// '@' and '/', and a hostile ID can be "../../etc/x". Joining one into a path
// unchecked escapes the store directory on both read and write.
func TestFileKeyContainsTraversal(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	hostile := []string{
		"../../../etc/passwd",
		"..",
		".",
		"a/b",
		"foo@s.whatsapp.net",
		"oc_a1b2/../..",
		"",
	}

	for _, chatID := range hostile {
		got := store.path(chatID)
		// filepath.Dir of the resolved path must still be the store dir.
		if d := filepath.Dir(filepath.Clean(got)); d != filepath.Clean(dir) {
			t.Errorf("path(%q) = %q, escapes %q", chatID, got, dir)
		}
		if strings.ContainsAny(filepath.Base(got), `/\`) {
			t.Errorf("path(%q) basename %q still contains a separator", chatID, filepath.Base(got))
		}
	}
}

// Note: the id-to-filename mapping itself is fs.SafeFileKey, tested in
// pkg/fs. What matters here is that this store routes through it.

// TestSaveLoadRoundTripsUnsafeID checks the store still works end-to-end for
// an ID that has to be hashed, and that the file lands owner-only.
func TestSaveLoadRoundTripsUnsafeID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	const chatID = "foo@s.whatsapp.net"
	want := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello")),
	}
	if err := store.Save(chatID, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load(chatID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d messages, want %d", len(got), len(want))
	}

	info, err := os.Stat(store.path(chatID))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %o, want 600 — history holds the user's conversation", mode)
	}

	// The written file must be inside the store dir, nowhere else.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("store dir holds %d entries, want exactly 1", len(entries))
	}
}

// TestSaveTightensLegacyFileMode covers whole-file histories written as 0644 by
// earlier versions. That file survives the move to append-only logs as a
// backup, so it still holds the full conversation and must not stay
// world-readable once we touch the chat again.
func TestSaveTightensLegacyFileMode(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	const chatID = "123456"
	if err := os.WriteFile(store.legacyPath(chatID), []byte("[]"), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	if err := store.Save(chatID, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(store.legacyPath(chatID))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("legacy file mode = %o, want 600", mode)
	}
}

// TestNewLogIsOwnerOnly pins the mode of the logs we create ourselves.
func TestNewLogIsOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	const chatID = "mode-check"
	msgs := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
	}
	if err := store.Save(chatID, msgs); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(store.path(chatID))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("session log mode = %o, want 600", mode)
	}
}

// TestLegacyWholeFileSessionIsImported is the upgrade path: a chat whose
// history was written by the previous whole-file store must still be there
// after the move to append-only logs, and must not be imported twice.
func TestLegacyWholeFileSessionIsImported(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	const chatID = "upgrade-me"
	old := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("question from before the upgrade")),
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(store.legacyPath(chatID), data, 0o600); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	loaded, err := store.Load(chatID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d messages, want the 1 from the legacy file", len(loaded))
	}

	// A second turn appends to the log rather than re-importing the legacy file.
	next := append(loaded, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("second turn")))
	if err := store.Save(chatID, next); err != nil {
		t.Fatalf("save: %v", err)
	}
	again, err := store.Load(chatID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(again) != 2 {
		t.Errorf("loaded %d messages, want 2 — legacy import must happen once", len(again))
	}
}

// TestClearArchivesLegacyToo guards a resurrection bug: if /clear archives only
// the log, the next open finds it empty and imports the legacy file, bringing
// the cleared conversation back.
func TestClearArchivesLegacyToo(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	const chatID = "clear-me"
	old := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("should not come back")),
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(store.legacyPath(chatID), data, 0o600); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if _, err := store.Load(chatID); err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := store.Clear(chatID); err != nil {
		t.Fatalf("clear: %v", err)
	}

	after, err := store.Load(chatID)
	if err != nil {
		t.Fatalf("load after clear: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("cleared chat came back with %d messages", len(after))
	}
}
