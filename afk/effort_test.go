package afk

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runOnceWithEffort(t *testing.T, level EffortLevel) capturedRequest {
	t.Helper()
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("ok") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Effort: level})
	require.NoError(t, err)
	_, _, err = eng.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	require.Len(t, srv.reqs, 1)
	return srv.reqs[0]
}

func effortOf(req capturedRequest) any {
	oc, ok := req.body["output_config"].(map[string]any)
	if !ok {
		return nil
	}
	return oc["effort"]
}

func TestEffort_DefaultSendsNoParameter(t *testing.T) {
	req := runOnceWithEffort(t, EffortModelDefault)
	assert.Nil(t, effortOf(req))
	assert.NotContains(t, string(req.rawBody), "output_config")
}

func TestEffort_EveryLevelReachesTheWire(t *testing.T) {
	for _, level := range []EffortLevel{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax} {
		t.Run(string(level), func(t *testing.T) {
			assert.Equal(t, string(level), effortOf(runOnceWithEffort(t, level)))
		})
	}
}

// Same typo guard as thinking: the level arrives as a plain string from config.
func TestEffort_UnknownLevelSendsNothing(t *testing.T) {
	assert.Nil(t, effortOf(runOnceWithEffort(t, EffortLevel("very-high"))))
}

// The two are separate axes and must not interfere: setting both puts both on
// the wire, unchanged.
func TestThinkingAndEffort_AreIndependent(t *testing.T) {
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("ok") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "k", Model: "m",
		Thinking: ThinkingVisible,
		Effort:   EffortHigh,
	})
	require.NoError(t, err)
	_, _, err = eng.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)

	req := srv.reqs[0]
	think := thinkingParamOf(req)
	require.NotNil(t, think)
	assert.Equal(t, "adaptive", think["type"])
	assert.Equal(t, "summarized", think["display"])
	assert.Equal(t, "high", effortOf(req))
}
