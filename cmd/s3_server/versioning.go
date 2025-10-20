package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iProDev/s3_server/miniobject"
)

// VersionManager handles object versioning
type VersionManager struct {
	backend     miniobject.Backend
	versionsDir string
	mu          sync.RWMutex
	enabled     map[string]bool // bucket -> enabled
}

type Version struct {
	VersionID    string    `json:"version_id"`
	IsLatest     bool      `json:"is_latest"`
	LastModified time.Time `json:"last_modified"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	DeleteMarker bool      `json:"delete_marker"`
}

type VersionList struct {
	Versions []Version `json:"versions"`
}

func NewVersionManager(versionsDir string, backend miniobject.Backend) *VersionManager {
	return &VersionManager{
		backend:     backend,
		versionsDir: versionsDir,
		enabled:     make(map[string]bool),
	}
}

// EnableVersioning enables versioning for a bucket
func (vm *VersionManager) EnableVersioning(bucket string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.enabled[bucket] = true
	
	// Create versions directory
	versionPath := filepath.Join(vm.versionsDir, bucket)
	return os.MkdirAll(versionPath, 0755)
}

// IsVersioningEnabled checks if versioning is enabled for bucket
func (vm *VersionManager) IsVersioningEnabled(bucket string) bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.enabled[bucket]
}

// PutVersion creates a new version of an object
func (vm *VersionManager) PutVersion(ctx context.Context, bucket, key string, etag string, size int64) (string, error) {
	if !vm.IsVersioningEnabled(bucket) {
		return "", nil // Versioning not enabled
	}

	versionID := generateVersionID()
	
	// Load existing versions
	versions, err := vm.loadVersions(bucket, key)
	if err != nil {
		versions = &VersionList{Versions: []Version{}}
	}

	// Mark all as not latest
	for i := range versions.Versions {
		versions.Versions[i].IsLatest = false
	}

	// Add new version
	newVersion := Version{
		VersionID:    versionID,
		IsLatest:     true,
		LastModified: time.Now(),
		Size:         size,
		ETag:         etag,
		DeleteMarker: false,
	}
	versions.Versions = append(versions.Versions, newVersion)

	// Save versions
	if err := vm.saveVersions(bucket, key, versions); err != nil {
		return "", err
	}

	return versionID, nil
}

// DeleteVersion creates a delete marker
func (vm *VersionManager) DeleteVersion(ctx context.Context, bucket, key string) (string, error) {
	if !vm.IsVersioningEnabled(bucket) {
		return "", nil // Versioning not enabled, regular delete
	}

	versionID := generateVersionID()
	
	versions, err := vm.loadVersions(bucket, key)
	if err != nil {
		versions = &VersionList{Versions: []Version{}}
	}

	// Mark all as not latest
	for i := range versions.Versions {
		versions.Versions[i].IsLatest = false
	}

	// Add delete marker
	deleteMarker := Version{
		VersionID:    versionID,
		IsLatest:     true,
		LastModified: time.Now(),
		DeleteMarker: true,
	}
	versions.Versions = append(versions.Versions, deleteMarker)

	if err := vm.saveVersions(bucket, key, versions); err != nil {
		return "", err
	}

	return versionID, nil
}

// ListVersions lists all versions of an object
func (vm *VersionManager) ListVersions(bucket, key string) ([]Version, error) {
	versions, err := vm.loadVersions(bucket, key)
	if err != nil {
		return nil, err
	}

	// Sort by last modified (newest first)
	sort.Slice(versions.Versions, func(i, j int) bool {
		return versions.Versions[i].LastModified.After(versions.Versions[j].LastModified)
	})

	return versions.Versions, nil
}

// GetVersion retrieves a specific version
func (vm *VersionManager) GetVersion(bucket, key, versionID string) (*Version, error) {
	versions, err := vm.loadVersions(bucket, key)
	if err != nil {
		return nil, err
	}

	for _, v := range versions.Versions {
		if v.VersionID == versionID {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("version not found")
}

// loadVersions loads version metadata from disk
func (vm *VersionManager) loadVersions(bucket, key string) (*VersionList, error) {
	versionFile := vm.versionPath(bucket, key)
	
	data, err := os.ReadFile(versionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &VersionList{Versions: []Version{}}, nil
		}
		return nil, err
	}

	var versions VersionList
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, err
	}

	return &versions, nil
}

// saveVersions saves version metadata to disk
func (vm *VersionManager) saveVersions(bucket, key string, versions *VersionList) error {
	versionFile := vm.versionPath(bucket, key)
	
	// Create directory if needed
	dir := filepath.Dir(versionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(versionFile, data, 0644)
}

func (vm *VersionManager) versionPath(bucket, key string) string {
	// Use hash to avoid deep directory structures
	safeKey := strings.ReplaceAll(key, "/", "_")
	return filepath.Join(vm.versionsDir, bucket, safeKey+".versions.json")
}

func generateVersionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Gateway handlers for versioning

func (s *GatewayServer) handlePutBucketVersioning(w http.ResponseWriter, r *http.Request, bucket string) {
	// Enable versioning for bucket
	if s.versionManager == nil {
		s.versionManager = NewVersionManager(filepath.Join(s.cfg.TmpDir, "versions"), s.backend)
	}

	if err := s.versionManager.EnableVersioning(bucket); err != nil {
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.Info("versioning enabled", "bucket", bucket)
}

func (s *GatewayServer) handleListObjectVersions(w http.ResponseWriter, r *http.Request, bucket string) {
	if s.versionManager == nil {
		s.writeS3Error(w, r, "InvalidRequest", "Versioning not configured", http.StatusBadRequest)
		return
	}

	key := r.URL.Query().Get("prefix")
	
	versions, err := s.versionManager.ListVersions(bucket, key)
	if err != nil {
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Return XML response (simplified)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListVersionsResult>
  <Name>%s</Name>
  <Prefix>%s</Prefix>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>`, bucket, key)

	for _, v := range versions {
		if v.DeleteMarker {
			fmt.Fprintf(w, `
  <DeleteMarker>
    <Key>%s</Key>
    <VersionId>%s</VersionId>
    <IsLatest>%t</IsLatest>
    <LastModified>%s</LastModified>
  </DeleteMarker>`, key, v.VersionID, v.IsLatest, v.LastModified.Format(time.RFC3339))
		} else {
			fmt.Fprintf(w, `
  <Version>
    <Key>%s</Key>
    <VersionId>%s</VersionId>
    <IsLatest>%t</IsLatest>
    <LastModified>%s</LastModified>
    <ETag>%s</ETag>
    <Size>%d</Size>
  </Version>`, key, v.VersionID, v.IsLatest, v.LastModified.Format(time.RFC3339), v.ETag, v.Size)
		}
	}

	fmt.Fprintf(w, "\n</ListVersionsResult>")
}
