package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// Test Enhanced Versioning

func TestEnhancedVersioning(t *testing.T) {
	logger := NewLogger("error")
	backend := &mockBackendSimple{objects: make(map[string][]byte)}
	vm := NewEnhancedVersionManager("./test_versions", backend, logger)
	defer cleanupTestDir("./test_versions")
	
	bucket := "test-bucket"
	key := "test-object"
	
	// Test 1: Enable versioning
	t.Run("EnableVersioning", func(t *testing.T) {
		err := vm.EnableVersioning(bucket, 5, 30)
		if err != nil {
			t.Fatalf("Failed to enable versioning: %v", err)
		}
		
		config := vm.GetVersioningStatus(bucket)
		if config == nil || !config.Enabled {
			t.Error("Versioning should be enabled")
		}
		
		if config.MaxVersions != 5 {
			t.Errorf("Expected max_versions=5, got %d", config.MaxVersions)
		}
	})
	
	// Test 2: Add versions
	t.Run("AddVersion", func(t *testing.T) {
		version := &VersionInfo{
			VersionID:    "v1",
			Key:          key,
			Size:         100,
			ETag:         "etag1",
			LastModified: time.Now(),
			IsLatest:     true,
			ContentType:  "text/plain",
		}
		
		err := vm.AddVersion(bucket, key, version)
		if err != nil {
			t.Fatalf("Failed to add version: %v", err)
		}
		
		versions, err := vm.ListVersions(bucket, key)
		if err != nil {
			t.Fatalf("Failed to list versions: %v", err)
		}
		
		if len(versions) != 1 {
			t.Errorf("Expected 1 version, got %d", len(versions))
		}
		
		if !versions[0].IsLatest {
			t.Error("First version should be latest")
		}
	})
	
	// Test 3: Get specific version
	t.Run("GetVersion", func(t *testing.T) {
		version, err := vm.GetVersion(bucket, key, "v1")
		if err != nil {
			t.Fatalf("Failed to get version: %v", err)
		}
		
		if version.VersionID != "v1" {
			t.Errorf("Expected version v1, got %s", version.VersionID)
		}
	})
	
	// Test 4: Get latest version
	t.Run("GetLatestVersion", func(t *testing.T) {
		latest, err := vm.GetLatestVersion(bucket, key)
		if err != nil {
			t.Fatalf("Failed to get latest version: %v", err)
		}
		
		if !latest.IsLatest {
			t.Error("Should return latest version")
		}
	})
	
	// Test 5: Max versions limit
	t.Run("MaxVersionsLimit", func(t *testing.T) {
		// Add more versions than limit
		for i := 2; i <= 10; i++ {
			version := &VersionInfo{
				VersionID:    versionID(i),
				Key:          key,
				Size:         100,
				ETag:         etagForVersion(i),
				LastModified: time.Now(),
			}
			vm.AddVersion(bucket, key, version)
		}
		
		versions, _ := vm.ListVersions(bucket, key)
		if len(versions) > 5 {
			t.Errorf("Expected max 5 versions, got %d", len(versions))
		}
	})
	
	// Test 6: Version statistics
	t.Run("VersionStatistics", func(t *testing.T) {
		stats, err := vm.GetVersionStatistics(bucket)
		if err != nil {
			t.Fatalf("Failed to get statistics: %v", err)
		}
		
		if stats.TotalObjects == 0 {
			t.Error("Should have at least one object")
		}
	})
}

// Test Batch Operations

