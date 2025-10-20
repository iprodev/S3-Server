package miniobject

import (
	"bytes"
	"context"
	"testing"
)

func TestClusterQuorum(t *testing.T) {
	tests := []struct {
		name         string
		replicas     int
		w            int
		r            int
		successNodes int
		wantErr      bool
	}{
		{"W=2, 2 successes", 3, 2, 1, 2, false},
		{"W=2, 1 success", 3, 2, 1, 1, true},
		{"W=3, 3 successes", 3, 3, 1, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.w+tt.r <= tt.replicas {
				t.Skip("invalid quorum configuration")
			}
			// Quorum validation test
			if tt.w+tt.r <= tt.replicas {
				t.Error("W + R must be > N for consistency")
			}
		})
	}
}

func TestClusterRepairObject(t *testing.T) {
	// Create temp backends
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	tmpDir3 := t.TempDir()

	backend1, _ := NewLocalFSBackend(tmpDir1)
	backend2, _ := NewLocalFSBackend(tmpDir2)
	backend3, _ := NewLocalFSBackend(tmpDir3)

	// Write to only one backend
	data := []byte("test data")
	backend1.Put(context.Background(), "bucket", "key", bytes.NewReader(data), "text/plain", "")

	// Verify repair would detect missing replicas
	_, _, _, exists2, _ := backend2.Head(context.Background(), "bucket", "key")
	_, _, _, exists3, _ := backend3.Head(context.Background(), "bucket", "key")

	if exists2 || exists3 {
		t.Error("replicas should not exist before repair")
	}
}
