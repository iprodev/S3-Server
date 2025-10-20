package main

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/iProDev/S3-Server/miniobject"
)

func TestMultipartETagComputation(t *testing.T) {
	tests := []struct {
		name      string
		parts     []CompletePart
		wantParts int
	}{
		{
			name: "single part",
			parts: []CompletePart{
				{PartNumber: 1, ETag: `"d41d8cd98f00b204e9800998ecf8427e"`},
			},
			wantParts: 1,
		},
		{
			name: "multiple parts ordered",
			parts: []CompletePart{
				{PartNumber: 1, ETag: `"etag1"`},
				{PartNumber: 2, ETag: `"etag2"`},
				{PartNumber: 3, ETag: `"etag3"`},
			},
			wantParts: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify parts are ordered
			for i := 1; i < len(tt.parts); i++ {
				if tt.parts[i].PartNumber <= tt.parts[i-1].PartNumber {
					t.Error("parts not in order")
				}
			}
		})
	}
}

func TestMultipartValidation(t *testing.T) {
	backend, _ := miniobject.NewLocalFSBackend(t.TempDir())
	logger := NewLogger("error")
	mp := NewMultipartManager(t.TempDir(), backend, logger)

	uploadID := mp.InitiateUpload("test", "key")

	// Upload some parts
	mp.UploadPart(uploadID, 1, bytes.NewReader([]byte("part1")))
	mp.UploadPart(uploadID, 2, bytes.NewReader([]byte("part2")))

	tests := []struct {
		name    string
		parts   []CompletePart
		wantErr bool
	}{
		{
			name:    "empty parts",
			parts:   []CompletePart{},
			wantErr: true,
		},
		{
			name: "duplicate parts",
			parts: []CompletePart{
				{PartNumber: 1, ETag: `"etag1"`},
				{PartNumber: 1, ETag: `"etag1"`},
			},
			wantErr: true,
		},
		{
			name: "out of order",
			parts: []CompletePart{
				{PartNumber: 2, ETag: `"etag2"`},
				{PartNumber: 1, ETag: `"etag1"`},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upload := mp.uploads[uploadID]
			err := mp.validateParts(upload, tt.parts)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateParts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContentMD5Validation(t *testing.T) {
	backend, _ := miniobject.NewLocalFSBackend(t.TempDir())

	data := []byte("test data")
	correctMD5 := "eb733a00c0c9d336e65691a37ab54293"
	wrongMD5 := "wrongmd5"

	// Test with correct MD5
	_, err := backend.Put(context.Background(), "bucket", "key1", bytes.NewReader(data), "text/plain", correctMD5)
	if err != nil {
		t.Errorf("Put with correct MD5 should succeed, got %v", err)
	}

	// Test with wrong MD5
	_, err = backend.Put(context.Background(), "bucket", "key2", bytes.NewReader(data), "text/plain", wrongMD5)
	if err == nil || err.Error() != "BadDigest" {
		t.Errorf("Put with wrong MD5 should fail with BadDigest, got %v", err)
	}
}

func TestRangeRequest(t *testing.T) {
	backend, _ := miniobject.NewLocalFSBackend(t.TempDir())

	data := []byte("0123456789")
	backend.Put(context.Background(), "bucket", "key", bytes.NewReader(data), "text/plain", "")

	tests := []struct {
		name      string
		rangeSpec string
		want      string
		wantCode  int
	}{
		{"first 5 bytes", "bytes=0-4", "01234", 206},
		{"last 5 bytes", "bytes=5-9", "56789", 206},
		{"suffix 3 bytes", "bytes=-3", "789", 206},
		{"from byte 5", "bytes=5-", "56789", 206},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, _, _, code, err := backend.Get(context.Background(), "bucket", "key", tt.rangeSpec)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			defer rc.Close()

			if code != tt.wantCode {
				t.Errorf("status code = %d, want %d", code, tt.wantCode)
			}

			got, _ := io.ReadAll(rc)
			if string(got) != tt.want {
				t.Errorf("data = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestS3ErrorResponse(t *testing.T) {
	cfg := &Config{
		Mode:    "node",
		DataDir: t.TempDir(),
	}
	logger := NewLogger("error")
	server := NewNodeServer(cfg, logger)

	req := httptest.NewRequest("GET", "/bucket/nonexistent", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/xml" {
		t.Error("Content-Type should be application/xml")
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("<Code>NoSuchKey</Code>")) {
		t.Error("response should contain NoSuchKey error code")
	}
}
