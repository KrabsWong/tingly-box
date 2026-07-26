package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTranscript(t *testing.T) (*Transcript, string) {
	t.Helper()
	dir := t.TempDir()
	tr, err := NewTranscript(dir)
	if err != nil {
		t.Fatalf("new transcript: %v", err)
	}
	return tr, dir
}

// TestTranscriptAppendsWithoutRewriting is the property the whole file-based
// design exists for: a message costs one append, and earlier bytes are never
// touched. The store this replaces re-marshalled every session on every
// message, which made a long conversation quadratic.
func TestTranscriptAppendsWithoutRewriting(t *testing.T) {
	tr, _ := newTranscript(t)
	const id = "sess-1"

	if err := tr.Append(id, Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("append first: %v", err)
	}
	sizeAfterFirst := fileSize(t, tr.Path(id))
	prefix := readBytes(t, tr.Path(id))

	if err := tr.Append(id, Message{Role: "assistant", Content: "second"}); err != nil {
		t.Fatalf("append second: %v", err)
	}

	full := readBytes(t, tr.Path(id))
	if len(full) <= sizeAfterFirst {
		t.Fatalf("file did not grow: %d then %d", sizeAfterFirst, len(full))
	}
	if string(full[:sizeAfterFirst]) != string(prefix) {
		t.Error("appending rewrote the existing bytes; the transcript is not append-only")
	}

	msgs, err := tr.Load(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Content != "first" || msgs[1].Content != "second" {
		t.Errorf("messages = %+v, want first then second", msgs)
	}
}

// TestTranscriptRoundTripsFields checks nothing is lost in the JSONL encoding.
func TestTranscriptRoundTripsFields(t *testing.T) {
	tr, _ := newTranscript(t)
	ts := time.Now().UTC().Truncate(time.Second)
	want := Message{Role: "assistant", Content: "body", Summary: "sum", Timestamp: ts}

	if err := tr.Append("s", want); err != nil {
		t.Fatalf("append: %v", err)
	}
	msgs, err := tr.Load("s")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Role != want.Role || got.Content != want.Content || got.Summary != want.Summary {
		t.Errorf("message = %+v, want %+v", got, want)
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("timestamp = %s, want %s", got.Timestamp, ts)
	}
}

// TestTranscriptSurvivesTornWrite covers a process killed mid-append: the
// trailing partial line must not make the completed history unreadable.
func TestTranscriptSurvivesTornWrite(t *testing.T) {
	tr, _ := newTranscript(t)
	const id = "sess-1"

	if err := tr.Append(id, Message{Role: "user", Content: "good"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	f, err := os.OpenFile(tr.Path(id), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString(`{"Role":"user","Content":"trunc`); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	f.Close()

	msgs, err := tr.Load(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "good" {
		t.Errorf("messages = %+v, want the one complete message", msgs)
	}
}

// TestTranscriptMissingIsEmpty: a session that never exchanged a message has
// no file, and that is not an error.
func TestTranscriptMissingIsEmpty(t *testing.T) {
	tr, _ := newTranscript(t)

	msgs, err := tr.Load("never-used")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("messages = %+v, want none", msgs)
	}
}

// TestTranscriptContainsTraversal guards the id-to-filename mapping. Session
// ids are usually ours, but CreateWithID binds a session to an id the user
// typed after /resume — so a hostile id must not escape the directory.
func TestTranscriptContainsTraversal(t *testing.T) {
	tr, dir := newTranscript(t)

	for _, id := range []string{"../../../etc/passwd", "..", ".", "a/b", "a\\b", "sess:1"} {
		got := tr.Path(id)
		if d := filepath.Dir(filepath.Clean(got)); d != filepath.Clean(dir) {
			t.Errorf("Path(%q) = %q, escapes %q", id, got, dir)
		}
		if strings.ContainsAny(filepath.Base(got), `/\`) {
			t.Errorf("Path(%q) basename %q still contains a separator", id, filepath.Base(got))
		}
	}

	// And a hostile id still round-trips, writing inside the directory.
	const hostile = "../../../etc/passwd"
	if err := tr.Append(hostile, Message{Role: "user", Content: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("store dir holds %d entries, want exactly 1", len(entries))
	}
	msgs, err := tr.Load(hostile)
	if err != nil || len(msgs) != 1 {
		t.Errorf("hostile id did not round-trip: msgs=%+v err=%v", msgs, err)
	}
}

// Note: the id-to-filename mapping itself is fs.SafeFileKey, tested in
// pkg/fs. What matters here is that the transcript routes through it.

// TestTranscriptFileIsOwnerOnly: transcripts hold the user's conversation.
func TestTranscriptFileIsOwnerOnly(t *testing.T) {
	tr, _ := newTranscript(t)
	if err := tr.Append("s", Message{Role: "user", Content: "secret"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	info, err := os.Stat(tr.Path("s"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %o, want 600", mode)
	}
}

// TestTranscriptDelete removes the file and is safe to repeat.
func TestTranscriptDelete(t *testing.T) {
	tr, _ := newTranscript(t)
	if err := tr.Append("s", Message{Role: "user", Content: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := tr.Delete("s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(tr.Path("s")); !os.IsNotExist(err) {
		t.Errorf("file survived delete: %v", err)
	}
	if err := tr.Delete("s"); err != nil {
		t.Errorf("second delete should be a no-op, got %v", err)
	}
}

// TestNilTranscriptIsUsable keeps a store built without a data dir working:
// history is dropped, nothing panics.
func TestNilTranscriptIsUsable(t *testing.T) {
	var tr *Transcript

	if got, err := NewTranscript(""); got != nil || err != nil {
		t.Fatalf("NewTranscript(\"\") = (%v, %v), want (nil, nil)", got, err)
	}
	if err := tr.Append("s", Message{Role: "user"}); err != nil {
		t.Errorf("append on nil: %v", err)
	}
	if msgs, err := tr.Load("s"); err != nil || len(msgs) != 0 {
		t.Errorf("load on nil = (%v, %v), want empty", msgs, err)
	}
	if err := tr.Delete("s"); err != nil {
		t.Errorf("delete on nil: %v", err)
	}
}

func fileSize(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return int(info.Size())
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