func TestBatchOperations(t *testing.T) {
	logger := NewLogger("error")
	backend := &mockBackendSimple{objects: make(map[string][]byte)}
	bp := NewBatchProcessor(backend, nil, logger)
	
	ctx := context.Background()
	
	// Setup test data
	backend.objects["bucket1/file1"] = []byte("content1")
	backend.objects["bucket1/file2"] = []byte("content2")
	backend.objects["bucket1/file3"] = []byte("content3")
	
	// Test 1: Batch delete
	t.Run("BatchDelete", func(t *testing.T) {
		request := &BatchRequest{
			Operation: BatchDelete,
			Operations: []BatchItem{
				{Bucket: "bucket1", Key: "file1"},
				{Bucket: "bucket1", Key: "file2"},
			},
			Options: BatchOptions{
				Concurrency: 2,
			},
		}
		
		response, err := bp.Execute(ctx, request)
		if err != nil {
			t.Fatalf("Batch delete failed: %v", err)
		}
		
		if response.TotalItems != 2 {
			t.Errorf("Expected 2 items, got %d", response.TotalItems)
		}
		
		// Wait for completion
		time.Sleep(100 * time.Millisecond)
		
		job, _ := bp.GetJob(response.JobID)
		if job.Response.Successful != 2 {
			t.Errorf("Expected 2 successful, got %d", job.Response.Successful)
		}
	})
	
	// Test 2: Batch copy
	t.Run("BatchCopy", func(t *testing.T) {
		request := &BatchRequest{
			Operation: BatchCopy,
			Operations: []BatchItem{
				{
					Bucket:     "bucket1",
					Key:        "file3",
					DestBucket: "bucket2",
					DestKey:    "copied-file3",
				},
			},
			Options: BatchOptions{
				Concurrency: 1,
			},
		}
		
		response, err := bp.Execute(ctx, request)
		if err != nil {
			t.Fatalf("Batch copy failed: %v", err)
		}
		
		time.Sleep(100 * time.Millisecond)
		
		// Check if copied
		if _, ok := backend.objects["bucket2/copied-file3"]; !ok {
			t.Error("File should be copied to destination")
		}
	})
	
	// Test 3: Dry run
	t.Run("DryRun", func(t *testing.T) {
		request := &BatchRequest{
			Operation: BatchDelete,
			Operations: []BatchItem{
				{Bucket: "bucket1", Key: "file3"},
			},
			Options: BatchOptions{
				DryRun: true,
			},
		}
		
		response, err := bp.Execute(ctx, request)
		if err != nil {
			t.Fatalf("Dry run failed: %v", err)
		}
		
		// File should still exist
		if _, ok := backend.objects["bucket1/file3"]; !ok {
			t.Error("File should not be deleted in dry run")
		}
	})
	
	// Test 4: Error handling
	t.Run("ErrorHandling", func(t *testing.T) {
		request := &BatchRequest{
			Operation: BatchDelete,
			Operations: []BatchItem{
				{Bucket: "bucket1", Key: "nonexistent"},
			},
			Options: BatchOptions{
				ContinueOnError: true,
			},
		}
		
		response, err := bp.Execute(ctx, request)
		if err != nil {
			t.Fatalf("Batch operation failed: %v", err)
		}
		
		time.Sleep(100 * time.Millisecond)
		
		job, _ := bp.GetJob(response.JobID)
		if job.Response.Failed == 0 {
			t.Error("Expected failed operations for nonexistent files")
		}
	})
	
	// Test 5: List jobs
	t.Run("ListJobs", func(t *testing.T) {
		jobs := bp.ListJobs()
		if len(jobs) == 0 {
			t.Error("Should have at least one job")
		}
	})
}

// Test Concurrent Upload

