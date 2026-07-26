package smart_guide

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// SessionStore persists Smart Guide conversation history as native Anthropic
// message params, one JSON file per chat. We are anthropic-first, so there is
// no neutral message type: the stored shape is exactly what the model API
// consumes, which round-trips losslessly through encoding/json.
type SessionStore struct {
	dir string
	mu  sync.Mutex
}

// NewSessionStore creates a session store rooted at dataDir. A blank dataDir
// disables persistence (returns nil, nil), mirroring the previous behavior.
func NewSessionStore(dataDir string) (*SessionStore, error) {
	if dataDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store (anthropic-native)")
	return &SessionStore{dir: dataDir}, nil
}

// safeChatID reports whether a chat ID can be used verbatim as a filename:
// ASCII letters, digits, underscore and hyphen only, and non-empty.
//
// This deliberately excludes '.', so "." and ".." can never survive.
func safeChatID(chatID string) bool {
	if chatID == "" {
		return false
	}
	for _, r := range chatID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// fileKey maps a platform chat ID to a filesystem-safe basename.
//
// Chat IDs come straight from the IM platform and are not filename-safe:
// Feishu chat IDs carry punctuation, WhatsApp JIDs contain '@' and '/', and a
// hostile ID could be "../../something". Joining one into a path unchecked is
// a directory traversal on both read and write.
//
// IDs that are already safe pass through verbatim so existing files (Telegram
// and Discord IDs are numeric) keep resolving with no migration. Anything else
// is replaced by a SHA-256 digest of the ID, which is stable, collision-free
// in practice, and cannot escape the directory.
func fileKey(chatID string) string {
	if safeChatID(chatID) {
		return chatID
	}
	sum := sha256.Sum256([]byte(chatID))
	return "h-" + hex.EncodeToString(sum[:])
}

// path returns the on-disk file for a chat's history.
func (s *SessionStore) path(chatID string) string {
	return filepath.Join(s.dir, fileKey(chatID)+"-smartguide.json")
}

// Load returns the stored history for a chat, or an empty slice if none exists.
// A corrupt or unreadable file is treated as empty (logged, not fatal) so a
// single bad session never blocks the user.
func (s *SessionStore) Load(chatID string) ([]anthropic.BetaMessageParam, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path(chatID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		logrus.WithError(err).WithField("chatID", chatID).Debug("SmartGuide session read failed, treating as empty")
		return nil, nil
	}

	var msgs []anthropic.BetaMessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Warn("SmartGuide session deserialize failed, treating as empty")
		return nil, nil
	}
	return msgs, nil
}

// Save overwrites the stored history for a chat.
func (s *SessionStore) Save(chatID string, messages []anthropic.BetaMessageParam) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	// 0600: this file holds the user's full conversation with the model, and
	// the rest of the remote state files are already owner-only.
	p := s.path(chatID)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return err
	}
	// WriteFile only applies the mode when creating, so tighten files that
	// were written as 0644 by earlier versions.
	if err := os.Chmod(p, 0o600); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Debug("Failed to tighten SmartGuide session file mode")
	}
	logrus.WithFields(logrus.Fields{"chatID": chatID, "msgCount": len(messages)}).Debug("Saved SmartGuide session")
	return nil
}

// Delete removes a chat's stored history.
func (s *SessionStore) Delete(chatID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path(chatID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
