package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/iProDev/s3_server/miniobject"
)

type MultipartUpload struct {
	UploadID  string
	Bucket    string
	Key       string
	Parts     map[int]PartInfo
	CreatedAt time.Time
	mu        sync.RWMutex
}

type PartInfo struct {
	PartNumber int
	ETag       string
	Size       int64
}

type MultipartManager struct {
	uploads map[string]*MultipartUpload
	mu      sync.RWMutex
	tmpDir  string
	backend miniobject.Backend
	logger  *Logger
}

func NewMultipartManager(tmpDir string, backend miniobject.Backend, logger *Logger) *MultipartManager {
	return &MultipartManager{
		uploads: make(map[string]*MultipartUpload),
		tmpDir:  tmpDir,
		backend: backend,
		logger:  logger,
	}
}

func (m *MultipartManager) InitiateUpload(bucket, key string) string {
	uploadID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), generateRequestID())

	upload := &MultipartUpload{
		UploadID:  uploadID,
		Bucket:    bucket,
		Key:       key,
		Parts:     make(map[int]PartInfo),
		CreatedAt: time.Now(),
	}

	m.mu.Lock()
	m.uploads[uploadID] = upload
	m.mu.Unlock()

	// Create temp directory for parts
	os.MkdirAll(m.partDir(uploadID), 0755)

	return uploadID
}

func (m *MultipartManager) UploadPart(uploadID string, partNumber int, data io.Reader) (string, error) {
	m.mu.RLock()
	upload, exists := m.uploads[uploadID]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("NoSuchUpload")
	}

	if partNumber < 1 || partNumber > 10000 {
		return "", fmt.Errorf("InvalidPart")
	}

	// Write part to temp file
	partPath := m.partPath(uploadID, partNumber)
	f, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := md5.New()
	size, err := io.Copy(io.MultiWriter(f, hash), data)
	if err != nil {
		return "", err
	}

	etag := `"` + hex.EncodeToString(hash.Sum(nil)) + `"`

	upload.mu.Lock()
	upload.Parts[partNumber] = PartInfo{
		PartNumber: partNumber,
		ETag:       etag,
		Size:       size,
	}
	upload.mu.Unlock()

	return etag, nil
}

func (m *MultipartManager) CompleteUpload(uploadID string, parts []CompletePart) (string, error) {
	m.mu.RLock()
	upload, exists := m.uploads[uploadID]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("NoSuchUpload")
	}

	// Validate parts
	if err := m.validateParts(upload, parts); err != nil {
		return "", err
	}

	// Concatenate parts and compute final ETag
	finalPath := filepath.Join(m.tmpDir, "complete-"+uploadID)
	final, err := os.Create(finalPath)
	if err != nil {
		return "", err
	}
	defer final.Close()
	defer os.Remove(finalPath)

	combinedHash := md5.New()

	for _, part := range parts {
		partPath := m.partPath(uploadID, part.PartNumber)
		pf, err := os.Open(partPath)
		if err != nil {
			return "", err
		}

		partHash := md5.New()
		if _, err := io.Copy(io.MultiWriter(final, partHash), pf); err != nil {
			pf.Close()
			return "", err
		}
		pf.Close()

		// Add part MD5 to combined hash
		combinedHash.Write(partHash.Sum(nil))
	}

	// Final ETag: MD5(concat of part MD5s) + "-" + part count
	finalETag := hex.EncodeToString(combinedHash.Sum(nil)) + "-" + strconv.Itoa(len(parts))
	finalETag = `"` + finalETag + `"`

	// Upload to backend
	final.Seek(0, 0)
	_, err = m.backend.Put(nil, upload.Bucket, upload.Key, final, "application/octet-stream", "")
	if err != nil {
		return "", err
	}

	// Cleanup
	m.cleanup(uploadID)

	return finalETag, nil
}

func (m *MultipartManager) AbortUpload(uploadID string) error {
	m.mu.Lock()
	delete(m.uploads, uploadID)
	m.mu.Unlock()

	m.cleanup(uploadID)
	return nil
}

