/*
Copyright (c) Gatis Beikerts

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cleanup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services/storage"
)

func TestCleanupExpiredListsLocalStorage(t *testing.T) {
	tempDir := t.TempDir()

	stor, err := storage.NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create local storage: %v", err)
	}

	cfg := &config.Config{CleanupEnabled: true, CleanupHour: 0, CleanupMinute: 0}
	service := NewService(cfg, stor, zap.NewNop())

	expiredDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	futureDate := time.Now().AddDate(0, 0, 3).Format("2006-01-02")

	createFullList(t, tempDir, "token_status_list/LV/pid/expired/full_list.json", expiredDate)
	createFullList(t, tempDir, "identifier_list/LV/pid/expired/full_list.json", expiredDate)
	createFullList(t, tempDir, "token_status_list/LV/pid/active/full_list.json", futureDate)
	createFullList(t, tempDir, "identifier_list/LV/pid/active/full_list.json", futureDate)

	deleted, err := service.cleanupExpiredLists()
	if err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if deleted != 1 {
		t.Fatalf("expected 1 deleted list, got %d", deleted)
	}

	if exists(t, tempDir, "token_status_list/LV/pid/expired") {
		t.Fatalf("expired token directory still exists")
	}

	if exists(t, tempDir, "identifier_list/LV/pid/expired") {
		t.Fatalf("expired identifier directory still exists")
	}

	if !exists(t, tempDir, "token_status_list/LV/pid/active") {
		t.Fatalf("active token directory should remain")
	}

	if !exists(t, tempDir, "identifier_list/LV/pid/active") {
		t.Fatalf("active identifier directory should remain")
	}
}

func createFullList(t *testing.T, baseDir, relativePath, expiry string) {
	t.Helper()

	dir := filepath.Dir(relativePath)
	fullDir := filepath.Join(baseDir, dir)

	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatalf("failed to create directory %s: %v", fullDir, err)
	}

	statusList := models.StatusListData{
		TokenStatusList:   models.NewIssuerStatusList(1, 16, "sequential"),
		IdentifierList:    map[string]int{},
		Expires:           &expiry,
		StatusListURI:     "https://example.org/token",
		IdentifierListURI: "https://example.org/identifier",
		Country:           "LV",
		Doctype:           "pid",
	}

	payload, err := json.Marshal(statusList)
	if err != nil {
		t.Fatalf("failed to marshal status list: %v", err)
	}

	if err := os.WriteFile(filepath.Join(baseDir, relativePath), payload, 0o644); err != nil {
		t.Fatalf("failed to write full_list.json: %v", err)
	}
}

func exists(t *testing.T, baseDir, relativePath string) bool {
	t.Helper()

	path := filepath.Join(baseDir, relativePath)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
