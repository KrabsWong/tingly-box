package protocolserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// TestGuardrailsState_SetCarriesOverHistoryAndCredentials pins the swap
// semantics GuardrailsState inherited from root *Server: when a new runtime
// arrives without a history store or credential cache, both must carry over
// from the previous runtime so an admin-triggered policy reload doesn't wipe
// evaluation history or credential masking state.
//
// ConfigDir points at a regular file so the post-swap RefreshCredentialCache
// fails and leaves the carried-over cache intact (with a valid dir the
// refresh would legitimately rebuild the cache from the credential store).
func TestGuardrailsState_SetCarriesOverHistoryAndCredentials(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := NewGuardrailsState(&config.Config{ConfigDir: filepath.Join(notADir, "sub")})

	history := guardrailsutils.NewStore(10, "")
	cache := guardrails.NewCredentialCache()
	cache.ByID["cred-1"] = guardrailscore.ProtectedCredential{ID: "cred-1", Secret: "s3cret", Enabled: true}

	prev := &guardrails.Guardrails{}
	prev.SetHistoryStore(history)
	prev.SetCredentialCache(cache)
	g.SetRef(prev)

	// New runtime carries neither history nor credentials — both must survive.
	g.Set(&guardrails.Guardrails{}, "test reload")

	got := g.Current()
	if got == nil {
		t.Fatal("Current() = nil after Set")
	}
	if got.HistoryStore() != history {
		t.Error("history store did not carry over from previous runtime")
	}
	if _, ok := got.CredentialCacheSnapshot().ByID["cred-1"]; !ok {
		t.Error("credential cache did not carry over from previous runtime")
	}
}

// TestGuardrailsState_NilSafety pins that a nil *GuardrailsState (bare
// handlers in tests construct Deps without one) never panics.
func TestGuardrailsState_NilSafety(t *testing.T) {
	var g *GuardrailsState
	if g.Current() != nil {
		t.Error("nil state Current() must return nil")
	}
	g.SetRef(&guardrails.Guardrails{}) // must not panic
	if g.EnabledForScenario("claude_code") {
		t.Error("nil state EnabledForScenario must be false")
	}
}
