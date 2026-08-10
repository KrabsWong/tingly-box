package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupFileNaming(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.json")
	if err := os.WriteFile(testFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Generate backup path
	backupPath := generateBackupPath(testFile)

	// Verify it contains timestamp
	expectedSuffix := ".json.bak-" + time.Now().Format("20060102-")
	if len(backupPath) < len(expectedSuffix) {
		t.Errorf("Backup path too short: %s", backupPath)
	}

	// Verify backup doesn't exist yet
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Errorf("Backup should not exist yet: %s", backupPath)
	}
}

func TestEnsureDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	nestedPath := filepath.Join(tempDir, "a", "b", "c", "file.json")

	// Ensure directory exists
	if err := ensureDir(nestedPath); err != nil {
		t.Fatalf("ensureDir failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(nestedPath)); os.IsNotExist(err) {
		t.Errorf("Expected directory to be created")
	}
}

// writeFakeBackup synthesizes a backup file at <dir>/backup/ with a controlled
// timestamp embedded in its filename. Used by rotation tests to avoid the
// 1-second granularity of the real timestamping path.
func writeFakeBackup(t *testing.T, originalPath string, ts time.Time, content string) string {
	t.Helper()
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(originalPath)
	backupDir := filepath.Join(dir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	name := fmt.Sprintf("%s.bak-%s%s", base, ts.Format(backupTimestampLayout), ext)
	path := filepath.Join(backupDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fake backup: %v", err)
	}
	return path
}

func TestBackupRotation_KeepsLatestN(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	if err := os.WriteFile(targetPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Synthesize 6 backups spaced 10 seconds apart.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	var paths []string
	for i := 0; i < 6; i++ {
		paths = append(paths, writeFakeBackup(t, targetPath, base.Add(time.Duration(i)*10*time.Second), fmt.Sprintf("v%d", i)))
	}

	if err := rotateBackups(targetPath, 3); err != nil {
		t.Fatalf("rotateBackups: %v", err)
	}

	backups, err := ListBackups(targetPath)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("Expected 3 backups after rotation, got %d", len(backups))
	}
	// The 3 newest must be the last 3 we created.
	expected := map[string]bool{paths[3]: true, paths[4]: true, paths[5]: true}
	for _, b := range backups {
		if !expected[b.Path] {
			t.Errorf("Unexpected backup retained: %s", b.Path)
		}
	}
	// Order is newest-first.
	for i := 1; i < len(backups); i++ {
		if !backups[i-1].Timestamp.After(backups[i].Timestamp) {
			t.Errorf("Backups not in descending order")
		}
	}
}

func TestBackupRotation_DoesNotTouchOtherBaseFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	settings := filepath.Join(tempDir, "settings.json")
	other := filepath.Join(tempDir, "other.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := os.WriteFile(other, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed other: %v", err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	otherBackup := writeFakeBackup(t, other, base, "x")
	for i := 0; i < 5; i++ {
		writeFakeBackup(t, settings, base.Add(time.Duration(i)*10*time.Second), fmt.Sprintf("s%d", i))
	}

	if err := rotateBackups(settings, defaultBackupRetention); err != nil {
		t.Fatalf("rotateBackups: %v", err)
	}

	if _, err := os.Stat(otherBackup); err != nil {
		t.Errorf("Rotation of settings.json removed unrelated backup %s: %v", otherBackup, err)
	}

	settingsBackups, err := ListBackups(settings)
	if err != nil {
		t.Fatalf("ListBackups(settings): %v", err)
	}
	if len(settingsBackups) != defaultBackupRetention {
		t.Errorf("Expected %d settings backups, got %d", defaultBackupRetention, len(settingsBackups))
	}
}

func TestRestoreLatestBackup_RoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	original := []byte(`{"version":"original"}`)
	mutated := []byte(`{"version":"mutated"}`)

	// Seed a synthetic backup that holds the "original" content.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	backupPath := writeFakeBackup(t, targetPath, base, string(original))

	// Live file holds the mutated state we want to undo.
	if err := os.WriteFile(targetPath, mutated, 0644); err != nil {
		t.Fatalf("seed live: %v", err)
	}

	result, err := RestoreLatestBackup(targetPath)
	if err != nil {
		t.Fatalf("RestoreLatestBackup: %v", err)
	}
	if !result.Success {
		t.Fatalf("Restore not successful: %s", result.Message)
	}
	if result.RestoredFrom != backupPath {
		t.Errorf("Restored from %q, want %q", result.RestoredFrom, backupPath)
	}
	if result.PreRestoreBackup == "" {
		t.Errorf("Expected a pre-restore backup to be created")
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("Restored content mismatch: got %s, want %s", got, original)
	}

	preData, err := os.ReadFile(result.PreRestoreBackup)
	if err != nil {
		t.Fatalf("read pre-restore backup: %v", err)
	}
	if string(preData) != string(mutated) {
		t.Errorf("Pre-restore backup mismatch: got %s, want %s", preData, mutated)
	}
}

func TestRestoreLatestBackup_NoBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	if err := os.WriteFile(targetPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := RestoreLatestBackup(targetPath)
	if err == nil {
		t.Fatalf("Expected error when no backup exists, got nil")
	}
	if result == nil || result.Success {
		t.Errorf("Expected unsuccessful result, got %+v", result)
	}
}

func TestListBackups_MissingDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backups, err := ListBackups(filepath.Join(tempDir, "settings.json"))
	if err != nil {
		t.Fatalf("ListBackups should not error on missing backup dir: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("Expected no backups, got %d", len(backups))
	}
}
