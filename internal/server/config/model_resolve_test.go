package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newResolveTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	tm := data.NewEmbeddedOnlyTemplateManager()
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)
	return cfg
}

func codexResolveProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Codex OAuth",
		APIBase:  protocol.CodexAPIBase,
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-codex-token",
			Issuer:      ai.IssuerCodex,
		},
	}
}

// Codex's /models endpoint is unsupported, so the resolver must fall through
// to the embedded template (never persisted) and report source=template.
func TestResolveProviderModels_Codex_TemplateFallback(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))

	got, err := cfg.ResolveProviderModels(true, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceTemplate, got.Source)
	assert.Contains(t, got.Models, "gpt-5.5")
}

// A non-empty DB cache is served verbatim (source=db) and the upstream API is
// NOT re-queried when forceRefresh is false.
func TestResolveProviderModels_CacheHit_NotRefetched(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"cached-only"}, db.ModelSourceAPI))

	got, err := cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceCache, got.Source)
	assert.Equal(t, []string{"cached-only"}, got.Models)
}

// forceRefresh bypasses the cache: even with a cached list present, codex
// re-resolves to the template (its /models endpoint is unsupported, so no real
// network call happens, but the cache is intentionally skipped).
func TestResolveProviderModels_ForceRefresh_BypassesCache(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"stale-cached"}, db.ModelSourceAPI))

	got, err := cfg.ResolveProviderModels(true, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceTemplate, got.Source)
	assert.NotContains(t, got.Models, "stale-cached")
	assert.Contains(t, got.Models, "gpt-5.5")
}

// A provider that does not exist yields an error (used by the refresh path to
// surface a 500) rather than silently resolving to an empty list.
func TestResolveProviderModels_UnknownProvider_Errors(t *testing.T) {
	cfg := newResolveTestConfig(t)

	_, err := cfg.ResolveProviderModels(true, false, "does-not-exist")
	require.Error(t, err)
}

// claudeCodeResolveProvider builds a Claude Code OAuth provider, for which the
// upstream /models endpoint is normally unreachable (the OAuth token 404s).
func claudeCodeResolveProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Claude Code OAuth",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "sk-ant-oat01-testtoken",
			Issuer:      ai.IssuerClaudeCode,
		},
	}
}

// By default the gateway bans model-list fetches from a Claude Code OAuth
// upstream (the token cannot reach /models). The ban is recorded as a fetch
// failure and the resolver falls through to the embedded template — the same
// observable source as a provider whose /models is genuinely unsupported.
func TestResolveProviderModels_ClaudeCode_BannedByDefault(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := claudeCodeResolveProvider()
	require.NoError(t, cfg.AddProvider(p))

	got, err := cfg.ResolveProviderModels(true, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceTemplate, got.Source, "banned fetch falls back to template")

	// The ban must be recorded for triage — not just silently swallowed.
	lastErr, exists := cfg.GetModelManager().GetFetchFailure(p.UUID)
	require.True(t, exists, "expected a recorded fetch failure for the banned Claude Code fetch")
	assert.Contains(t, lastErr, "claude code upstream")
}

// With force_upstream, the ban is lifted and a real upstream fetch is
// attempted. This test points the provider at a dead local address so the
// fetch fails fast without depending on network reachability, and asserts the
// ban was NOT applied (no "do not allow ..." record) — the failure, if any,
// is the genuine connection error, not the ban.
func TestResolveProviderModels_ClaudeCode_ForceLiftsBan(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := claudeCodeResolveProvider()
	// Dead local address: the real fetch fails instantly, deterministically.
	p.APIBase = "http://127.0.0.1:1"
	require.NoError(t, cfg.AddProvider(p))

	_, _ = cfg.ResolveProviderModels(true, true, p.UUID)

	lastErr, exists := cfg.GetModelManager().GetFetchFailure(p.UUID)
	if exists {
		assert.NotContains(t, lastErr, "do not allow to list models from claude code upstream",
			"force_upstream must lift the ban, not record it")
	}
}
