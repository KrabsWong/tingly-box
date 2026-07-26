package fs

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeFileKeyContainsTraversal is the reason this exists: the identifiers
// it maps come from IM platforms and from whatever a user types after /resume,
// so joining one into a path unchecked escapes the store directory.
func TestSafeFileKeyContainsTraversal(t *testing.T) {
	const dir = "/var/store"

	for _, id := range []string{
		"../../../etc/passwd",
		"..",
		".",
		"a/b",
		`a\b`,
		"foo@s.whatsapp.net",
		"oc_a1b2/../..",
		"",
		"用户1",
	} {
		key := SafeFileKey(id)
		if strings.ContainsAny(key, `/\`) {
			t.Errorf("SafeFileKey(%q) = %q, still contains a separator", id, key)
		}
		if key == "." || key == ".." {
			t.Errorf("SafeFileKey(%q) = %q, resolves to a directory", id, key)
		}
		joined := filepath.Join(dir, key)
		if filepath.Dir(filepath.Clean(joined)) != dir {
			t.Errorf("SafeFileKey(%q) = %q escapes %q", id, key, dir)
		}
	}
}

// TestSafeFileKeyPreservesSafeIDs keeps numeric chat ids and UUID session ids
// readable on disk, and means existing files keep resolving after upgrade.
func TestSafeFileKeyPreservesSafeIDs(t *testing.T) {
	for _, id := range []string{
		"123456789",
		"-1001234567890",
		"chat_42",
		"AbC-123",
		"550e8400-e29b-41d4-a716-446655440000",
	} {
		if got := SafeFileKey(id); got != id {
			t.Errorf("SafeFileKey(%q) = %q, want it unchanged", id, got)
		}
	}
}

// TestSafeFileKeyIsInjective guards against two identifiers sharing one file,
// which would leak one conversation into another.
func TestSafeFileKeyIsInjective(t *testing.T) {
	ids := []string{"a/b", "a.b", "a@b", "../a", "a b", "用户1", "用户2", ""}
	seen := make(map[string]string, len(ids))
	for _, id := range ids {
		key := SafeFileKey(id)
		if prev, dup := seen[key]; dup {
			t.Errorf("SafeFileKey(%q) collides with SafeFileKey(%q) = %q", id, prev, key)
		}
		seen[key] = id
	}
}

// TestSafeFileKeyIsStable: the same identifier must always map to the same
// file, or a restart would lose the history it wrote.
func TestSafeFileKeyIsStable(t *testing.T) {
	const id = "foo@s.whatsapp.net"
	if SafeFileKey(id) != SafeFileKey(id) {
		t.Error("SafeFileKey is not deterministic")
	}
}
