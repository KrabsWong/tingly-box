package tui

import (
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// RunRuleMode is the entry point for the Rule mode loop.
func RunRuleMode(mgr TUIManager) error {
	for {
		items := []SelectItem[string]{
			{Title: "List", Description: "Show all routing rules", Value: "list"},
			{Title: "Add", Description: "Create a new rule", Value: "add"},
			{Title: "Edit", Description: "Re-pick the service on an existing rule", Value: "edit"},
			{Title: "Delete", Description: "Remove a rule", Value: "delete"},
			{Title: "Back", Description: "Return to the main menu", Value: "back"},
		}
		r, err := Select("Rule:", items, SelectOptions{
			Header:    titleStyle.Render("Tingly Box · TUI · Rule"),
			CanGoBack: true,
			PageSize:  10,
		})
		if err != nil {
			return err
		}
		if r.IsCancel() || r.IsBack() || r.Value == "back" {
			return nil
		}

		var opErr error
		switch r.Value {
		case "list":
			opErr = ruleList(mgr)
		case "add":
			opErr = ruleAdd(mgr)
		case "edit":
			opErr = ruleEdit(mgr)
		case "delete":
			opErr = ruleDelete(mgr)
		}
		if opErr != nil && opErr != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", opErr)))
		}
	}
}

func ruleList(mgr TUIManager) error {
	rules := usecase.NewRuleUseCase(mgr.GetGlobalConfig()).List().Rules
	if len(rules) == 0 {
		fmt.Println(descStyle.Render("No rules configured."))
		Pause("")
		return nil
	}
	fmt.Println()
	fmt.Println(promptStyle.Render(fmt.Sprintf("%d rule(s):", len(rules))))
	for i := range rules {
		r := &rules[i]
		svc := formatRuleService(mgr, r)
		fmt.Printf("  %d. %s  %s\n",
			i+1,
			valueStyle.Render(r.RequestModel),
			descStyle.Render("("+string(r.Scenario)+")"))
		fmt.Println(descStyle.Render(fmt.Sprintf("     uuid:    %s", r.UUID)))
		fmt.Println(descStyle.Render(fmt.Sprintf("     service: %s", svc)))
	}
	fmt.Println()
	Pause("")
	return nil
}

func formatRuleService(mgr TUIManager, r *typ.Rule) string {
	if len(r.Services) == 0 {
		return "(none)"
	}
	s := r.Services[0]
	label := s.Provider
	if result, err := usecase.NewProviderUseCase(mgr.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: s.Provider}); err == nil {
		label = result.Provider.Name
	}
	extra := ""
	if len(r.Services) > 1 {
		extra = fmt.Sprintf(" (+%d more)", len(r.Services)-1)
	}
	return fmt.Sprintf("%s:%s%s", label, s.Model, extra)
}

func ruleAdd(mgr TUIManager) error {
	scn, ok, err := pickScenario(typ.ScenarioOpenAI)
	if err != nil || !ok {
		return err
	}

	rmR, err := Input("Request model (e.g. gpt-4o, claude-3-5-sonnet):", InputOptions{Required: true, CanGoBack: true})
	if err != nil || rmR.IsCancel() || rmR.IsBack() {
		return nil
	}

	svc, err := pickRuleService(mgr)
	if err != nil || svc == nil {
		return err
	}

	cfm, err := Confirm("Save this rule?", ConfirmOptions{DefaultYes: true, CanGoBack: true,
		Description: fmt.Sprintf("%s · %s → %s:%s", rmR.Value, scn, providerName(mgr, svc.Provider), svc.Model)})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}

	ruleUC := usecase.NewRuleUseCase(mgr.GetGlobalConfig())
	res, err := ruleUC.Create(usecase.CreateRuleRequest{
		Scenario:     scn,
		RequestModel: rmR.Value,
		Services:     []*loadbalance.Service{svc},
	})
	if err != nil {
		var exists usecase.ErrRuleExists
		if errors.As(err, &exists) {
			return fmt.Errorf("a rule for %q + %q already exists (uuid %s); use Edit instead",
				exists.RequestModel, exists.Scenario, exists.UUID)
		}
		return err
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ Rule added (uuid: %s).", res.Rule.UUID)))
	Pause("")
	return nil
}

func ruleEdit(mgr TUIManager) error {
	rule, err := pickRule(mgr, "Select rule to edit:")
	if err != nil || rule == nil {
		return err
	}
	fmt.Println(descStyle.Render(fmt.Sprintf("Current service: %s", formatRuleService(mgr, rule))))

	svc, err := pickRuleService(mgr)
	if err != nil || svc == nil {
		return err
	}

	cfm, err := Confirm("Apply update?", ConfirmOptions{DefaultYes: true, CanGoBack: true,
		Description: fmt.Sprintf("new service: %s:%s", providerName(mgr, svc.Provider), svc.Model)})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}

	ruleUC := usecase.NewRuleUseCase(mgr.GetGlobalConfig())
	if _, err := ruleUC.UpdateService(usecase.UpdateServiceRequest{
		UUID:     rule.UUID,
		Services: []*loadbalance.Service{svc},
	}); err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓ Rule updated."))
	Pause("")
	return nil
}

