package fs

import (
	"crypto/sha256"
	"encoding/hex"
)

// SafeFileKey maps an externally-supplied identifier to a basename that is
// safe to join into a directory path.
//
// The identifiers this exists for do not come from us: IM platforms mint chat
// ids containing '@' or '/' (WhatsApp JIDs, Feishu ids), and a session id can
// be whatever a user typed after /resume. Joining one into a path unchecked is
// a directory traversal on both read and write — "../../etc/passwd" resolves
// outside the store.
//
// Identifiers that are already safe (ASCII letters, digits, '_' and '-') pass
// through verbatim, so numeric chat ids and UUID session ids stay readable on
// disk and existing files keep resolving with no migration. Anything else is
// replaced by a SHA-256 digest: stable, collision-free in practice, and unable
// to escape the directory.
//
// Note '.' is deliberately unsafe, so "." and ".." can never survive.
func SafeFileKey(id string) string {
	if isSafeFileKey(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "h-" + hex.EncodeToString(sum[:])
}

// isSafeFileKey reports whether id can be used verbatim as a filename.
func isSafeFileKey(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
