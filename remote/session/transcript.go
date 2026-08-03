package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Transcript stores a session's message history as one append-only JSONL file
// per session.
//
// Why files and not rows. Messages are written once, read whole or not at all,
// never queried by content, and grow without bound with the length of a
// conversation. That is the opposite of what a table is good for: in SQLite
// every append becomes a transaction against the database every other part of
// the product shares, and the database file inflates with conversation text
// that nothing ever selects on. As a file per session, an append is one
// O_APPEND write, no session's history sits in another's file, and the
// transcript stays something a user can tail, grep, and hand to a bug report.
// (One mutex still serialises this process's writes, so a large message cannot
// be torn by an interleaving append — that is about write integrity, not about
// sessions sharing storage.)
//
// This mirrors how Claude Code keeps its own sessions, which matters here
// beyond taste: a remote session can be bound to a Claude on-disk session id
// (see Manager.CreateWithID, used by /resume), so the two halves of one
// conversation stay the same kind of artifact.
//
// What stays in SQLite is the session INDEX — binding, status, timestamps —
// because that genuinely needs indexed lookup, and it is small and bounded.
// The split is by access pattern, not by preference for one medium.
type Transcript struct {
	dir string
	mu  sync.Mutex
}

// NewTranscript creates a transcript store rooted at dir. A blank dir returns
// (nil, nil): a nil *Transcript is usable and simply drops history, which
// keeps sessions working in tests and in stores built without a data dir.
func NewTranscript(dir string) (*Transcript, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create transcript dir: %w", err)
	}
	return &Transcript{dir: dir}, nil
}

// Path returns the on-disk transcript file for a session.
//
// The id goes through safeFileKey because it is not always ours:
// Manager.CreateWithID binds a session to a Claude session id supplied by the
// user through /resume, so "../../etc/x" must not escape the directory.
func (t *Transcript) Path(sessionID string) string {
	if t == nil {
		return ""
	}
	return filepath.Join(t.dir, safeFileKey(sessionID)+".jsonl")
}

// Append writes one message to the end of a session's transcript.
//
// O_APPEND means the cost does not grow with the conversation — the defect
// that made the previous whole-file store quadratic in message count.
func (t *Transcript) Append(sessionID string, msg Message) error {
	if t == nil || sessionID == "" {
		return nil
	}

	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.OpenFile(t.Path(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	return nil
}

// Load reads a session's full history. A missing transcript is empty, not an
// error — a session that never exchanged a message has no file.
//
// A malformed line is skipped rather than failing the whole read: a torn
// final write (killed mid-append) must not make the preceding history
// unreadable.
func (t *Transcript) Load(sessionID string) ([]Message, error) {
	if t == nil || sessionID == "" {
		return nil, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.Open(t.Path(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var out []Message
	scanner := bufio.NewScanner(f)
	// Allow long single messages; the default 64KiB token limit is easily hit
	// by a pasted file or a long model response.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read transcript: %w", err)
	}
	return out, nil
}

// Delete removes a session's transcript. A missing file is not an error.
func (t *Transcript) Delete(sessionID string) error {
	if t == nil || sessionID == "" {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := os.Remove(t.Path(sessionID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete transcript: %w", err)
	}
	return nil
}