func (m *MultipartManager) validateParts(upload *MultipartUpload, parts []CompletePart) error {
	upload.mu.RLock()
	defer upload.mu.RUnlock()

	if len(parts) == 0 {
		return fmt.Errorf("InvalidPart")
	}

	// Check ordering and duplicates
	seen := make(map[int]bool)
	prevNum := 0

	for _, part := range parts {
		if part.PartNumber <= prevNum {
			return fmt.Errorf("InvalidPartOrder")
		}
		if seen[part.PartNumber] {
			return fmt.Errorf("InvalidPart")
		}
		seen[part.PartNumber] = true

		// Verify ETag matches
		stored, exists := upload.Parts[part.PartNumber]
		if !exists {
			return fmt.Errorf("InvalidPart")
		}
		if stored.ETag != part.ETag {
			return fmt.Errorf("InvalidPart")
		}

		prevNum = part.PartNumber
	}

	return nil
}

func (m *MultipartManager) cleanup(uploadID string) {
	os.RemoveAll(m.partDir(uploadID))
}

func (m *MultipartManager) partDir(uploadID string) string {
	return filepath.Join(m.tmpDir, "multipart-"+uploadID)
}

func (m *MultipartManager) partPath(uploadID string, partNumber int) string {
	return filepath.Join(m.partDir(uploadID), fmt.Sprintf("part-%d", partNumber))
}

func (m *MultipartManager) SweepStale(ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, upload := range m.uploads {
		if now.Sub(upload.CreatedAt) > ttl {
			m.logger.Info("sweeping stale multipart upload", "uploadId", id, "age", now.Sub(upload.CreatedAt))
			delete(m.uploads, id)
			go m.cleanup(id)
		}
	}
}

// XML structs

type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadId string   `xml:"UploadId"`
}

type CompleteMultipartUpload struct {
	XMLName xml.Name       `xml:"CompleteMultipartUpload"`
	Parts   []CompletePart `xml:"Part"`
}

type CompletePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

// Gateway handlers for multipart

func (s *GatewayServer) handleInitiateMultipart(w http.ResponseWriter, r *http.Request, bucket, key string) {
	uploadID := s.multipart.InitiateUpload(bucket, key)

	result := InitiateMultipartUploadResult{
		Bucket:   bucket,
		Key:      key,
		UploadId: uploadID,
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(result)
}

func (s *GatewayServer) handleUploadPart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	partNumberStr := r.URL.Query().Get("partNumber")
	partNumber, err := strconv.Atoi(partNumberStr)
	if err != nil || partNumber < 1 || partNumber > 10000 {
		s.writeS3Error(w, r, "InvalidPart", "Invalid part number", http.StatusBadRequest)
		return
	}

	etag, err := s.multipart.UploadPart(uploadID, partNumber, r.Body)
	if err != nil {
		if err.Error() == "NoSuchUpload" {
			s.writeS3Error(w, r, "NoSuchUpload", "Upload not found", http.StatusNotFound)
			return
		}
		s.logger.Error("upload part failed", "uploadId", uploadID, "part", partNumber, "error", err)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func (s *GatewayServer) handleCompleteMultipart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	var complete CompleteMultipartUpload
	if err := xml.NewDecoder(r.Body).Decode(&complete); err != nil {
		s.writeS3Error(w, r, "MalformedXML", "Malformed XML", http.StatusBadRequest)
		return
	}

	etag, err := s.multipart.CompleteUpload(uploadID, complete.Parts)
	if err != nil {
		if err.Error() == "NoSuchUpload" {
			s.writeS3Error(w, r, "NoSuchUpload", "Upload not found", http.StatusNotFound)
			return
		}
		if err.Error() == "InvalidPart" {
			s.writeS3Error(w, r, "InvalidPart", "Invalid part", http.StatusBadRequest)
			return
		}
		if err.Error() == "InvalidPartOrder" {
			s.writeS3Error(w, r, "InvalidPartOrder", "Parts not in order", http.StatusBadRequest)
			return
		}
		s.logger.Error("complete multipart failed", "uploadId", uploadID, "error", err)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	result := CompleteMultipartUploadResult{
		Location: fmt.Sprintf("/%s/%s", bucket, key),
		Bucket:   bucket,
		Key:      key,
		ETag:     etag,
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(result)
}

func (s *GatewayServer) handleAbortMultipart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	err := s.multipart.AbortUpload(uploadID)
	if err != nil {
		s.logger.Error("abort multipart failed", "uploadId", uploadID, "error", err)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
