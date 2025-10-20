package main

import (
	"os"
	"testing"
	"time"
)

func TestAuthManager(t *testing.T) {
	// Create temp auth file
	tmpFile := "test_auth.json"
	defer os.Remove(tmpFile)

	// Initialize auth manager
	am, err := NewAuthManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	// Test 1: Create credential
	t.Run("CreateCredential", func(t *testing.T) {
		cred, err := am.CreateCredential("test-app", []string{"read", "write"})
		if err != nil {
			t.Fatalf("Failed to create credential: %v", err)
		}

		if cred.Name != "test-app" {
			t.Errorf("Expected name 'test-app', got '%s'", cred.Name)
		}

		if len(cred.AccessKey) == 0 {
			t.Error("Access key should not be empty")
		}

		if len(cred.SecretKey) == 0 {
			t.Error("Secret key should not be empty")
		}

		if !cred.Active {
			t.Error("Credential should be active")
		}

		if len(cred.Permissions) != 2 {
			t.Errorf("Expected 2 permissions, got %d", len(cred.Permissions))
		}
	})

	// Test 2: Get credential
	t.Run("GetCredential", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-2", []string{"read"})
		
		retrieved := am.GetCredential(cred.AccessKey)
		if retrieved == nil {
			t.Fatal("Failed to retrieve credential")
		}

		if retrieved.AccessKey != cred.AccessKey {
			t.Errorf("Expected access key %s, got %s", cred.AccessKey, retrieved.AccessKey)
		}
	})

	// Test 3: Validate signature
	t.Run("ValidateSignature", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-3", []string{"read", "write"})
		
		stringToSign := "GET\n/mybucket/mykey\n2025-01-15T10:00:00Z"
		signature := computeSignature(cred.SecretKey, stringToSign)

		err := am.Validate(cred.AccessKey, signature, stringToSign)
		if err != nil {
			t.Errorf("Signature validation failed: %v", err)
		}
	})

	// Test 4: Invalid signature
	t.Run("InvalidSignature", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-4", []string{"read"})
		
		stringToSign := "GET\n/mybucket/mykey\n2025-01-15T10:00:00Z"
		wrongSignature := "invalid-signature"

		err := am.Validate(cred.AccessKey, wrongSignature, stringToSign)
		if err == nil {
			t.Error("Expected validation to fail with invalid signature")
		}
	})

	// Test 5: Revoke credential
	t.Run("RevokeCredential", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-5", []string{"read"})
		
		err := am.RevokeCredential(cred.AccessKey)
		if err != nil {
			t.Fatalf("Failed to revoke credential: %v", err)
		}

		retrieved := am.GetCredential(cred.AccessKey)
		if retrieved.Active {
			t.Error("Credential should be inactive after revocation")
		}
	})

	// Test 6: List credentials
	t.Run("ListCredentials", func(t *testing.T) {
		am.CreateCredential("app-1", []string{"read"})
		am.CreateCredential("app-2", []string{"write"})
		
		creds := am.ListCredentials()
		if len(creds) < 2 {
			t.Errorf("Expected at least 2 credentials, got %d", len(creds))
		}
	})

	// Test 7: Has permission
	t.Run("HasPermission", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-6", []string{"read", "write"})
		
		if !am.HasPermission(cred.AccessKey, "read") {
			t.Error("Should have read permission")
		}

		if !am.HasPermission(cred.AccessKey, "write") {
			t.Error("Should have write permission")
		}

		if am.HasPermission(cred.AccessKey, "delete") {
			t.Error("Should not have delete permission")
		}
	})

	// Test 8: Wildcard permission
	t.Run("WildcardPermission", func(t *testing.T) {
		cred, _ := am.CreateCredential("test-app-7", []string{"*"})
		
		if !am.HasPermission(cred.AccessKey, "read") {
			t.Error("Wildcard should grant read permission")
		}

		if !am.HasPermission(cred.AccessKey, "write") {
			t.Error("Wildcard should grant write permission")
		}

		if !am.HasPermission(cred.AccessKey, "delete") {
			t.Error("Wildcard should grant delete permission")
		}
	})
}

