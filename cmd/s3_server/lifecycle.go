package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iProDev/s3_server/miniobject"
)

// LifecycleRule defines an object lifecycle rule
type LifecycleRule struct {
	ID                   string        `json:"id"`
	Prefix               string        `json:"prefix"`
	Enabled              bool          `json:"enabled"`
	ExpirationDays       int           `json:"expiration_days"`        // Delete after N days
	TransitionDays       int           `json:"transition_days"`        // Transition to cheaper storage after N days
	AbortIncompleteMultipartDays int  `json:"abort_incomplete_multipart_days"` // Abort incomplete multipart uploads
	DeleteMarkerExpiration bool        `json:"delete_marker_expiration"` // Remove expired delete markers
}

// LifecycleConfig bucket lifecycle configuration
type LifecycleConfig struct {
	Bucket string           `json:"bucket"`
	Rules  []LifecycleRule  `json:"rules"`
}

// LifecycleManager manages object lifecycle policies
type LifecycleManager struct {
	configs    map[string]*LifecycleConfig
	configDir  string
	backend    miniobject.Backend
	mu         sync.RWMutex
	stopCh     chan struct{}
	logger     *Logger
}

// NewLifecycleManager creates a new lifecycle manager
func NewLifecycleManager(configDir string, backend miniobject.Backend, logger *Logger) *LifecycleManager {
	lm := &LifecycleManager{
		configs:   make(map[string]*LifecycleConfig),
		configDir: configDir,
		backend:   backend,
		stopCh:    make(chan struct{}),
		logger:    logger,
	}

	os.MkdirAll(configDir, 0755)
	lm.loadAll()
	
	return lm
}

// loadAll loads all lifecycle configurations
func (lm *LifecycleManager) loadAll() error {
	files, err := os.ReadDir(lm.configDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			if err := lm.loadConfig(filepath.Join(lm.configDir, file.Name())); err != nil {
				lm.logger.Error("failed to load lifecycle config", "file", file.Name(), "error", err)
			}
		}
	}

	return nil
}

// loadConfig loads a single lifecycle configuration
func (lm *LifecycleManager) loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config LifecycleConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	lm.mu.Lock()
	lm.configs[config.Bucket] = &config
	lm.mu.Unlock()

	return nil
}

// saveConfig saves a lifecycle configuration
func (lm *LifecycleManager) saveConfig(bucket string) error {
	lm.mu.RLock()
	config := lm.configs[bucket]
	lm.mu.RUnlock()

	if config == nil {
		return nil
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(lm.configDir, bucket+".json")
	return os.WriteFile(path, data, 0644)
}

// SetBucketLifecycle sets lifecycle configuration for a bucket
func (lm *LifecycleManager) SetBucketLifecycle(bucket string, rules []LifecycleRule) error {
	config := &LifecycleConfig{
		Bucket: bucket,
		Rules:  rules,
	}

	lm.mu.Lock()
	lm.configs[bucket] = config
	lm.mu.Unlock()

	return lm.saveConfig(bucket)
}

// GetBucketLifecycle gets lifecycle configuration for a bucket
func (lm *LifecycleManager) GetBucketLifecycle(bucket string) *LifecycleConfig {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.configs[bucket]
}

// DeleteBucketLifecycle removes lifecycle configuration for a bucket
func (lm *LifecycleManager) DeleteBucketLifecycle(bucket string) error {
	lm.mu.Lock()
	delete(lm.configs, bucket)
	lm.mu.Unlock()

	path := filepath.Join(lm.configDir, bucket+".json")
	return os.Remove(path)
}

// Start begins the lifecycle processing loop
func (lm *LifecycleManager) Start() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run immediately on start
	lm.processAll()

	for {
		select {
		case <-ticker.C:
			lm.processAll()
		case <-lm.stopCh:
			return
		}
	}
}

// Stop stops the lifecycle manager
func (lm *LifecycleManager) Stop() {
	close(lm.stopCh)
}

// processAll processes lifecycle rules for all buckets
func (lm *LifecycleManager) processAll() {
	lm.mu.RLock()
	configs := make([]*LifecycleConfig, 0, len(lm.configs))
	for _, config := range lm.configs {
		configs = append(configs, config)
	}
	lm.mu.RUnlock()

	for _, config := range configs {
		lm.processBucket(config)
	}
}

// processBucket processes lifecycle rules for a single bucket
func (lm *LifecycleManager) processBucket(config *LifecycleConfig) {
	lm.logger.Info("processing lifecycle rules", "bucket", config.Bucket, "rules", len(config.Rules))

	objects, err := lm.backend.List(context.Background(), config.Bucket, "", "", 1000)
	if err != nil {
		lm.logger.Error("failed to list objects for lifecycle", "bucket", config.Bucket, "error", err)
		return
	}

	now := time.Now()
	
	for _, obj := range objects {
		for _, rule := range config.Rules {
			if !rule.Enabled {
				continue
			}

			// Check if object matches rule prefix
			if rule.Prefix != "" && !strings.HasPrefix(obj.Key, rule.Prefix) {
				continue
			}

			// Check expiration
			if rule.ExpirationDays > 0 {
				// Parse LastModified string to time
				lastModified, err := time.Parse(time.RFC3339, obj.LastModified)
				if err != nil {
					continue // Skip if can't parse
				}
				age := now.Sub(lastModified)
				if age >= time.Duration(rule.ExpirationDays)*24*time.Hour {
					lm.logger.Info("expiring object", "bucket", config.Bucket, "key", obj.Key, "age_days", int(age.Hours()/24))
					if err := lm.backend.Delete(context.Background(), config.Bucket, obj.Key); err != nil {
						lm.logger.Error("failed to expire object", "bucket", config.Bucket, "key", obj.Key, "error", err)
					}
				}
			}
		}
	}
}

// ShouldExpire checks if an object should be expired based on lifecycle rules
func (lm *LifecycleManager) ShouldExpire(bucket, key string, lastModified time.Time) bool {
	lm.mu.RLock()
	config := lm.configs[bucket]
	lm.mu.RUnlock()

	if config == nil {
		return false
	}

	now := time.Now()
	
	for _, rule := range config.Rules {
		if !rule.Enabled {
			continue
		}

		if rule.Prefix != "" && !strings.HasPrefix(key, rule.Prefix) {
			continue
		}

		if rule.ExpirationDays > 0 {
			age := now.Sub(lastModified)
			if age >= time.Duration(rule.ExpirationDays)*24*time.Hour {
				return true
			}
		}
	}

	return false
}
