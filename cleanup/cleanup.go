package cleanup

import (
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services/storage"
)

const (
	fullListFile = "full_list.json"
)

// Service removes expired status list artifacts from the configured storage backend.
type Service struct {
	cfg     *config.Config
	storage storage.Storage
	logger  *zap.Logger
}

// NewService creates a cleanup service instance.
func NewService(cfg *config.Config, stor storage.Storage, logger *zap.Logger) *Service {
	return &Service{cfg: cfg, storage: stor, logger: logger}
}

// RunOnce executes the cleanup workflow immediately.
func (s *Service) RunOnce() {
	hostname, _ := os.Hostname()
	s.logger.Debug("cleanup starting", zap.String("pod", hostname))

	count, err := s.cleanupExpiredLists()
	if err != nil {
		s.logger.Error("status list cleanup finished with errors", zap.String("pod", hostname), zap.Int("removed", count), zap.Error(err))
		return
	}

	s.logger.Info("status list cleanup completed", zap.String("pod", hostname), zap.Int("removed", count))
}

// Start launches the background scheduler that performs cleanup daily at the configured time.
func (s *Service) Start() {
	go s.schedule()
}

// StartCleanupWorker creates and starts the cleanup worker if the feature is enabled.
func StartCleanupWorker(cfg *config.Config, stor storage.Storage, logger *zap.Logger) {
	if !cfg.CleanupEnabled {
		logger.Info("status list cleanup disabled via configuration")
		return
	}

	service := NewService(cfg, stor, logger)
	service.RunOnce()
	service.Start()
}

func (s *Service) schedule() {
	for {
		now := time.Now()
		next := nextRun(now, s.cfg.CleanupHour, s.cfg.CleanupMinute)
		delay := time.Until(next)

		s.logger.Debug("next cleanup scheduled", zap.Duration("in", delay))

		time.Sleep(delay)
		s.RunOnce()
	}
}

func (s *Service) cleanupExpiredLists() (int, error) {
	paths, err := s.storage.List("token_status_list/")
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	var errs []error
	deleted := 0

	for _, filePath := range paths {
		if !isTokenFullList(filePath) {
			continue
		}

		statusList, loadErr := s.loadStatusList(filePath)
		if loadErr != nil {
			if stdErrors.Is(loadErr, errors.ErrNotFound) {
				s.logger.Debug("file already removed (concurrent cleanup?), skipping", zap.String("file", filePath))
				continue
			}
			errs = append(errs, loadErr)
			continue
		}

		expired, evalErr := hasExpired(statusList, now)
		if evalErr != nil {
			errs = append(errs, evalErr)
			continue
		}

		if !expired {
			continue
		}

		dirPath := path.Dir(filePath)
		s.logger.Info("removing expired status list", zap.String("dir", dirPath), zap.String("expiry", valueOrUnknown(statusList.Expires)))

		if err := s.storage.DeleteTree(dirPath); err != nil {
			errs = append(errs, fmt.Errorf("delete token list %s: %w", dirPath, err))
			continue
		}

		if err := s.deleteIdentifierDir(dirPath); err != nil {
			errs = append(errs, err)
		}

		deleted++
	}

	return deleted, stdErrors.Join(errs...)
}

func (s *Service) loadStatusList(path string) (*models.StatusListData, error) {
	data, err := s.storage.Read(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var statusList models.StatusListData
	if err := json.Unmarshal(data, &statusList); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}

	return &statusList, nil
}

func isTokenFullList(path string) bool {
	return strings.Contains(path, "token_status_list/") && strings.HasSuffix(path, "/"+fullListFile)
}

func hasExpired(statusList *models.StatusListData, now time.Time) (bool, error) {
	if statusList.Expires == nil {
		return false, nil
	}

	expiresAt, err := time.Parse("2006-01-02", *statusList.Expires)
	if err != nil {
		return false, fmt.Errorf("parse expiry %q: %w", *statusList.Expires, err)
	}

	return expiresAt.Before(now), nil
}

func (s *Service) deleteIdentifierDir(tokenDir string) error {
	identifierDir := strings.Replace(tokenDir, "token_status_list", "identifier_list", 1)
	if identifierDir == tokenDir {
		return nil
	}

	if err := s.storage.DeleteTree(identifierDir); err != nil {
		return fmt.Errorf("delete identifier list %s: %w", identifierDir, err)
	}

	return nil
}

func valueOrUnknown(value *string) string {
	if value == nil {
		return "unknown"
	}
	return *value
}

func nextRun(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
