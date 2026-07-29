package smart_guide

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/afk/session"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

// SessionStore persists Smart Guide conversations as append-only logs, one file
// per chat. We are anthropic-first, so there is no neutral message type: the
// stored shape is exactly what the model API consumes.
//
// The store owns the chatID-to-path mapping and the archive semantics; the log
// format and its durability live in afk/session.
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
	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store (append-only log)")
	return &SessionStore{dir: dataDir}, nil
}

// path returns the on-disk log for a chat.
//
// The chat ID goes through fs.SafeFileKey because it comes straight from the IM
// platform and is not filename-safe: Feishu ids carry punctuation and WhatsApp
// JIDs contain '@' and '/'.
func (s *SessionStore) path(chatID string) string {
	return filepath.Join(s.dir, fs.SafeFileKey(chatID)+"-smartguide.jsonl")
}

// legacyPath returns the whole-file JSON location written by earlier versions.
// It is read once, on first open, so upgrading does not drop live conversations.
func (s *SessionStore) legacyPath(chatID string) string {
	return filepath.Join(s.dir, fs.SafeFileKey(chatID)+"-smartguide.json")
}

// open loads a chat's log, importing a legacy whole-file session if this is the
// first time the chat has been opened since the format changed.
func (s *SessionStore) open(chatID string) (*session.Session, error) {
	sess, err := session.Open(s.path(chatID))
	if err != nil {
		return nil, err
	}
	legacy := s.legacyPath(chatID)
	if err := sess.ImportLegacy(legacy); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).
			Warn("Failed to import legacy SmartGuide session")
	}
	// Earlier versions could leave the whole-file session at 0644. New logs are
	// created 0600, but the legacy file lingers as a backup and still holds the
	// full conversation, so tighten it rather than leave it world-readable.
	if err := os.Chmod(legacy, 0o600); err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).WithField("chatID", chatID).
			Debug("Failed to tighten legacy SmartGuide session mode")
	}
	return sess, nil
}

// Load returns the stored conversation for a chat, or nil if there is none. A
// read failure is treated as empty (logged, not fatal) so one bad session never
// blocks the user.
func (s *SessionStore) Load(chatID string) ([]anthropic.BetaMessageParam, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.open(chatID)
	if err != nil {
		logrus.WithError(err).WithField("chatID", chatID).
			Warn("SmartGuide session read failed, treating as empty")
		return nil, nil
	}
	return sess.Messages(), nil
}

// Save persists a chat's conversation, appending whatever part of messages is
// not already logged.
//
// Callers hand over the agent's full history, the shape they already had; the
// log works out the delta. Nothing on disk is rewritten, so an interrupted save
// can lose at most the turn in flight instead of truncating the file.
func (s *SessionStore) Save(chatID string, messages []anthropic.BetaMessageParam) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.open(chatID)
	if err != nil {
		return err
	}
	added, err := sess.AppendNew(messages)
	if err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"chatID": chatID, "appended": added, "total": len(messages),
	}).Debug("Saved SmartGuide session")
	return nil
}

// Clear ends a chat's current Smart Guide session: the live log is archived
// (renamed with a timestamp suffix) rather than deleted, so /clear deactivates
// the conversation instead of destroying it — the same "closed, not erased"
// semantics remote/session.Manager.Close gives @cc sessions. The next Load for
// chatID sees no file and starts fresh; the archived log is left on disk.
func (s *SessionStore) Clear(chatID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	archive := s.archivePath(chatID)
	if err := os.Rename(s.path(chatID), archive); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Archive any legacy file alongside it. Leaving it behind would resurrect
	// the cleared conversation: the next open finds an empty log and imports it.
	if err := os.Rename(s.legacyPath(chatID), archive+".legacy"); err != nil && !os.IsNotExist(err) {
		return err
	}
	logrus.WithField("chatID", chatID).Debug("Archived SmartGuide session")
	return nil
}

// archivePath returns a unique on-disk location for an archived (cleared)
// session log, distinct from path()'s canonical live-session location.
func (s *SessionStore) archivePath(chatID string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-smartguide.%d.jsonl", fs.SafeFileKey(chatID), time.Now().UnixNano()))
}
