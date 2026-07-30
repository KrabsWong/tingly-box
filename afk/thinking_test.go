package afk

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// thinkingParamOf returns the thinking object from a captured request body, or
// nil when the request carried none.
func thinkingParamOf(req capturedRequest) map[string]any {
	t, _ := req.body["thinking"].(map[string]any)
	return t
}

func runOnceWithThinking(t *testing.T, mode ThinkingMode) capturedRequest {
	t.Helper()
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("ok") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Thinking: mode})
	require.NoError(t, err)
	_, _, err = eng.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	require.Len(t, srv.reqs, 1)
	return srv.reqs[0]
}

// The zero value sends nothing, which is the only mode guaranteed to be
// accepted by whatever model the gateway routes to.
func TestThinking_DefaultSendsNoParameter(t *testing.T) {
	req := runOnceWithThinking(t, ThinkingModelDefault)
	assert.Nil(t, thinkingParamOf(req))
	assert.NotContains(t, string(req.rawBody), `"thinking"`)
}

// "visible" is the mode that makes OnThinking useful: without display
// summarized, current models return thinking blocks with empty text.
func TestThinking_VisibleAsksForSummarizedDisplay(t *testing.T) {
	think := thinkingParamOf(runOnceWithThinking(t, ThinkingVisible))
	require.NotNil(t, think)
	assert.Equal(t, "adaptive", think["type"])
	assert.Equal(t, "summarized", think["display"],
		"reasoning only comes back with content when it is asked for")
}

func TestThinking_HiddenAsksForAdaptiveWithoutContent(t *testing.T) {
	think := thinkingParamOf(runOnceWithThinking(t, ThinkingHidden))
	require.NotNil(t, think)
	assert.Equal(t, "adaptive", think["type"])
	assert.Equal(t, "omitted", think["display"])
}

func TestThinking_OffDisables(t *testing.T) {
	think := thinkingParamOf(runOnceWithThinking(t, ThinkingOff))
	require.NotNil(t, think)
	assert.Equal(t, "disabled", think["type"])
	assert.NotContains(t, think, "display", "there is nothing to display when off")
}

// An unrecognized value falls back to sending nothing rather than guessing. The
// mode arrives as a plain string from config, so a typo must not become a
// request the model rejects.
func TestThinking_UnknownModeSendsNothing(t *testing.T) {
	req := runOnceWithThinking(t, ThinkingMode("enabled-with-budget"))
	assert.Nil(t, thinkingParamOf(req))
}