func TestConcurrentUpload(t *testing.T) {
	logger := NewLogger("error")
	backend := &mockBackendSimple{objects: make(map[string][]byte)}
	cum := NewConcurrentUploadManager(backend, nil, logger)
	
	bucket := "test-bucket"
	key := "large-file"
	
	// Test 1: Initiate concurrent upload
	t.Run("InitiateConcurrentUpload", func(t *testing.T) {
		totalSize := int64(50 * 1024 * 1024) // 50MB
		chunkSize := int64(5 * 1024 * 1024)  // 5MB
		
		upload, err := cum.InitiateConcurrentUpload(bucket, key, totalSize, chunkSize, 3)
		if err != nil {
			t.Fatalf("Failed to initiate upload: %v", err)
		}
		
		if upload.Concurrency != 3 {
			t.Errorf("Expected concurrency 3, got %d", upload.Concurrency)
		}
		
		expectedParts := 10 // 50MB / 5MB
		if len(upload.Parts) != expectedParts {
			t.Errorf("Expected %d parts, got %d", expectedParts, len(upload.Parts))
		}
	})
	
	// Test 2: Get progress
	t.Run("GetProgress", func(t *testing.T) {
		upload, _ := cum.InitiateConcurrentUpload(bucket, "file2", 10*1024*1024, 5*1024*1024, 2)
		
		progress, err := cum.GetProgress(upload.UploadID)
		if err != nil {
			t.Fatalf("Failed to get progress: %v", err)
		}
		
		if progress.TotalBytes != 10*1024*1024 {
			t.Errorf("Expected total bytes 10MB, got %d", progress.TotalBytes)
		}
		
		if progress.PartsTotal != 2 {
			t.Errorf("Expected 2 parts, got %d", progress.PartsTotal)
		}
	})
	
	// Test 3: Upload parts
	t.Run("UploadParts", func(t *testing.T) {
		data := make([]byte, 10*1024*1024) // 10MB
		for i := range data {
			data[i] = byte(i % 256)
		}
		reader := bytes.NewReader(data)
		
		upload, _ := cum.InitiateConcurrentUpload(bucket, "file3", int64(len(data)), 5*1024*1024, 2)
		
		err := cum.UploadParts(upload.UploadID, reader)
		if err != nil {
			t.Fatalf("Failed to upload parts: %v", err)
		}
		
		// Check all parts uploaded
		for _, part := range upload.Parts {
			if !part.Uploaded {
				t.Errorf("Part %d should be uploaded", part.PartNumber)
			}
		}
	})
	
	// Test 4: Abort upload
	t.Run("AbortUpload", func(t *testing.T) {
		upload, _ := cum.InitiateConcurrentUpload(bucket, "file4", 10*1024*1024, 5*1024*1024, 2)
		
		err := cum.AbortConcurrentUpload(upload.UploadID)
		if err != nil {
			t.Fatalf("Failed to abort upload: %v", err)
		}
		
		// Should not be able to get progress after abort
		_, err = cum.GetProgress(upload.UploadID)
		if err == nil {
			t.Error("Should not find upload after abort")
		}
	})
	
	// Test 5: Invalid parameters
	t.Run("InvalidParameters", func(t *testing.T) {
		// Chunk size too small
		_, err := cum.InitiateConcurrentUpload(bucket, key, 100*1024*1024, 1024, 5)
		if err != nil {
			// Should auto-correct to minimum
		}
		
		// Too many parts
		_, err = cum.InitiateConcurrentUpload(bucket, key, 1024*1024*1024*1024, 1024, 5)
		if err != ErrTooManyParts {
			t.Error("Should reject when too many parts")
		}
	})
}

// Helper functions and mocks

func versionID(i int) string {
	return string(rune('v')) + string(rune('0'+i))
}

func etagForVersion(i int) string {
	return string(rune('e')) + string(rune('0'+i))
}

func cleanupTestDir(dir string) {
	// os.RemoveAll(dir)
}

type mockBackendSimple struct {
	objects map[string][]byte
	mu      sync.RWMutex
}

func (m *mockBackendSimple) Put(ctx context.Context, bucket, key string, data io.Reader, contentType, contentMD5 string) (string, error) {
	bytes, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}
	
	m.mu.Lock()
	m.objects[bucket+"/"+key] = bytes
	m.mu.Unlock()
	
	return "etag-" + key, nil
}

func (m *mockBackendSimple) Get(ctx context.Context, bucket, key, rangeSpec string) (io.ReadCloser, string, string, int64, int, error) {
	m.mu.RLock()
	data, ok := m.objects[bucket+"/"+key]
	m.mu.RUnlock()
	
	if !ok {
		return nil, "", "", 0, 0, errors.New("NoSuchKey")
	}
	
	return io.NopCloser(bytes.NewReader(data)), "application/octet-stream", "etag-"+key, int64(len(data)), 200, nil
}

func (m *mockBackendSimple) Delete(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	delete(m.objects, bucket+"/"+key)
	m.mu.Unlock()
	return nil
}

func (m *mockBackendSimple) Head(ctx context.Context, bucket, key string) (string, string, int64, bool, error) {
	m.mu.RLock()
	data, ok := m.objects[bucket+"/"+key]
	m.mu.RUnlock()
	
	if !ok {
		return "", "", 0, false, nil
	}
	
	return "application/octet-stream", "etag-"+key, int64(len(data)), true, nil
}

func (m *mockBackendSimple) List(ctx context.Context, bucket, prefix, marker string, maxKeys int) ([]Object, error) {
	return []Object{}, nil
}
