package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyOpenCodeConfig_NewFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	payload := map[string]interface{}{
		"provider": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name": "tingly-box",
				"npm":  "@ai-sdk/anthropic",
				"options": map[string]interface{}{
					"baseURL": "http://localhost:12580/tingly/opencode",
				},
				"models": map[string]interface{}{
					"test-model": map[string]interface{}{
						"name": "test-model",
					},
				},
			},
		},
	}

	result, err := ApplyOpenCodeConfig(payload)
	if err != nil {
		t.Fatalf("ApplyOpenCodeConfig failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Created {
		t.Errorf("Expected Created to be true")
	}
}

func TestApplyOpenCodeConfig_ExistingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create existing config directory and file
	configDir := filepath.Join(tempDir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	existingConfig := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]interface{}{
			"other-provider": map[string]interface{}{
				"name": "other-provider",
			},
		},
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	targetPath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	payload := map[string]interface{}{
		"provider": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name": "tingly-box",
				"npm":  "@ai-sdk/anthropic",
			},
		},
	}

	result, err := ApplyOpenCodeConfig(payload)
	if err != nil {
		t.Fatalf("ApplyOpenCodeConfig failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Updated {
		t.Errorf("Expected Updated to be true")
	}

	// Verify other provider is preserved and tingly-box is added
	data, _ := os.ReadFile(targetPath)
	var config map[string]interface{}
	json.Unmarshal(data, &config)

	providers, ok := config["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected provider section")
	}

	if providers["other-provider"] == nil {
		t.Errorf("Expected other-provider to be preserved")
	}

	if providers["tingly-box"] == nil {
		t.Errorf("Expected tingly-box to be added")
	}
}