func TestPresignedURLGenerator(t *testing.T) {
	// Create temp auth file
	tmpFile := "test_presigned_auth.json"
	defer os.Remove(tmpFile)

	am, _ := NewAuthManager(tmpFile)
	cred, _ := am.CreateCredential("test-app", []string{"read", "write"})

	urlGen := NewPresignedURLGenerator(am, "http://localhost:9000")

	// Test 1: Generate download URL
	t.Run("GenerateDownloadURL", func(t *testing.T) {
		url, err := urlGen.GenerateDownloadURL(
			"mybucket",
			"myfile.txt",
			cred.AccessKey,
			time.Hour,
		)

		if err != nil {
			t.Fatalf("Failed to generate URL: %v", err)
		}

		if len(url) == 0 {
			t.Error("Generated URL should not be empty")
		}

		if !contains(url, "AWSAccessKeyId=") {
			t.Error("URL should contain AWSAccessKeyId parameter")
		}

		if !contains(url, "Expires=") {
			t.Error("URL should contain Expires parameter")
		}

		if !contains(url, "Signature=") {
			t.Error("URL should contain Signature parameter")
		}
	})

	// Test 2: Generate upload URL
	t.Run("GenerateUploadURL", func(t *testing.T) {
		url, err := urlGen.GenerateUploadURL(
			"mybucket",
			"newfile.txt",
			cred.AccessKey,
			30*time.Minute,
		)

		if err != nil {
			t.Fatalf("Failed to generate upload URL: %v", err)
		}

		if len(url) == 0 {
			t.Error("Generated URL should not be empty")
		}
	})

	// Test 3: Generate delete URL
	t.Run("GenerateDeleteURL", func(t *testing.T) {
		url, err := urlGen.GenerateDeleteURL(
			"mybucket",
			"oldfile.txt",
			cred.AccessKey,
			15*time.Minute,
		)

		if err != nil {
			t.Fatalf("Failed to generate delete URL: %v", err)
		}

		if len(url) == 0 {
			t.Error("Generated URL should not be empty")
		}
	})

	// Test 4: Invalid access key
	t.Run("InvalidAccessKey", func(t *testing.T) {
		_, err := urlGen.GenerateDownloadURL(
			"mybucket",
			"file.txt",
			"invalid-key",
			time.Hour,
		)

		if err == nil {
			t.Error("Expected error with invalid access key")
		}
	})
}

func TestLifecycleManager(t *testing.T) {
	// Create temp directories
	tmpDir := "test_lifecycle"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	// Mock logger
	logger := NewLogger("error")

	// Mock backend
	backend := &mockBackend{
		objects: make(map[string]*mockObject),
	}

	lm := NewLifecycleManager(tmpDir, backend, logger)

	// Test 1: Set bucket lifecycle
	t.Run("SetBucketLifecycle", func(t *testing.T) {
		rules := []LifecycleRule{
			{
				ID:                   "expire-old",
				Prefix:               "logs/",
				Enabled:              true,
				ExpirationDays:       30,
				TransitionDays:       0,
				AbortIncompleteMultipartDays: 7,
				DeleteMarkerExpiration: true,
			},
		}

		err := lm.SetBucketLifecycle("test-bucket", rules)
		if err != nil {
			t.Fatalf("Failed to set lifecycle: %v", err)
		}
	})

	// Test 2: Get bucket lifecycle
	t.Run("GetBucketLifecycle", func(t *testing.T) {
		config := lm.GetBucketLifecycle("test-bucket")
		if config == nil {
			t.Fatal("Failed to get lifecycle config")
		}

		if len(config.Rules) != 1 {
			t.Errorf("Expected 1 rule, got %d", len(config.Rules))
		}

		if config.Rules[0].ExpirationDays != 30 {
			t.Errorf("Expected expiration days 30, got %d", config.Rules[0].ExpirationDays)
		}
	})

	// Test 3: Should expire check
	t.Run("ShouldExpire", func(t *testing.T) {
		// Object older than 30 days
		oldTime := time.Now().Add(-31 * 24 * time.Hour)
		if !lm.ShouldExpire("test-bucket", "logs/old.txt", oldTime) {
			t.Error("Should expire old object")
		}

		// Object younger than 30 days
		newTime := time.Now().Add(-1 * 24 * time.Hour)
		if lm.ShouldExpire("test-bucket", "logs/new.txt", newTime) {
			t.Error("Should not expire new object")
		}

		// Object not matching prefix
		if lm.ShouldExpire("test-bucket", "data/old.txt", oldTime) {
			t.Error("Should not expire object not matching prefix")
		}
	})

	// Test 4: Delete bucket lifecycle
	t.Run("DeleteBucketLifecycle", func(t *testing.T) {
		err := lm.DeleteBucketLifecycle("test-bucket")
		if err != nil {
			t.Fatalf("Failed to delete lifecycle: %v", err)
		}

		config := lm.GetBucketLifecycle("test-bucket")
		if config != nil {
			t.Error("Lifecycle config should be nil after deletion")
		}
	})
}

