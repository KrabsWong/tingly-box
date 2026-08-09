package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/config"
	exportpkg "github.com/tingly-dev/tingly-box/internal/dataio"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// AppManager is the command process host: it owns AppConfig and server
// lifecycle. Domain behavior belongs in internal/usecase rather than here.
type AppManager struct {
	appConfig *config.AppConfig
}

// NewAppManager creates a new AppManager with the given config directory.
func NewAppManager(configDir string) (*AppManager, error) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}

	return &AppManager{
		appConfig: appConfig,
	}, nil
}

// NewAppManagerWithConfig creates a new AppManager with an existing AppConfig.
func NewAppManagerWithConfig(appConfig *config.AppConfig) *AppManager {
	return &AppManager{
		appConfig: appConfig,
	}
}

// AppConfig returns the underlying AppConfig.
func (am *AppManager) AppConfig() *config.AppConfig {
	return am.appConfig
}

// GetGlobalConfig returns the global configuration manager.
func (am *AppManager) GetGlobalConfig() *serverconfig.Config {
	return am.appConfig.GetGlobalConfig()
}

// ============
// Server Management
// ============

// StartServerAt initializes and starts the in-process server used by the TUI.
func (am *AppManager) StartServerAt(port int) error {
	serverManager := NewServerManager(am.appConfig)
	if err := serverManager.Setup(port); err != nil {
		return err
	}
	return serverManager.Start()
}

// ============
// Configuration Accessors
// ============

// GetRuntimeServerPort returns the port the running server is actually
// listening on. The server port is intentionally not persisted in the config
// file, so a server started with --port would be invisible to other CLI
// processes; the server therefore records its port in a runtime port file
// next to the PID lock. When the server is running (lock held) and the port
// file is readable, that port wins; otherwise this falls back to the
// configured port.
func (am *AppManager) GetRuntimeServerPort() int {
	fileLock := lock.NewFileLock(am.appConfig.ConfigDir())
	if fileLock.IsLocked() {
		if port, err := fileLock.ReadPort(); err == nil {
			return port
		}
	}
	return am.appConfig.GetServerPort()
}

// ============
// Import/Export Types
// ============

// ImportOptions controls how imports are handled when conflicts occur.
type ImportOptions struct {
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string
	// Quiet suppresses progress output
	Quiet bool
}

// ProviderImportInfo contains information about an imported or used provider
type ProviderImportInfo struct {
	UUID   string
	Name   string
	Action string // "created", "used", "skipped"
}

// ImportResult contains the results of an import operation.
type ImportResult struct {
	ProvidersCreated int
	ProvidersUsed    int
	Providers        []ProviderImportInfo
	ProviderMap      map[string]string // old UUID -> new UUID
}

// convertProviderInfoList converts dataio.ProviderImportInfo to command.ProviderImportInfo
func convertProviderInfoList(dataioList []exportpkg.ProviderImportInfo) []ProviderImportInfo {
	result := make([]ProviderImportInfo, len(dataioList))
	for i, p := range dataioList {
		result[i] = ProviderImportInfo{
			UUID:   p.UUID,
			Name:   p.Name,
			Action: p.Action,
		}
	}
	return result
}

// ============
// Export
// ============

// CollectProvidersFromRule collects all providers referenced by the rule's services.
// This is a helper function for gathering providers to export with a rule.
func (am *AppManager) CollectProvidersFromRule(rule *typ.Rule) ([]*typ.Provider, error) {
	globalConfig := am.appConfig.GetGlobalConfig()

	providerUUIDs := am.getProviderUUIDsFromRule(rule)
	providers := make([]*typ.Provider, 0, len(providerUUIDs))

	for _, providerUUID := range providerUUIDs {
		provider, err := globalConfig.GetProviderByUUID(providerUUID)
		if err == nil && provider != nil {
			providers = append(providers, provider)
		}
	}

	return providers, nil
}

// ExportRule exports the providers referenced by a rule (or an explicit
// provider list) in the specified format. dataio export/import is
// provider-only; the rule itself is only used here, by the caller, to pick
// which providers to include — it is not part of the exported payload.
func (am *AppManager) ExportRule(rule *typ.Rule, providers []*typ.Provider, format exportpkg.Format) (string, error) {
	if len(providers) == 0 {
		return "", fmt.Errorf("providers must be specified for export")
	}

	// Build export request
	req := &exportpkg.ExportRequest{
		Providers: providers,
	}

	// Perform export
	result, err := exportpkg.Export(req, format)
	if err != nil {
		return "", fmt.Errorf("failed to export: %w", err)
	}

	return result.Content, nil
}

// getProviderUUIDsFromRule extracts all provider UUIDs from the rule's services
func (am *AppManager) getProviderUUIDsFromRule(rule *typ.Rule) []string {
	uuids := make(map[string]bool)
	for _, service := range rule.Services {
		if service.Provider != "" {
			uuids[service.Provider] = true
		}
	}

	result := make([]string, 0, len(uuids))
	for uuid := range uuids {
		result = append(result, uuid)
	}
	return result
}

// ============
// Import
// ============

// ImportRule imports providers from data in the specified format. Despite
// the name (kept for call-site compatibility), only providers are
// imported — dataio export/import no longer carries rule data.
func (am *AppManager) ImportRule(data string, format exportpkg.Format, opts ImportOptions) (*ImportResult, error) {
	globalConfig := am.appConfig.GetGlobalConfig()

	// Convert command.ImportOptions to import.ImportOptions
	importOpts := exportpkg.ImportOptions{
		OnProviderConflict: opts.OnProviderConflict,
		Quiet:              opts.Quiet,
	}

	// Perform import
	result, err := exportpkg.Import(data, globalConfig, format, importOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to import providers: %w", err)
	}

	// Convert import.ImportResult to command.ImportResult
	return &ImportResult{
		ProvidersCreated: result.ProvidersCreated,
		ProvidersUsed:    result.ProvidersUsed,
		Providers:        convertProviderInfoList(result.Providers),
		ProviderMap:      result.ProviderMap,
	}, nil
}