func ruleDelete(mgr TUIManager) error {
	rule, err := pickRule(mgr, "Select rule to delete:")
	if err != nil || rule == nil {
		return err
	}
	cfm, err := Confirm(fmt.Sprintf("Delete rule '%s' (%s)?", rule.RequestModel, rule.Scenario), ConfirmOptions{
		DefaultYes: false, CanGoBack: true,
	})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}

	ruleUC := usecase.NewRuleUseCase(mgr.GetGlobalConfig())
	if err := ruleUC.Delete(usecase.DeleteRuleRequest{UUID: rule.UUID}); err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓ Rule deleted."))
	Pause("")
	return nil
}

func pickRule(mgr TUIManager, prompt string) (*typ.Rule, error) {
	ruleUC := usecase.NewRuleUseCase(mgr.GetGlobalConfig())
	rules := ruleUC.List().Rules
	if len(rules) == 0 {
		fmt.Println(descStyle.Render("No rules configured."))
		Pause("")
		return nil, nil
	}
	items := make([]SelectItem[string], 0, len(rules))
	for i := range rules {
		r := &rules[i]
		items = append(items, SelectItem[string]{
			Title:       r.RequestModel,
			Description: fmt.Sprintf("%s · %s", r.Scenario, formatRuleService(mgr, r)),
			Value:       r.UUID,
		})
	}
	sel, err := Select(prompt, items, SelectOptions{CanGoBack: true, PageSize: 12})
	if err != nil {
		return nil, err
	}
	if sel.IsCancel() || sel.IsBack() {
		return nil, nil
	}
	result, err := ruleUC.Get(usecase.GetRuleRequest{UUID: sel.Value})
	if err != nil {
		return nil, err
	}
	return &result.Rule, nil
}

func pickRuleService(mgr TUIManager) (*loadbalance.Service, error) {
	p, err := pickProvider(mgr, "Provider for this rule:")
	if err != nil || p == nil {
		return nil, err
	}
	model, err := pickProviderModel(mgr, p, "Model on "+p.Name+":")
	if err != nil || model == "" {
		return nil, err
	}
	return &loadbalance.Service{
		Provider: p.UUID,
		Model:    model,
		Weight:   1,
		Active:   true,
	}, nil
}

// pickProviderModel offers a Select over the provider's models. If no
// models are cached yet it first refreshes them from the provider so the
// user gets a picker rather than a free-form Input. The list still
// includes a "Custom…" escape hatch for vendors that don't return a
// listable catalog. Returns ("", nil) on cancel.
func pickProviderModel(mgr TUIManager, p *typ.Provider, prompt string) (string, error) {
	models := availableModels(mgr, p)
	if len(models) == 0 {
		_, _ = WithSpinner("Fetching models from "+p.Name, func() (struct{}, error) {
			_, err := usecase.NewProviderUseCase(mgr.GetGlobalConfig()).RefreshModels(usecase.RefreshModelsRequest{UUID: p.UUID})
			return struct{}{}, err
		})
		models = availableModels(mgr, p)
	}

	if len(models) == 0 {
		r, err := Input(prompt, InputOptions{
			Placeholder: "e.g. gpt-4o, claude-3-5-sonnet-20241022",
			Required:    true,
			CanGoBack:   true,
		})
		if err != nil || r.IsCancel() || r.IsBack() {
			return "", err
		}
		return r.Value, nil
	}

	items := make([]SelectItem[string], 0, len(models)+1)
	for _, m := range models {
		items = append(items, SelectItem[string]{Title: m, Value: m})
	}
	items = append(items, SelectItem[string]{Title: "Custom…", Description: "Enter a model name manually", Value: ""})

	sel, err := Select(prompt, items, SelectOptions{CanGoBack: true, PageSize: 12})
	if err != nil || sel.IsCancel() || sel.IsBack() {
		return "", err
	}
	if sel.Value != "" {
		return sel.Value, nil
	}
	r, err := Input("Model name:", InputOptions{Required: true, CanGoBack: true})
	if err != nil || r.IsCancel() || r.IsBack() {
		return "", err
	}
	return r.Value, nil
}

// availableModels returns the model list for a provider with the same
// fallback order FetchAndSaveProviderModels uses internally:
//
//  1. DB-cached models (populated by a successful provider /v1/models call)
//  2. compile-time embedded template (GetEmbeddedModelsForProvider) — used
//     when the upstream API has no models endpoint (Anthropic, OAuth-only
//     providers, etc.) or the call failed.
//
// Without the second step the UI shows an empty list for providers whose
// catalogs only exist as build-time data — exactly the case the user hit.
func availableModels(mgr TUIManager, p *typ.Provider) []string {
	cfg := mgr.GetGlobalConfig()
	if cfg == nil {
		return nil
	}
	if mm := cfg.GetModelManager(); mm != nil {
		if models := mm.GetModels(p.UUID); len(models) > 0 {
			return models
		}
	}
	if tm := cfg.GetTemplateManager(); tm != nil {
		if models, err := tm.GetEmbeddedModelsForProvider(p); err == nil {
			return models
		}
	}
	return nil
}

func providerName(mgr TUIManager, uuid string) string {
	if result, err := usecase.NewProviderUseCase(mgr.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: uuid}); err == nil {
		return result.Provider.Name
	}
	return uuid
}
