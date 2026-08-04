package core

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestEffectiveSegments_NilOptsReturnsNil(t *testing.T) {
	if got := EffectiveSegments(nil); got != nil {
		t.Fatalf("nil opts should fall back to Text path (nil segments), got %v", got)
	}
}

func TestEffectiveSegments_EmptySegmentsReturnsNil(t *testing.T) {
	opts := &SendMessageOptions{Text: "hi", Segments: nil}
	if got := EffectiveSegments(opts); got != nil {
		t.Fatalf("empty Segments should fall back to Text path, got %v", got)
	}
}

func TestEffectiveSegments_SegmentsAuthoritative(t *testing.T) {
	segs := []Segment{{Kind: SegmentBody, Text: "body"}, {Kind: SegmentThinking, Text: "thinking"}}
	opts := &SendMessageOptions{Segments: segs}
	got := EffectiveSegments(opts)
	if len(got) != 2 || got[0].Kind != SegmentBody || got[1].Kind != SegmentThinking {
		t.Fatalf("Segments should be returned as-is, got %v", got)
	}
}

func TestEffectiveSegments_WarnsWhenTextAlsoSet(t *testing.T) {
	// Capture the package logrus logger output to assert the shadow warning.
	var buf bytes.Buffer
	prevOut := logrus.StandardLogger().Out
	logrus.SetOutput(&buf)
	defer logrus.SetOutput(prevOut)

	opts := &SendMessageOptions{
		Text:     "shadowed",
		Segments: []Segment{{Kind: SegmentBody, Text: "body"}},
	}
	EffectiveSegments(opts)

	out := buf.String()
	if !strings.Contains(out, "Segments") || !strings.Contains(out, "Text") {
		t.Fatalf("warning should mention Segments and Text, got %q", out)
	}
	if !strings.Contains(out, "level=warning") {
		t.Fatalf("should be logged at warning level, got %q", out)
	}
}

func TestEffectiveSegments_NoWarnWhenOnlySegments(t *testing.T) {
	var buf bytes.Buffer
	prevOut := logrus.StandardLogger().Out
	logrus.SetOutput(&buf)
	defer logrus.SetOutput(prevOut)

	opts := &SendMessageOptions{Segments: []Segment{{Kind: SegmentBody, Text: "body"}}}
	EffectiveSegments(opts)
	if buf.Len() != 0 {
		t.Fatalf("expected no shadow warning when Text unset, got %q", buf.String())
	}
}

func TestCloneSegments_DeepCopy(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentBody, Text: "body", Entities: []Entity{{Type: "bold", Offset: 0, Length: 4}}},
		{Kind: SegmentThinking, Text: "thinking"},
	}
	cloned := cloneSegments(segs)

	if len(cloned) != 2 {
		t.Fatalf("expected 2 cloned segments, got %d", len(cloned))
	}
	// Mutating the original must not affect the clone.
	segs[0].Text = "mutated"
	segs[0].Entities[0].Type = "italic"
	if cloned[0].Text != "body" {
		t.Fatalf("clone should not share segment storage, Text=%q", cloned[0].Text)
	}
	if cloned[0].Entities[0].Type != "bold" {
		t.Fatalf("clone should not share entity storage, Type=%q", cloned[0].Entities[0].Type)
	}
}

func TestCloneSegments_NilSafe(t *testing.T) {
	if got := cloneSegments(nil); got != nil {
		t.Fatalf("cloneSegments(nil) should return nil, got %v", got)
	}
	if got := cloneSegments([]Segment{}); got != nil {
		t.Fatalf("cloneSegments(empty) should return nil, got %v", got)
	}
}

func TestTextContentClone_PreservesSegments(t *testing.T) {
	msg := &Message{
		Content: &TextContent{
			Text: "body",
			Segments: []Segment{
				{Kind: SegmentThinking, Text: "reasoning"},
				{Kind: SegmentBody, Text: "answer"},
			},
		},
	}
	clone := msg.Clone()
	tc, ok := clone.Content.(*TextContent)
	if !ok {
		t.Fatalf("clone content should be TextContent")
	}
	if len(tc.Segments) != 2 || tc.Segments[0].Kind != SegmentThinking || tc.Segments[1].Kind != SegmentBody {
		t.Fatalf("clone should preserve segments, got %v", tc.Segments)
	}
}

func TestPlatformCapabilities_EffectiveThinkingRender_Default(t *testing.T) {
	var caps *PlatformCapabilities // nil
	if got := caps.EffectiveThinkingRender(); got != ThinkingRenderDimmed {
		t.Fatalf("nil caps should default to dimmed, got %s", got)
	}
	empty := &PlatformCapabilities{}
	if got := empty.EffectiveThinkingRender(); got != ThinkingRenderDimmed {
		t.Fatalf("unset ThinkingRender should default to dimmed, got %s", got)
	}
}

func TestPlatformCapabilities_EffectiveThinkingRender_Declared(t *testing.T) {
	caps := &PlatformCapabilities{ThinkingRender: ThinkingRenderCollapsed}
	if got := caps.EffectiveThinkingRender(); got != ThinkingRenderCollapsed {
		t.Fatalf("declared ThinkingRender should be respected, got %s", got)
	}
}

