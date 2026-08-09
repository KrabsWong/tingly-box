package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tingly-dev/tingly-box/internal/command"
	exportpkg "github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TinglyService manages the web UI and HTTP server functionality
type TinglyService struct {
	appManager    *command.AppManager
	serverManager *command.ServerManager
	app           *application.App
}

// NewTinglyServiceWithServerManager creates a new UI service instance with a pre-configured ServerManager
func NewTinglyServiceWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager) *TinglyService {
	res := &TinglyService{
		appManager:    appManager,
		serverManager: serverManager,
	}

	log.Printf("config file: %s\n", appManager.AppConfig().GetGlobalConfig().ConfigFile)

	return res
}

// Start starts the UI service synchronously and returns any error
func (s *TinglyService) Start(ctx context.Context) error {
	go func() {
		err := s.serverManager.Start()
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (s *TinglyService) GetGinEngine() *gin.Engine {
	return s.serverManager.GetGinEngine()
}

// ServeHTTP implements the http.Handler interface
func (s *TinglyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
	s.serverManager.ServeHTTP(w, r)
}

// ServiceStartup is called when the service starts
func (s *TinglyService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.Start(ctx)

	// Store the application instance for later use
	s.app = application.Get()

	// Register an event handler that can be triggered from the frontend
	s.app.Event.On("gin-api-event", func(event *application.CustomEvent) {
		// Log the event data
		s.app.Logger.Info("Received event from frontend", "data", event.Data)

		// Emit an event back to the frontend
		s.app.Event.Emit("gin-api-response",
			map[string]interface{}{
				"message": "Response from Gin API Service",
				"time":    time.Now().Format(time.RFC3339),
			},
		)
	})

	return nil
}

// ServiceShutdown is called when the service shuts down
func (s *TinglyService) ServiceShutdown(ctx context.Context) error {
	// Clean up resources if needed
	return nil
}

// ============
// Configuration Accessors
// ============

func (s *TinglyService) GetUserAuthToken() string {
	logrus.Debugf("Getting auth token %s\n", s.appManager.GetUserToken())
	return s.appManager.GetUserToken()
}

func (s *TinglyService) GetPort() int {
	logrus.Debugf("Getting port %d\n", s.appManager.GetServerPort())
	return s.appManager.GetServerPort()
}

// ChoosePath opens a native file dialog and returns a selected file or directory path.
func (s *TinglyService) ChoosePath() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("application is not ready")
	}

	return s.app.Dialog.OpenFile().
		SetTitle("Choose File or Directory").
		CanChooseFiles(true).
		CanChooseDirectories(true).
		ShowHiddenFiles(true).
		PromptForSingleSelection()
}

// ============
// Provider Management (exposed to GUI)
// ============

// ListProviders returns all configured providers
func (s *TinglyService) ListProviders() []*typ.Provider {
	return s.appManager.ListProviders()
}

// AddProvider adds a new AI provider
func (s *TinglyService) AddProvider(name, apiBase, token, apiStyle string) (string, error) {
	return s.appManager.AddProvider(name, apiBase, token, protocol.APIStyle(apiStyle))
}

// DeleteProvider removes an AI provider by name
func (s *TinglyService) DeleteProvider(name string) error {
	return s.appManager.DeleteProvider(name)
}

// GetProvider returns a provider by name
func (s *TinglyService) GetProvider(name string) (*typ.Provider, error) {
	return s.appManager.GetProvider(name)
}

// ============
// Rule Management (exposed to GUI)
// ============

// ListRules returns all configured rules
func (s *TinglyService) ListRules() []typ.Rule {
	return s.appManager.ListRules()
}

// ImportRule imports providers from JSONL/base64 export data. Despite the
// name (kept for call-site compatibility), only providers are imported —
// dataio export/import no longer carries rule data.
func (s *TinglyService) ImportRule(data string) (*command.ImportResult, error) {
	return s.appManager.ImportRule(data, exportpkg.FormatAuto, command.ImportOptions{
		OnProviderConflict: "use",
		Quiet:              true,
	})
}
