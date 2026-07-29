package afk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isSummarizeRequest reports whether a captured request body is the
// summarization call rather than a normal agent step. The summarizer runs with
// no tools, which is the one structural difference. Routing on shape instead of
// a request counter matters because the SDK retries 5xx responses, which shifts
// every count.
func isSummarizeRequest(body []byte) bool {
	return !bytes.Contains(body, []byte(`"tools"`))
}

// routeByShape serves agent steps and the summarization call differently.
func routeByShape(t *testing.T, onStep func(n int) string, onSummarize func() (string, int)) http.HandlerFunc {
	t.Helper()
	var steps int32
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if isSummarizeRequest(raw) {
			body, status := onSummarize()
			if status != 0 {
				http.Error(w, body, status)
				return
			}
			writeSSE(t, w, body)
			return
		}
		writeSSE(t, w, onStep(int(atomic.AddInt32(&steps, 1))))
	}
}

func userTurn(text string) anthropic.BetaMessageParam {
	return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text))
}

func assistantTurn(text string) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(text)},
	}
}

func assistantToolUse(id, name string) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role: anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{
			{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: id, Name: name, Input: map[string]any{}}},
		},
	}
}

func toolResultTurn(id, out string) anthropic.BetaMessageParam {
	return anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, out, false))
}

// The boundary must land on a real user turn. Cutting on a tool_result orphans
// it from the tool_use that asked for it, which the API rejects outright.
func TestCompactionBoundary_LandsOnAUserTurn(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{
		userTurn("q1"), assistantTurn("a1"),
		userTurn("q2"), assistantTurn("a2"),
		userTurn("q3"), assistantTurn("a3"),
		userTurn("q4"), assistantTurn("a4"),
	}

	b := compactionBoundary(msgs, 4)
	require.Greater(t, b, 0)
	assert.True(t, isConversationTurnStart(msgs[b]), "the cut must open an exchange")
	assert.Equal(t, anthropic.BetaMessageParamRoleUser, msgs[b].Role)
}

func TestCompactionBoundary_SkipsToolResultMessages(t *testing.T) {
	// The ideal boundary lands inside a tool loop; it must move forward to the
	// next genuine user turn rather than cutting a tool_result loose.
	msgs := []anthropic.BetaMessageParam{
		userTurn("q1"), assistantTurn("a1"),
		userTurn("q2"),
		assistantToolUse("t1", "read"), toolResultTurn("t1", "contents"),
		assistantToolUse("t2", "read"), toolResultTurn("t2", "more"),
		assistantTurn("a2"),
		userTurn("q3"), assistantTurn("a3"),
	}

	b := compactionBoundary(msgs, 6)
	require.Greater(t, b, 0)
	assert.True(t, isConversationTurnStart(msgs[b]),
		"boundary %d landed on a tool_result or assistant turn", b)

	// Everything kept must be self-contained: no tool_result whose tool_use was
	// left behind.
	kept := msgs[b:]
	seen := map[string]bool{}
	for _, m := range kept {
		for _, blk := range m.Content {
			if blk.OfToolUse != nil {
				seen[blk.OfToolUse.ID] = true
			}
			if blk.OfToolResult != nil {
				assert.True(t, seen[blk.OfToolResult.ToolUseID],
					"tool_result %s kept without its tool_use", blk.OfToolResult.ToolUseID)
			}
		}
	}
}

// A tail that is one unbroken tool loop has no safe cut. Compaction must give
// up rather than cut somewhere that breaks the conversation.
func TestCompactionBoundary_GivesUpWhenNoSafeCutExists(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{userTurn("go")}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("t%d", i)
		msgs = append(msgs, assistantToolUse(id, "read"), toolResultTurn(id, "out"))
	}
	assert.Equal(t, 0, compactionBoundary(msgs, 6), "no clean turn boundary in the tail")
}

func TestCompactionBoundary_ShortConversationIsNotCompacted(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{userTurn("q"), assistantTurn("a")}
	assert.Equal(t, 0, compactionBoundary(msgs, 12))
}

