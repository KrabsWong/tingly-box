package usecase

import (
	"errors"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestRuleConfig returns a config with builtin rules disabled and one
// enabled provider seeded, so callers can build valid Services without
// tripping validateRuleServices.
func newTestRuleConfig(t *testing.T) (*serverconfig.Config, string) {
	t.Helper()
	cfg, err := serverconfig.NewConfig(
		serverconfig.WithConfigDir(t.TempDir()),
		serverconfig.WithDisableBuiltIn(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	provider := &typ.Provider{
		UUID:     serverconfig.GenerateUUID(),
		Name:     "test-provider",
		APIBase:  "https://api.example.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test",
		Enabled:  true,
	}
	if err := cfg.AddProvider(provider); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	return cfg, provider.UUID
}

func TestErrRuleExists_Error(t *testing.T) {
	err := ErrRuleExists{RequestModel: "gpt-4", Scenario: typ.ScenarioOpenAI, UUID: "rule-1"}
	got := err.Error()
	for _, want := range []string{"gpt-4", "openai", "rule-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, expected it to contain %q", got, want)
		}
	}
}

func TestErrRuleNotFound_Error(t *testing.T) {
	err := ErrRuleNotFound{UUID: "rule-1"}
	if got := err.Error(); !strings.Contains(got, "rule-1") {
		t.Errorf("Error() = %q, expected it to contain %q", got, "rule-1")
	}
}

func TestRuleUseCase_Create(t *testing.T) {
	cfg, providerUUID := newTestRuleConfig(t)
	uc := NewRuleUseCase(cfg)

	tests := []struct {
		name           string
		req            CreateRuleRequest
		wantRuleExists bool // Create should return ErrRuleExists
		wantErr        bool // Create should return some other error (underlying config validation)
	}{
		{
			name: "creates a new rule",
			req: CreateRuleRequest{
				Scenario:     typ.ScenarioOpenAI,
				RequestModel: "gpt-4",
				Services: []*loadbalance.Service{
					{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
				},
			},
		},
		{
			name: "duplicate request-model+scenario is rejected",
			req: CreateRuleRequest{
				Scenario:     typ.ScenarioOpenAI,
				RequestModel: "gpt-4",
				Services: []*loadbalance.Service{
					{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
				},
			},
			wantRuleExists: true,
		},
		{
			name: "same request-model different scenario is allowed",
			req: CreateRuleRequest{
				Scenario:     typ.ScenarioAnthropic,
				RequestModel: "gpt-4",
				Services: []*loadbalance.Service{
					{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
				},
			},
		},
		{
			name: "service referencing a non-existent provider is rejected by the underlying config",
			req: CreateRuleRequest{
				Scenario:     typ.ScenarioOpenAI,
				RequestModel: "gpt-4-invalid-provider",
				Services: []*loadbalance.Service{
					{Provider: "does-not-exist", Model: "gpt-4", Weight: 1, Active: true},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := uc.Create(tt.req)
			if tt.wantRuleExists {
				var target ErrRuleExists
				if !errors.As(err, &target) {
					t.Fatalf("expected ErrRuleExists, got %v", err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				var exists ErrRuleExists
				if errors.As(err, &exists) {
					t.Fatalf("expected a non-ErrRuleExists error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if res.Rule.UUID == "" {
				t.Fatal("expected a generated UUID")
			}
			if res.Rule.RequestModel != tt.req.RequestModel {
				t.Errorf("RequestModel = %q, want %q", res.Rule.RequestModel, tt.req.RequestModel)
			}
		})
	}
}

func TestRuleUseCase_Get(t *testing.T) {
	cfg, providerUUID := newTestRuleConfig(t)
	uc := NewRuleUseCase(cfg)

	created, err := uc.Create(CreateRuleRequest{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		res, err := uc.Get(GetRuleRequest{UUID: created.Rule.UUID})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if res.Rule.UUID != created.Rule.UUID {
			t.Errorf("UUID = %q, want %q", res.Rule.UUID, created.Rule.UUID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.Get(GetRuleRequest{UUID: "does-not-exist"})
		var target ErrRuleNotFound
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrRuleNotFound, got %v", err)
		}
	})
}

func TestRuleUseCase_List(t *testing.T) {
	cfg, providerUUID := newTestRuleConfig(t)
	uc := NewRuleUseCase(cfg)

	before := len(uc.List().Rules)

	if _, err := uc.Create(CreateRuleRequest{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
		},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := len(uc.List().Rules); got != before+1 {
		t.Fatalf("expected %d rules after Create, got %d", before+1, got)
	}
}

func TestRuleUseCase_UpdateService(t *testing.T) {
	cfg, providerUUID := newTestRuleConfig(t)
	uc := NewRuleUseCase(cfg)

	created, err := uc.Create(CreateRuleRequest{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("updates services, leaves the rest alone", func(t *testing.T) {
		res, err := uc.UpdateService(UpdateServiceRequest{
			UUID: created.Rule.UUID,
			Services: []*loadbalance.Service{
				{Provider: providerUUID, Model: "gpt-4-turbo", Weight: 1, Active: true},
			},
		})
		if err != nil {
			t.Fatalf("UpdateService: %v", err)
		}
		if len(res.Rule.Services) != 1 || res.Rule.Services[0].Model != "gpt-4-turbo" {
			t.Errorf("Services not updated: %+v", res.Rule.Services)
		}
		if res.Rule.RequestModel != created.Rule.RequestModel {
			t.Errorf("RequestModel changed: got %q, want %q", res.Rule.RequestModel, created.Rule.RequestModel)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.UpdateService(UpdateServiceRequest{UUID: "does-not-exist"})
		var target ErrRuleNotFound
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrRuleNotFound, got %v", err)
		}
	})

	t.Run("service referencing a non-existent provider is rejected by the underlying config", func(t *testing.T) {
		_, err := uc.UpdateService(UpdateServiceRequest{
			UUID: created.Rule.UUID,
			Services: []*loadbalance.Service{
				{Provider: "does-not-exist", Model: "gpt-4", Weight: 1, Active: true},
			},
		})
		if err == nil {
			t.Fatal("expected an error for a service referencing a non-existent provider")
		}
		var notFound ErrRuleNotFound
		if errors.As(err, &notFound) {
			t.Fatalf("expected a validation error, not ErrRuleNotFound: %v", err)
		}
	})
}

func TestRuleUseCase_Delete(t *testing.T) {
	cfg, providerUUID := newTestRuleConfig(t)
	uc := NewRuleUseCase(cfg)

	created, err := uc.Create(CreateRuleRequest{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: "gpt-4", Weight: 1, Active: true},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("deletes an existing rule", func(t *testing.T) {
		if err := uc.Delete(DeleteRuleRequest{UUID: created.Rule.UUID}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := uc.Get(GetRuleRequest{UUID: created.Rule.UUID}); err == nil {
			t.Fatal("expected rule to be gone after Delete")
		}
	})

	t.Run("not found", func(t *testing.T) {
		err := uc.Delete(DeleteRuleRequest{UUID: "does-not-exist"})
		var target ErrRuleNotFound
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrRuleNotFound, got %v", err)
		}
	})
}
