package tui

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestPersistQuickstartProvider_ReentryUpdatesSameProvider(t *testing.T) {
	mgr := newTUIHarness(t)
	state := quickstartState{
		mgr:          mgr,
		providerName: "first-name",
		apiBase:      "https://first.example/v1",
		apiToken:     "first-token",
		apiStyle:     protocol.APIStyleOpenAI,
	}

	created, err := persistQuickstartProvider(state)
	if err != nil {
		t.Fatalf("first persist: %v", err)
	}
	if created.provider == nil || !created.providerCreated {
		t.Fatalf("created state = %#v", created)
	}
	originalUUID := created.provider.UUID

	created.providerName = "updated-name"
	created.apiBase = "https://updated.example/v1"
	updated, err := persistQuickstartProvider(created)
	if err != nil {
		t.Fatalf("second persist: %v", err)
	}

	providers := configuredProviders(mgr.GetGlobalConfig())
	if len(providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(providers))
	}
	if updated.provider.UUID != originalUUID || providers[0].UUID != originalUUID {
		t.Errorf("provider UUID changed: state=%q stored=%q want=%q", updated.provider.UUID, providers[0].UUID, originalUUID)
	}
	if providers[0].Name != "updated-name" || providers[0].APIBase != "https://updated.example/v1" {
		t.Errorf("provider was not updated: %#v", providers[0])
	}
}
