package afk

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Default truncation limits for tool output. Two independent caps: whichever is
// reached first wins. The byte cap is what actually protects the context window
// (one 200KB line is as damaging as ten thousand short ones); the line cap is
// what keeps ordinary command output readable.
const (
	DefaultMaxOutputLines = 2000
	DefaultMaxOutputBytes = 50 * 1024
)

// TruncateOptions configures a single truncation. Zero values mean "use the
// default cap", so the zero TruncateOptions is a usable head-truncation config.
type TruncateOptions struct {
	MaxLines int
	MaxBytes int
	// KeepTail retains the end of the output instead of the beginning. Command
	// output belongs here — the failure and the summary are at the bottom, and
	// a build log's first 2000 lines are the least interesting part of it.
	// File reads want the default (head), which matches reading from an offset.
	KeepTail bool
}

// Truncation is the result of a Truncate call: the text to hand back, plus what
// it took to get there so the caller can tell the model what it is missing.
type Truncation struct {
	Text       string
	Truncated  bool
	TotalLines int
	KeptLines  int
	TotalBytes int
	KeptBytes  int
	// keptTail records which end was retained, for Notice.
	keptTail bool
}

// Notice returns a one-line explanation to append to the tool result, or "" if
// nothing was dropped. It states what was cut and how to get the rest: a model
// that knows it is looking at a tail can re-run with a narrower command, while
// one handed a silently-truncated result will reason from it as if complete.
func (t Truncation) Notice() string { return t.NoticeFrom(0) }

// NoticeFrom is Notice for a caller that reads by offset, so the model can be
// told absolute line numbers and where to resume. firstLine is the 1-based
// number of the input's first line; 0 means the caller has no offset to page
// with (a command's output, say) and reads exactly as Notice.
//
// It exists so the offset case extends the shared notice instead of hand-rolling
// a second one. The notice is a contract with the model — what got cut and how
// to get the rest — and two wordings for that is one too many.
func (t Truncation) NoticeFrom(firstLine int) string {
	if !t.Truncated {
		return ""
	}
	recovery := "Re-run with a narrower command, a filter such as grep/head/tail, or redirect to a file and read ranges from it."
	var what string
	switch {
	case firstLine >= 1:
		last := firstLine + t.KeptLines - 1
		what = fmt.Sprintf("showing lines %d-%d of the %d selected", firstLine, last, t.TotalLines)
		recovery = fmt.Sprintf("Re-read with offset=%d to continue from where this stops.", last+1)
	case t.keptTail:
		what = fmt.Sprintf("showing the last %d of %d lines", t.KeptLines, t.TotalLines)
	default:
		what = fmt.Sprintf("showing the first %d of %d lines", t.KeptLines, t.TotalLines)
	}
	return fmt.Sprintf("[Output truncated: %s (%d of %d bytes). %s]",
		what, t.KeptBytes, t.TotalBytes, recovery)
}

// String returns the truncated text with the notice appended when one applies,
// which is what a tool wants to return to the model.
func (t Truncation) String() string { return t.StringFrom(0) }

// StringFrom is String with NoticeFrom's absolute line numbering.
func (t Truncation) StringFrom(firstLine int) string {
	if !t.Truncated {
		return t.Text
	}
	notice := t.NoticeFrom(firstLine)
	if t.Text == "" {
		return notice
	}
	return t.Text + "\n\n" + notice
}

// Truncate caps s to the configured line and byte limits, cutting on line
// boundaries so output is never left mid-line — except when a single line alone
// exceeds the byte cap, where it is cut at a UTF-8 rune boundary rather than
// splitting a character.
func Truncate(s string, opt TruncateOptions) Truncation {
	if opt.MaxLines <= 0 {
		opt.MaxLines = DefaultMaxOutputLines
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = DefaultMaxOutputBytes
	}

	lines := strings.Split(s, "\n")
	// A trailing newline produces a final empty element that is a terminator,
	// not a line; counting it would report one line more than the output has.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	res := Truncation{
		Text:       s,
		TotalLines: len(lines),
		TotalBytes: len(s),
		keptTail:   opt.KeepTail,
	}
	if len(lines) <= opt.MaxLines && len(s) <= opt.MaxBytes {
		res.KeptLines, res.KeptBytes = res.TotalLines, res.TotalBytes
		return res
	}

	kept := lines
	if len(kept) > opt.MaxLines {
		if opt.KeepTail {
			kept = kept[len(kept)-opt.MaxLines:]
		} else {
			kept = kept[:opt.MaxLines]
		}
	}

	// Walk out from the retained edge, taking whole lines until the next one
	// would cross the byte cap.
	size, count := 0, 0
	for i := range kept {
		idx := i
		if opt.KeepTail {
			idx = len(kept) - 1 - i
		}
		lineSize := len(kept[idx]) + 1 // the newline it is followed by
		if size+lineSize > opt.MaxBytes {
			break
		}
		size += lineSize
		count++
	}

	if count == 0 {
		// The single line at the edge is itself over the cap; cut within it.
		edge := kept[0]
		if opt.KeepTail {
			edge = cutTail(kept[len(kept)-1], opt.MaxBytes)
		} else {
			edge = cutHead(edge, opt.MaxBytes)
		}
		res.Text = edge
		res.Truncated = true
		res.KeptLines = 1
		res.KeptBytes = len(edge)
		return res
	}

	if opt.KeepTail {
		kept = kept[len(kept)-count:]
	} else {
		kept = kept[:count]
	}

	res.Text = strings.Join(kept, "\n")
	res.Truncated = true
	res.KeptLines = len(kept)
	res.KeptBytes = len(res.Text)
	return res
}

// cutHead returns the first max bytes of s, backing off to the nearest rune
// boundary so the result is never invalid UTF-8.
func cutHead(s string, max int) string {
	if len(s) <= max {
		return s
	}
	b := s[:max]
	for len(b) > 0 && !utf8.ValidString(b) {
		b = b[:len(b)-1]
	}
	return b
}

// cutTail returns the last max bytes of s, advancing to the nearest rune
// boundary so the result is never invalid UTF-8.
func cutTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	b := s[len(s)-max:]
	for len(b) > 0 && !utf8.ValidString(b) {
		b = b[1:]
	}
	return b
}
