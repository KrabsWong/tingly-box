package usecase

import (
	"fmt"
	"sort"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProfileUseCase assembles profile metadata and routing details without
// imposing a CLI, TUI, or HTTP presentation. Profiles are an intentional
// product concept; this use case makes that concept reusable rather than
// treating the top-level profile command as an alias for another command.
type ProfileUseCase struct {
	cfg *serverconfig.Config
}

// NewProfileUseCase constructs a ProfileUseCase over the given config.
func NewProfileUseCase(cfg *serverconfig.Config) *ProfileUseCase {
	return &ProfileUseCase{cfg: cfg}
}

// ErrProfileNotFound means no profile matched the supplied ID or name within
// the base scenario.
type ErrProfileNotFound struct {
	Scenario   typ.RuleScenario
	Identifier string
}

func (e ErrProfileNotFound) Error() string {
	return fmt.Sprintf("profile %q not found in scenario %q", e.Identifier, e.Scenario)
}

// ListProfilesRequest identifies the base scenario whose profiles should be
// returned.
type ListProfilesRequest struct {
	Scenario typ.RuleScenario `json:"scenario"`
}

// ListProfilesResult is the output of List.
type ListProfilesResult struct {
	Profiles []typ.ProfileMeta `json:"profiles"`
}

// List returns a detached copy of the profiles for a base scenario.
func (uc *ProfileUseCase) List(req ListProfilesRequest) ListProfilesResult {
	return ListProfilesResult{Profiles: uc.cfg.GetProfiles(req.Scenario)}
}

// GetProfileRequest identifies a profile by ID or exact name.
type GetProfileRequest struct {
	Scenario   typ.RuleScenario `json:"scenario"`
	Identifier string           `json:"identifier"`
}

// ResolveProfileResult contains the canonical profile identity for a supplied
// name or ID. Launch callers use this result without paying to assemble rule
// presentation data.
type ResolveProfileResult struct {
	Profile  typ.ProfileMeta  `json:"profile"`
	Scenario typ.RuleScenario `json:"scenario"`
}

// Resolve converts a profile name or ID into canonical metadata and its
// profiled scenario.
func (uc *ProfileUseCase) Resolve(req GetProfileRequest) (ResolveProfileResult, error) {
	resolvedID, err := uc.cfg.ResolveProfileNameOrID(req.Scenario, req.Identifier)
	if err != nil || resolvedID == "" {
		return ResolveProfileResult{}, ErrProfileNotFound{Scenario: req.Scenario, Identifier: req.Identifier}
	}

	meta, found := uc.cfg.GetProfile(req.Scenario, resolvedID)
	if !found {
		return ResolveProfileResult{}, ErrProfileNotFound{Scenario: req.Scenario, Identifier: req.Identifier}
	}
	return ResolveProfileResult{
		Profile:  meta,
		Scenario: typ.ProfiledScenarioName(req.Scenario, resolvedID),
	}, nil
}

// ProfileRule describes the display-relevant portion of one rule belonging
// to a profile. ProviderName falls back to ProviderUUID when the referenced
// provider no longer exists.
type ProfileRule struct {
	RequestModel string `json:"request_model"`
	ProviderUUID string `json:"provider_uuid,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	Model        string `json:"model,omitempty"`
	Configured   bool   `json:"configured"`
	Active       bool   `json:"active"`
}

// GetProfileResult contains resolved profile identity plus its concrete
// profiled scenario and routing rules.
type GetProfileResult struct {
	Profile  typ.ProfileMeta  `json:"profile"`
	Scenario typ.RuleScenario `json:"scenario"`
	Rules    []ProfileRule    `json:"rules"`
}

// Get resolves a profile name or ID and assembles its routing details.
func (uc *ProfileUseCase) Get(req GetProfileRequest) (GetProfileResult, error) {
	resolved, err := uc.Resolve(req)
	if err != nil {
		return GetProfileResult{}, err
	}

	result := GetProfileResult{
		Profile:  resolved.Profile,
		Scenario: resolved.Scenario,
	}

	for i := range uc.cfg.Rules {
		rule := &uc.cfg.Rules[i]
		if rule.Scenario != resolved.Scenario {
			continue
		}

		view := ProfileRule{RequestModel: rule.RequestModel, Active: rule.Active}
		if len(rule.Services) > 0 {
			service := rule.Services[0]
			view.ProviderUUID = service.Provider
			view.ProviderName = service.Provider
			view.Model = service.Model
			view.Configured = service.Provider != ""
			if provider, lookupErr := uc.cfg.GetProviderByUUID(service.Provider); lookupErr == nil && provider != nil {
				view.ProviderName = provider.Name
			}
		}
		result.Rules = append(result.Rules, view)
	}

	sort.Slice(result.Rules, func(i, j int) bool {
		return result.Rules[i].RequestModel < result.Rules[j].RequestModel
	})
	return result, nil
}
