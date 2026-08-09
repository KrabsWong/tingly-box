package usecase

import (
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newTestAgentConfig(t *testing.T) *serverconfig.Config {
	t.Helper()
	cfg, err := serverconfig.NewConfig(
		serverconfig.WithConfigDir(t.TempDir()),
		serverconfig.WithDisableBuiltIn(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return cfg
}

func TestAgentUseCase_RoutingKey(t *testing.T) {
	uc := NewAgentUseCase(newTestAgentConfig(t), "localhost")

	tests := []struct {
		name             string
		agentType        agent.AgentType
		wantRequestModel string
		wantScenario     typ.RuleScenario
		wantErr          bool
	}{
		{
			name:             "claude code",
			agentType:        agent.AgentTypeClaudeCode,
			wantRequestModel: "tingly/cc",
			wantScenario:     typ.ScenarioClaudeCode,
		},
		{
			name:             "opencode",
			agentType:        agent.AgentTypeOpenCode,
			wantRequestModel: "tingly-opencode",
			wantScenario:     typ.ScenarioOpenCode,
		},
		{
			name:             "codex",
			agentType:        agent.AgentTypeCodex,
			wantRequestModel: "tingly-codex",
			wantScenario:     typ.ScenarioCodex,
		},
		{
			name:      "unsupported type errors rather than falling back",
			agentType: agent.AgentType("not-a-real-agent"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestModel, scenario, err := uc.RoutingKey(tt.agentType)
			if tt.wantErr {
				var target ErrUnsupportedAgentType
				if !errors.As(err, &target) {
					t.Fatalf("expected ErrUnsupportedAgentType, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RoutingKey: %v", err)
			}
			if requestModel != tt.wantRequestModel {
				t.Errorf("requestModel = %q, want %q", requestModel, tt.wantRequestModel)
			}
			if scenario != tt.wantScenario {
				t.Errorf("scenario = %q, want %q", scenario, tt.wantScenario)
			}
		})
	}
}

func TestAgentUseCase_ResolveRouting(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	t.Run("no rule configured yet", func(t *testing.T) {
		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if res.RuleFound {
			t.Error("expected RuleFound=false with no rule configured")
		}
		if res.RequestModel != "tingly/cc" {
			t.Errorf("RequestModel = %q, want %q", res.RequestModel, "tingly/cc")
		}
	})

	t.Run("rule exists with a usable provider", func(t *testing.T) {
		provider := &typ.Provider{
			UUID: serverconfig.GenerateUUID(), Name: "test-provider",
			APIBase: "https://api.example.com", APIStyle: "openai",
			AuthType: typ.AuthTypeAPIKey, Token: "sk-test", Enabled: true,
		}
		if err := cfg.AddProvider(provider); err != nil {
			t.Fatalf("AddProvider: %v", err)
		}
		ruleUC := NewRuleUseCase(cfg)
		if _, err := ruleUC.Create(CreateRuleRequest{
			Scenario:     typ.ScenarioClaudeCode,
			RequestModel: "tingly/cc",
			Services: []*loadbalance.Service{
				{Provider: provider.UUID, Model: "claude-sonnet", Weight: 1, Active: true},
			},
		}); err != nil {
			t.Fatalf("RuleUseCase.Create: %v", err)
		}

		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if !res.RuleFound {
			t.Fatal("expected RuleFound=true")
		}
		if !res.ServiceUsable {
			t.Fatal("expected ServiceUsable=true with a valid provider")
		}
		if res.ProviderUUID != provider.UUID {
			t.Errorf("ProviderUUID = %q, want %q", res.ProviderUUID, provider.UUID)
		}
		if res.Model != "claude-sonnet" {
			t.Errorf("Model = %q, want %q", res.Model, "claude-sonnet")
		}
	})

	t.Run("unsupported agent type propagates the error", func(t *testing.T) {
		_, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentType("bogus")})
		var target ErrUnsupportedAgentType
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrUnsupportedAgentType, got %v", err)
		}
	})
}

func TestAgentUseCase_Show(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	t.Run("known agent type", func(t *testing.T) {
		res, err := uc.Show(ShowRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("Show: %v", err)
		}
		if res.Info.Type != agent.AgentTypeClaudeCode {
			t.Errorf("Info.Type = %q, want %q", res.Info.Type, agent.AgentTypeClaudeCode)
		}
		if res.Routing.RequestModel != "tingly/cc" {
			t.Errorf("Routing.RequestModel = %q, want %q", res.Routing.RequestModel, "tingly/cc")
		}
	})

	t.Run("unknown agent type", func(t *testing.T) {
		_, err := uc.Show(ShowRequest{AgentType: agent.AgentType("bogus")})
		if err == nil {
			t.Fatal("expected an error for an unknown agent type")
		}
	})
}
