package afk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingLog captures what a Harness checkpoints, and when.
type recordingLog struct {
	mu       sync.Mutex
	appended []anthropic.BetaMessageParam
	batches  [][]anthropic.BetaMessageParam
	failWith error
}

func (l *recordingLog) Append(msgs ...anthropic.BetaMessageParam) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failWith != nil {
		return l.failWith
	}
	l.appended = append(l.appended, msgs...)
	l.batches = append(l.batches, msgs)
	return nil
}

func (l *recordingLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.appended)
}

func (l *recordingLog) roles() []anthropic.BetaMessageParamRole {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]anthropic.BetaMessageParamRole, 0, len(l.appended))
	for _, m := range l.appended {
		out = append(out, m.Role)
	}
	return out
}

func lastRole(msgs []anthropic.BetaMessageParam) anthropic.BetaMessageParamRole {
	return msgs[len(msgs)-1].Role
}

// TestHarness_CheckpointsEachStep is the durability property: a multi-step run
// writes as it goes rather than only at the end, so an interruption keeps the
// work already done.
func TestHarness_CheckpointsEachStep(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("72F"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	assert.Equal(t, 2, res.Steps)
	assert.Equal(t, len(res.Messages), log.count(), "everything the run produced should be logged")

	// Two checkpoints, one per step — not a single flush at the end.
	require.Len(t, log.batches, 2, "each completed step should checkpoint")
	assert.Len(t, log.batches[0], 3, "first batch: user prompt, assistant tool_use, tool results")
	assert.Len(t, log.batches[1], 1, "second batch: the final assistant turn")
}

// TestHarness_KeepsWorkWhenStepFails is what the checkpoint is for: the first
// step ran tools whose side effects already happened, and a failure on the
// second must not throw that away.
func TestHarness_KeepsWorkWhenStepFails(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tool := &fakeTool{name: "get_weather", result: "72F"}
	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "weather?", &recordingSink{})
	require.Error(t, err, "the failing model call should surface")
	assert.True(t, res.Interrupted)

	assert.Equal(t, int32(1), atomic.LoadInt32(&tool.called), "the tool really did run")
	assert.GreaterOrEqual(t, log.count(), 3,
		"the completed step must be durable even though the run then failed")
}

// TestHarness_InterruptedRunLeavesReplayableTranscript pins the invariant a
// partial run has to satisfy: it may not end on a user turn. Appending the next
// user message to a transcript that already ends on one puts two in a row,
// which the API rejects — so persisting partial runs would otherwise wedge the
// conversation permanently.
func TestHarness_InterruptedRunLeavesReplayableTranscript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, toolUseResponse("toolu_loop", "loop_tool", `{"city":"x"}`))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		MaxIterations: 2,
		Tools:         []Tool{&fakeTool{name: "loop_tool", result: "still going"}},
	})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "loop", &recordingSink{})
	require.NoError(t, err)
	assert.True(t, res.Interrupted)

	// The run ended on tool results, which are a user turn; the harness closes
	// it out so the transcript alternates.
	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, lastRole(res.Messages),
		"an interrupted transcript must not end on a user turn")
	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, lastRole(log.appended),
		"and what was persisted must satisfy the same invariant")
	assert.Equal(t, len(res.Messages), log.count())
}

// A model call that fails on the very first step leaves only the user prompt,
// which is a user turn — the same invariant applies.
func TestHarness_FirstStepFailureStillLeavesReplayableTranscript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hello?", &recordingSink{})
	require.Error(t, err)

	require.NotEmpty(t, res.Messages)
	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, lastRole(res.Messages))
	assert.Equal(t, []anthropic.BetaMessageParamRole{
		anthropic.BetaMessageParamRoleUser,
		anthropic.BetaMessageParamRoleAssistant,
	}, log.roles(), "the prompt is kept, closed out so the next turn can be appended")
}

// A run that ends normally already ends on an assistant turn; no note is added.
func TestHarness_CompletedRunGetsNoInterruptionNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("all done"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	assert.False(t, res.Interrupted)
	assert.Equal(t, "all done", res.FinalText)
	require.Len(t, res.Messages, 2)
	assert.Equal(t, "all done", res.Messages[1].Content[0].OfText.Text,
		"a completed run should not be annotated")
}

// Cancellation mid-run is the /stop path: the work so far is kept and the
// transcript is closed out.
func TestHarness_CancelledRunIsCheckpointedAndClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("should not get here"))
	}))
	defer srv.Close()

	// Cancel once the first step's tool has run, so the second step never starts.
	tool := &cancellingTool{name: "get_weather", result: "72F", cancel: cancel}
	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(ctx, nil, "weather?", &recordingSink{})
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, res.Interrupted)

	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, lastRole(res.Messages))
	assert.Equal(t, len(res.Messages), log.count(), "cancelled work is still durable")
	assert.Equal(t, int32(1), atomic.LoadInt32(&reqCount), "the next step never started")
}

// A log that cannot be written must not fail the run: the tool side effects
// already happened, and dropping the turn would lose them without undoing them.
func TestHarness_LogFailureDoesNotFailTheRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("fine"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{failWith: errors.New("disk full")}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err, "a persistence failure is not a run failure")
	assert.Equal(t, "fine", res.FinalText)
}

// A nil log means "no durability wanted", which is how Engine.Run behaves.
func TestHarness_NilLogRunsNormally(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("no log needed"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	res, err := NewHarness(eng, nil).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	assert.Equal(t, "no log needed", res.FinalText)
	assert.Len(t, res.Messages, 2)
}

// cancellingTool cancels the run's context from inside a tool call, which is
// where a real /stop lands: after the model has committed to tools.
type cancellingTool struct {
	name   string
	result string
	cancel context.CancelFunc
}

func (c *cancellingTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        c.name,
		Description: anthropic.String("cancels the run"),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{"city": map[string]any{"type": "string"}},
		},
	}
}

func (c *cancellingTool) Call(ctx context.Context, raw json.RawMessage) (string, error) {
	c.cancel()
	return c.result, nil
}