func TestSegment_IsThinkingSegment(t *testing.T) {
	if !(Segment{Kind: SegmentThinking}).IsThinkingSegment() {
		t.Fatal("thinking segment should report true")
	}
	if (Segment{Kind: SegmentBody}).IsThinkingSegment() {
		t.Fatal("body segment should report false")
	}
}

func TestFlattenSegments_BodyOnly(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentBody, Text: "hello"},
		{Kind: SegmentBody, Text: "world"},
	}
	f := FlattenSegments(segs, ThinkingRenderDimmed)
	if f.Text != "hello\n\nworld" {
		t.Fatalf("body segments should be joined with blank line, got %q", f.Text)
	}
	if f.ParseMode != ParseModeNone {
		t.Fatalf("body-only should be ParseModeNone, got %s", f.ParseMode)
	}
}

func TestFlattenSegments_ThinkingDimmed(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentThinking, Text: "reasoning here"},
		{Kind: SegmentBody, Text: "answer"},
	}
	f := FlattenSegments(segs, ThinkingRenderDimmed)
	// thinking becomes a blockquote, body follows, separated by blank line
	want := "> reasoning here\n\nanswer"
	if f.Text != want {
		t.Fatalf("dimmed thinking should be blockquoted, got %q", f.Text)
	}
	if f.ParseMode != ParseModeMarkdown {
		t.Fatalf("blockquote requires Markdown parse mode, got %s", f.ParseMode)
	}
}

func TestFlattenSegments_ThinkingMultilineBlockquote(t *testing.T) {
	segs := []Segment{{Kind: SegmentThinking, Text: "line1\nline2"}}
	f := FlattenSegments(segs, ThinkingRenderDimmed)
	want := "> line1\n> line2"
	if f.Text != want {
		t.Fatalf("each line should be quoted, got %q", f.Text)
	}
}

func TestFlattenSegments_ThinkingHidden(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentThinking, Text: "secret reasoning"},
		{Kind: SegmentBody, Text: "answer"},
	}
	f := FlattenSegments(segs, ThinkingRenderHidden)
	if f.Text != "answer" {
		t.Fatalf("hidden thinking should be dropped, got %q", f.Text)
	}
	if f.ParseMode != ParseModeNone {
		t.Fatalf("no blockquote → ParseModeNone, got %s", f.ParseMode)
	}
}

func TestFlattenSegments_ThinkingInline(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentThinking, Text: "thinking"},
		{Kind: SegmentBody, Text: "answer"},
	}
	f := FlattenSegments(segs, ThinkingRenderInline)
	want := "thinking\n\nanswer"
	if f.Text != want {
		t.Fatalf("inline thinking should append plain, got %q", f.Text)
	}
	if f.ParseMode != ParseModeNone {
		t.Fatalf("inline → ParseModeNone, got %s", f.ParseMode)
	}
}

func TestFlattenSegments_CollapsedDegradesToDimmed(t *testing.T) {
	// Collapsed has no native fold on the degradation path → behaves as Dimmed.
	segs := []Segment{{Kind: SegmentThinking, Text: "reasoning"}, {Kind: SegmentBody, Text: "answer"}}
	f := FlattenSegments(segs, ThinkingRenderCollapsed)
	if f.Text != "> reasoning\n\nanswer" {
		t.Fatalf("collapsed should degrade to blockquote, got %q", f.Text)
	}
	if f.ParseMode != ParseModeMarkdown {
		t.Fatalf("should be Markdown, got %s", f.ParseMode)
	}
}

func TestFlattenSegments_Interleaved(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentThinking, Text: "think1"},
		{Kind: SegmentBody, Text: "body1"},
		{Kind: SegmentThinking, Text: "think2"},
		{Kind: SegmentBody, Text: "body2"},
	}
	f := FlattenSegments(segs, ThinkingRenderDimmed)
	want := "> think1\n\nbody1\n\n> think2\n\nbody2"
	if f.Text != want {
		t.Fatalf("interleaved order must be preserved, got %q", f.Text)
	}
}

func TestResolveTextFromSegments_AppliesFlattening(t *testing.T) {
	opts := &SendMessageOptions{
		Segments: []Segment{
			{Kind: SegmentThinking, Text: "reasoning"},
			{Kind: SegmentBody, Text: "answer"},
		},
	}
	applied := ResolveTextFromSegments(opts, ThinkingRenderDimmed)
	if !applied {
		t.Fatal("should report applied=true when segments present")
	}
	if opts.Text != "> reasoning\n\nanswer" {
		t.Fatalf("Text should hold flattened payload, got %q", opts.Text)
	}
	if opts.ParseMode != ParseModeMarkdown {
		t.Fatalf("ParseMode should be Markdown, got %s", opts.ParseMode)
	}
	if opts.Entities != nil {
		t.Fatal("Entities should be cleared")
	}
	if opts.Segments != nil {
		t.Fatal("Segments should be cleared to prevent re-entry")
	}
}

func TestResolveTextFromSegments_NoSegmentsReturnsFalse(t *testing.T) {
	opts := &SendMessageOptions{Text: "plain"}
	applied := ResolveTextFromSegments(opts, ThinkingRenderDimmed)
	if applied {
		t.Fatal("should return false when no segments")
	}
	if opts.Text != "plain" {
		t.Fatal("opts should be untouched when no segments")
	}
}
