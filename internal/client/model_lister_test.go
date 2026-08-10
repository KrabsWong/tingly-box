package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestOpenAIClient_ListModels_ReturnsRawErrorResponse(t *testing.T) {
	const responseBody = `{"error":{"message":"rate limited","type":"rate_limit"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	provider := &typ.Provider{
		Name:     "openai-compatible",
		APIBase:  server.URL,
		APIStyle: protocol.APIStyleOpenAI,
		Token:    "test-token",
	}
	client, err := NewOpenAIClient(provider, "", typ.SessionID{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	result, err := client.ListModels(context.Background())
	require.Error(t, err)
	require.NotNil(t, result)
	raw, ok := result.Raw.(json.RawMessage)
	require.True(t, ok)
	assert.JSONEq(t, responseBody, string(raw))
}
