package miniobject

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalFSBackend implements Backend interface using local filesystem
type LocalFSBackend struct {
	dataDir string
}

// NewLocalFSBackend creates a new local filesystem backend
func NewLocalFSBackend(dataDir string) (*LocalFSBackend, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}
	return &LocalFSBackend{dataDir: dataDir}, nil
}

type metadata struct {
	ContentType string `json:"content_type"`
	ETag        string `json:"etag"`
	Size        int64  `json:"size"`
}

func (fs *LocalFSBackend) objectPath(bucket, key string) string {
	return filepath.Join(fs.dataDir, bucket, key)
}

func (fs *LocalFSBackend) metadataPath(bucket, key string) string {
	return filepath.Join(fs.dataDir, bucket, key+".meta.json")
}

// Put stores an object with atomic write
func (fs *LocalFSBackend) Put(ctx context.Context, bucket, key string, r io.Reader, contentType, contentMD5 string) (string, error) {
	objPath := fs.objectPath(bucket, key)
	metaPath := fs.metadataPath(bucket, key)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(objPath), 0755); err != nil {
		return "", fmt.Errorf("mkdir failed: %w", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp(filepath.Dir(objPath), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp failed: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Compute MD5 while writing
	hash := md5.New()
	mw := io.MultiWriter(tmpFile, hash)
	size, err := io.Copy(mw, r)
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Fsync before close
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("fsync failed: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close failed: %w", err)
	}

	etag := `"` + hex.EncodeToString(hash.Sum(nil)) + `"`

	// Validate Content-MD5 if provided
	if contentMD5 != "" {
		expectedMD5 := strings.Trim(etag, `"`)
		if contentMD5 != expectedMD5 {
			return "", errors.New("BadDigest")
		}
	}

	// Write metadata
	meta := metadata{
		ContentType: contentType,
		ETag:        etag,
		Size:        size,
	}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return "", fmt.Errorf("write metadata failed: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, objPath); err != nil {
		return "", fmt.Errorf("rename failed: %w", err)
	}

	// Fsync parent directory for durability
	if parent, err := os.Open(filepath.Dir(objPath)); err == nil {
		parent.Sync()
		parent.Close()
	}

	return etag, nil
}

// Get retrieves an object
func (fs *LocalFSBackend) Get(ctx context.Context, bucket, key string, rangeSpec string) (io.ReadCloser, string, string, int64, int, error) {
	objPath := fs.objectPath(bucket, key)
	metaPath := fs.metadataPath(bucket, key)

	// Read metadata
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", "", 0, 404, errors.New("NoSuchKey")
		}
		return nil, "", "", 0, 500, err
	}

	var meta metadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, "", "", 0, 500, err
	}

	// Open file
	f, err := os.Open(objPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", "", 0, 404, errors.New("NoSuchKey")
		}
		return nil, "", "", 0, 500, err
	}

	// Handle range request
	if rangeSpec != "" {
		start, end, err := parseRange(rangeSpec, meta.Size)
		if err != nil {
			f.Close()
			return nil, "", "", 0, 416, err
		}

		if _, err := f.Seek(start, 0); err != nil {
			f.Close()
			return nil, "", "", 0, 500, err
		}

		// Return limited reader for range
		lr := &io.LimitedReader{R: f, N: end - start + 1}
		rc := &rangeReadCloser{lr: lr, f: f}
		return rc, meta.ContentType, meta.ETag, end - start + 1, 206, nil
	}

	return f, meta.ContentType, meta.ETag, meta.Size, 200, nil
}

type rangeReadCloser struct {
	lr *io.LimitedReader
	f  *os.File
}

func (rc *rangeReadCloser) Read(p []byte) (int, error) {
	return rc.lr.Read(p)
}

func (rc *rangeReadCloser) Close() error {
	return rc.f.Close()
}

// Head returns object metadata
func (fs *LocalFSBackend) Head(ctx context.Context, bucket, key string) (string, string, int64, bool, error) {
	metaPath := fs.metadataPath(bucket, key)

	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", 0, false, nil
		}
		return "", "", 0, false, err
	}

	var meta metadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return "", "", 0, false, err
	}

	return meta.ContentType, meta.ETag, meta.Size, true, nil
}

// Delete removes an object
func (fs *LocalFSBackend) Delete(ctx context.Context, bucket, key string) error {
	objPath := fs.objectPath(bucket, key)
	metaPath := fs.metadataPath(bucket, key)

	os.Remove(objPath)
	os.Remove(metaPath)
	return nil
}

// List lists objects with prefix
func (fs *LocalFSBackend) List(ctx context.Context, bucket, prefix, marker string, limit int) ([]ObjectInfo, error) {
	bucketPath := filepath.Join(fs.dataDir, bucket)

	var results []ObjectInfo
	err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		relPath, _ := filepath.Rel(bucketPath, path)
		relPath = filepath.ToSlash(relPath)

		if prefix != "" && !strings.HasPrefix(relPath, prefix) {
			return nil
		}

		if marker != "" && relPath <= marker {
			return nil
		}

		metaPath := path + ".meta.json"
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			return nil
		}

		var meta metadata
		if err := json.Unmarshal(metaData, &meta); err != nil {
			return nil
		}

		results = append(results, ObjectInfo{
			Key:          relPath,
			Size:         meta.Size,
			LastModified: info.ModTime().UTC().Format("2006-01-02T15:04:05.000Z"),
			ETag:         meta.ETag,
			ContentType:  meta.ContentType,
		})

		if len(results) >= limit {
			return filepath.SkipAll
		}

		return nil
	})

	return results, err
}

func parseRange(rangeSpec string, size int64) (int64, int64, error) {
	// Format: "bytes=start-end"
	if !strings.HasPrefix(rangeSpec, "bytes=") {
		return 0, 0, errors.New("invalid range")
	}

	rangeSpec = strings.TrimPrefix(rangeSpec, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range")
	}

	var start, end int64

	if parts[0] == "" {
		// "-500" means last 500 bytes
		if parts[1] == "" {
			return 0, 0, errors.New("invalid range")
		}
		suffix := int64(0)
		fmt.Sscanf(parts[1], "%d", &suffix)
		start = size - suffix
		if start < 0 {
			start = 0
		}
		end = size - 1
	} else if parts[1] == "" {
		// "500-" means from 500 to end
		fmt.Sscanf(parts[0], "%d", &start)
		end = size - 1
	} else {
		// "500-999"
		fmt.Sscanf(parts[0], "%d", &start)
		fmt.Sscanf(parts[1], "%d", &end)
	}

	if start < 0 || end >= size || start > end {
		return 0, 0, errors.New("range not satisfiable")
	}

	return start, end, nil
}