// TestHarness_CompactsWhenPromptExceedsBudget is the end-to-end behavior: an
// oversized prompt is summarized between steps instead of growing until the
// request fails.
func TestHarness_CompactsWhenPromptExceedsBudget(t *testing.T) {
	srv := httptest.NewServer(routeByShape(t,
		func(n int) string {
			if n == 1 {
				// A huge prompt, and the model wants another step.
				return oversizedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`, 500_000)
			}
			return textResponse("done")
		},
		func() (string, int) {
			return textResponse("Earlier: the user asked about weather; we read one file."), 0
		},
	))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	// Seed enough history that there is something to compact.
	var history []anthropic.BetaMessageParam
	for i := 0; i < 20; i++ {
		history = append(history, userTurn(fmt.Sprintf("q%d", i)), assistantTurn(fmt.Sprintf("a%d", i)))
	}

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), history, "and now?", &recordingSink{})
	require.NoError(t, err)

	assert.Equal(t, 1, res.Compactions, "the oversized prompt should have triggered one compaction")
	require.Len(t, log.compactions, 1, "compaction must be recorded in the log")
	assert.Contains(t, log.compactions[0].summary, "Earlier: the user asked about weather")
	assert.Greater(t, log.compactions[0].replaced, 0)

	// The conversation shrank, and still starts on a user turn carrying the summary.
	assert.Less(t, len(res.Messages), len(history)+4, "compaction should have dropped older messages")
	require.NotEmpty(t, res.Messages)
	assert.Equal(t, anthropic.BetaMessageParamRoleUser, res.Messages[0].Role)
	require.NotNil(t, res.Messages[0].Content[0].OfText)
	assert.Contains(t, res.Messages[0].Content[0].OfText.Text, summaryPrefix,
		"the summary rides on the first surviving message")
}

// A conversation inside the budget must not be touched — compaction costs a
// model call and loses detail, so it only happens when it has to.
func TestHarness_DoesNotCompactWhenUnderBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("short answer"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	var history []anthropic.BetaMessageParam
	for i := 0; i < 20; i++ {
		history = append(history, userTurn(fmt.Sprintf("q%d", i)), assistantTurn(fmt.Sprintf("a%d", i)))
	}

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), history, "hi", &recordingSink{})
	require.NoError(t, err)

	assert.Zero(t, res.Compactions)
	assert.Empty(t, log.compactions)
	assert.Len(t, res.Messages, len(history)+2)
}

// If the summarizing call fails, the run continues on the full conversation. An
// oversized prompt still has a chance of succeeding; a half-compacted one does
// not.
func TestHarness_CompactionFailureLeavesConversationIntact(t *testing.T) {
	srv := httptest.NewServer(routeByShape(t,
		func(n int) string {
			if n == 1 {
				return oversizedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`, 500_000)
			}
			return textResponse("done anyway")
		},
		func() (string, int) { return "summarizer down", http.StatusInternalServerError },
	))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	var history []anthropic.BetaMessageParam
	for i := 0; i < 20; i++ {
		history = append(history, userTurn(fmt.Sprintf("q%d", i)), assistantTurn(fmt.Sprintf("a%d", i)))
	}

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), history, "and now?", &recordingSink{})
	require.NoError(t, err)

	assert.Zero(t, res.Compactions)
	assert.Empty(t, log.compactions)
	assert.Equal(t, "done anyway", res.FinalText, "the run still finished")
	assert.GreaterOrEqual(t, len(res.Messages), len(history), "nothing was dropped")
}

// If the compaction cannot be recorded, it must not be applied either: the log
// is what the next turn is rebuilt from, so the two must not diverge.
func TestHarness_CompactionNotAppliedWhenItCannotBeRecorded(t *testing.T) {
	srv := httptest.NewServer(routeByShape(t,
		func(n int) string {
			if n == 1 {
				return oversizedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`, 500_000)
			}
			return textResponse("done")
		},
		func() (string, int) { return textResponse("a summary"), 0 },
	))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	var history []anthropic.BetaMessageParam
	for i := 0; i < 20; i++ {
		history = append(history, userTurn(fmt.Sprintf("q%d", i)), assistantTurn(fmt.Sprintf("a%d", i)))
	}

	log := &recordingLog{failCompactWith: assert.AnError}
	res, err := NewHarness(eng, log).Run(context.Background(), history, "and now?", &recordingSink{})
	require.NoError(t, err)

	assert.Zero(t, res.Compactions, "an unrecordable compaction must not be applied")
	assert.GreaterOrEqual(t, len(res.Messages), len(history))
}

// Compaction can be turned off, in which case a long conversation eventually
// fails on its own rather than being summarized.
func TestHarness_CompactionCanBeDisabled(t *testing.T) {
	var summarized int32
	srv := httptest.NewServer(routeByShape(t,
		func(n int) string {
			if n == 1 {
				return oversizedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`, 500_000)
			}
			return textResponse("done")
		},
		func() (string, int) {
			atomic.AddInt32(&summarized, 1)
			return textResponse("should not be called"), 0
		},
	))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		CompactAtTokens: -1,
		Tools:           []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	var history []anthropic.BetaMessageParam
	for i := 0; i < 20; i++ {
		history = append(history, userTurn(fmt.Sprintf("q%d", i)), assistantTurn(fmt.Sprintf("a%d", i)))
	}

	res, err := NewHarness(eng, nil).Run(context.Background(), history, "and now?", &recordingSink{})
	require.NoError(t, err)
	assert.Zero(t, res.Compactions)
	assert.Zero(t, atomic.LoadInt32(&summarized), "no summarization call should be made")
}

// oversizedToolUseResponse is toolUseResponse with a large reported input-token
// count, so the harness sees a prompt over budget without the test having to
// build a genuinely enormous conversation.
func oversizedToolUseResponse(id, name, inputJSON string, inputTokens int64) string {
	w := &sseWriter{}
	w.event("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_big","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":%d,"output_tokens":1}}}`, inputTokens))
	w.event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, id, name))
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%q}}`, inputJSON))
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}