func TestMetrics(t *testing.T) {
	metrics := NewMetrics()

	// Test 1: Record request
	t.Run("RecordRequest", func(t *testing.T) {
		metrics.RecordRequest("GET", "GetObject", "200", 100*time.Millisecond)
		// Metrics are recorded, no error expected
	})

	// Test 2: In-flight requests
	t.Run("InFlightRequests", func(t *testing.T) {
		metrics.IncRequestsInFlight()
		metrics.IncRequestsInFlight()
		metrics.DecRequestsInFlight()
		// No error expected
	})

	// Test 3: Record operations
	t.Run("RecordOperations", func(t *testing.T) {
		metrics.RecordUpload("mybucket", 1024)
		metrics.RecordDownload("mybucket", 512)
		metrics.RecordDelete()
		metrics.RecordList()
		metrics.RecordHead()
		// No error expected
	})

	// Test 4: Set gauges
	t.Run("SetGauges", func(t *testing.T) {
		metrics.SetObjectsStored(1000)
		metrics.SetBytesStored(1024 * 1024 * 100) // 100 MB
		metrics.SetBucketsTotal(5)
		// No error expected
	})

	// Test 5: Multipart metrics
	t.Run("MultipartMetrics", func(t *testing.T) {
		metrics.RecordMultipartUploadStarted()
		metrics.RecordMultipartUploadCompleted()
		metrics.RecordMultipartUploadStarted()
		metrics.RecordMultipartUploadAborted()
		// No error expected
	})

	// Test 6: Auth metrics
	t.Run("AuthMetrics", func(t *testing.T) {
		metrics.RecordAuthSuccess()
		metrics.RecordAuthSuccess()
		metrics.RecordAuthFailure()
		// No error expected
	})

	// Test 7: Error metrics
	t.Run("ErrorMetrics", func(t *testing.T) {
		metrics.RecordError("GetObject", "NoSuchKey")
		metrics.RecordError("PutObject", "InternalError")
		// No error expected
	})

	// Test 8: Lifecycle metrics
	t.Run("LifecycleMetrics", func(t *testing.T) {
		metrics.RecordLifecycleExpiration()
		// No error expected
	})
}

// Helper functions and mocks

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		len(s) >= len(substr) && 
		s[:len(substr)] == substr || 
		(len(s) > len(substr) && contains(s[1:], substr))
}

type mockObject struct {
	key          string
	lastModified time.Time
}

type mockBackend struct {
	objects map[string]*mockObject
}

func (m *mockBackend) List(bucket, prefix, marker string, maxKeys int) ([]Object, error) {
	var result []Object
	for key, obj := range m.objects {
		result = append(result, Object{
			Key:          key,
			LastModified: obj.lastModified,
		})
	}
	return result, nil
}

func (m *mockBackend) Delete(bucket, key string) error {
	delete(m.objects, key)
	return nil
}
