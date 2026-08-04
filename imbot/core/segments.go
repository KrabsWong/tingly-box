package core

import "strings"

// segments.go centralizes the SendMessageOptions.Segments resolution rule.
//
// Design: Segments is authoritative-when-set (see bot.go). When non-empty it
// wins over Text/Entities/ParseMode; when empty the caller falls back to the
// legacy Text path, byte-identical to pre-Segments behavior. This mirrors how
// Actions superseded Metadata["replyMarkup"] — same pattern, same warn-on-both
// nudge, no forced migration.
//
// Segmentation itself is produced upstream (the gateway / stream assembler).
// imbot never derives segments from text; it only consumes what it is given.

// EffectiveSegments returns the authoritative segment sequence for the given
// options, or nil when the caller should fall back to the legacy Text path.
//
// Resolution:
//   - opts == nil or opts.Segments empty → returns nil (use Text/Entities/ParseMode).
//   - opts.Segments non-empty → returns opts.Segments; if Text is also set, a
//     one-time warning is logged so callers notice the shadowed field during
//     migration.
//
// Adapters use the nil result to select the legacy path, keeping Segments fully
// opt-in and zero-regression.
func EffectiveSegments(opts *SendMessageOptions) []Segment {
	if opts == nil || len(opts.Segments) == 0 {
		return nil
	}
	if opts.Text != "" {
		warn("SendMessageOptions: Segments is set and shadows Text/Entities/ParseMode; use one or the other")
	}
	return opts.Segments
}

// cloneSegments returns a deep copy of segs (including each segment's Entities
// slice), or nil when empty. Used by Message.Clone so an inbound message's
// segments are not shared across clones.
func cloneSegments(segs []Segment) []Segment {
	if len(segs) == 0 {
		return nil
	}
	out := make([]Segment, len(segs))
	for i, s := range segs {
		out[i] = s
		if len(s.Entities) > 0 {
			ent := make([]Entity, len(s.Entities))
			copy(ent, s.Entities)
			out[i].Entities = ent
		}
	}
	return out
}

// IsThinkingSegment reports whether s is a reasoning (non-body) segment.
func (s Segment) IsThinkingSegment() bool { return s.Kind == SegmentThinking }

// ResolveTextFromSegments checks whether opts carries an authoritative segment
// sequence and, if so, flattens it into Text/ParseMode in place so the caller's
// existing text-sending path handles it unchanged. Entities are cleared (the
// flattened output is plain text + optional markdown blockquote). Segments is
// reset to nil to prevent re-entry.
//
// Returns true when it applied a flattening (caller proceeds with the modified
// opts as a normal Text message), false when the caller should use opts as-is
// (no segments, legacy path, zero regression).
//
// This is the single integration point platforms use to support Segments via
// their existing text path:
//
//	core.ResolveTextFromSegments(opts, core.GetPlatformCapabilities(core.PlatformX).EffectiveThinkingRender())
func ResolveTextFromSegments(opts *SendMessageOptions, render ThinkingRender) bool {
	segs := EffectiveSegments(opts)
	if segs == nil {
		return false
	}
	f := FlattenSegments(segs, render)
	opts.Text = f.Text
	opts.ParseMode = f.ParseMode
	opts.Entities = nil
	opts.Segments = nil
	return true
}

// Flattened is the plain-text reduction of a segment sequence, used by
// platforms that lack native interleaving (no collapsible thinking block).
// Body segments are concatenated in order; thinking segments are reduced per
// the platform's ThinkingRender. Rich-format entities are intentionally not
// preserved on this path — degradation favors correctness of order and text
// over formatting fidelity.
type Flattened struct {
	Text      string
	ParseMode ParseMode // Markdown when any thinking blockquote was emitted; otherwise None
}

// FlattenSegments reduces an ordered segment sequence to a single text payload
// for platforms without native interleaving (discord, dingtalk, weixin, wecom,
// whatsapp). render declares how SegmentThinking should appear:
//
//   - Dimmed / Collapsed: thinking becomes a markdown blockquote (each line
//     prefixed with "> "), visually separated from adjacent body. Collapsed
//     degrades to Dimmed here since these platforms have no native fold.
//   - Hidden: thinking is dropped entirely (body-only output).
//   - Inline: thinking is appended as plain text, no special styling.
//
// Segments are joined with blank-line separators. When a blockquote is emitted
// the ParseMode is set to Markdown (required for the "> " syntax to render);
// otherwise it is None and the caller may treat the text as plain.
func FlattenSegments(segs []Segment, render ThinkingRender) Flattened {
	var sb strings.Builder
	mode := ParseModeNone

	first := true
	emit := func(text string) {
		if text == "" {
			return
		}
		if !first {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text)
		first = false
	}

	for _, s := range segs {
		switch {
		case s.Kind == SegmentBody:
			emit(s.Text)
		case s.Kind == SegmentThinking:
			switch render {
			case ThinkingRenderHidden:
				// drop
			case ThinkingRenderInline:
				emit(s.Text)
			default: // Dimmed, Collapsed, empty → blockquote
				if s.Text != "" {
					mode = ParseModeMarkdown
					emit(blockquote(s.Text))
				}
			}
		}
	}

	return Flattened{Text: sb.String(), ParseMode: mode}
}

// blockquote prefixes every line of text with "> " so it renders as a quoted,
// visually de-emphasized block in markdown renderers.
func blockquote(text string) string {
	var sb strings.Builder
	for i, line := range splitLines(text) {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("> ")
		sb.WriteString(line)
	}
	return sb.String()
}

// splitLines splits text on "\n" without dropping a trailing empty line the way
// strings.Split would expose it; empty input yields no lines.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
