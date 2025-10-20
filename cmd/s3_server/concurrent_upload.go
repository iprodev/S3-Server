package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iProDev/S3-Server/miniobject"
)

const (
	DefaultChunkSize    = 5 * 1024 * 1024 // 5MB minimum chunk size
	MaxConcurrentChunks = 10              // Maximum parallel uploads
	DefaultConcurrency  = 5               // Default parallel uploads
	MaxPartNumber       = 10000           // S3 limit
)

var (
	ErrInvalidChunkSize = errors.New("chunk size must be at least 5MB")
	ErrTooManyParts     = errors.New("too many parts (max 10000)")
	ErrPartTooSmall     = errors.New("part size too small (min 5MB except last)")
)

// ConcurrentUpload manages a concurrent multipart upload
type ConcurrentUpload struct {
	UploadID     string
	Bucket       string
	Key          string
	Parts        []*UploadPart
	Concurrency  int
	ChunkSize    int64
	TotalSize    int64
	UploadedSize int64
	StartTime    time.Time
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// UploadPart represents a single part in a concurrent upload
type UploadPart struct {
	PartNumber int
	Size       int64
	ETag       string
	Offset     int64
	Uploaded   bool
	Error      error
	StartTime  time.Time
	EndTime    time.Time
}

// UploadProgress tracks upload progress
type UploadProgress struct {
	TotalBytes    int64
	UploadedBytes int64
	PartsTotal    int
	PartsComplete int
	PartsFailed   int
	StartTime     time.Time
	ElapsedTime   time.Duration
	Speed         float64 // bytes per second
	ETA           time.Duration
}

// ConcurrentUploadManager manages concurrent multipart uploads
type ConcurrentUploadManager struct {
	backend   miniobject.Backend
	multipart *MultipartManager
	uploads   map[string]*ConcurrentUpload
	mu        sync.RWMutex
	logger    *Logger
}

// NewConcurrentUploadManager creates a new concurrent upload manager
func NewConcurrentUploadManager(backend miniobject.Backend, multipart *MultipartManager, logger *Logger) *ConcurrentUploadManager {
	return &ConcurrentUploadManager{
		backend:   backend,
		multipart: multipart,
		uploads:   make(map[string]*ConcurrentUpload),
		logger:    logger,
	}
}

// InitiateConcurrentUpload starts a new concurrent upload
func (cum *ConcurrentUploadManager) InitiateConcurrentUpload(bucket, key string, totalSize int64, chunkSize int64, concurrency int) (*ConcurrentUpload, error) {
	// Validate parameters
	if chunkSize < DefaultChunkSize {
		chunkSize = DefaultChunkSize
	}

	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	if concurrency > MaxConcurrentChunks {
		concurrency = MaxConcurrentChunks
	}

	// Calculate number of parts
	numParts := int((totalSize + chunkSize - 1) / chunkSize)
	if numParts > MaxPartNumber {
		return nil, ErrTooManyParts
	}

	// Create upload
	ctx, cancel := context.WithCancel(context.Background())
	upload := &ConcurrentUpload{
		UploadID:    generateUploadID(),
		Bucket:      bucket,
		Key:         key,
		Concurrency: concurrency,
		ChunkSize:   chunkSize,
		TotalSize:   totalSize,
		StartTime:   time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Create parts
	for i := 0; i < numParts; i++ {
		offset := int64(i) * chunkSize
		size := chunkSize
		if offset+size > totalSize {
			size = totalSize - offset
		}

		upload.Parts = append(upload.Parts, &UploadPart{
			PartNumber: i + 1,
			Size:       size,
			Offset:     offset,
			Uploaded:   false,
		})
	}

	// Register upload
	cum.mu.Lock()
	cum.uploads[upload.UploadID] = upload
	cum.mu.Unlock()

	cum.logger.Info("concurrent upload initiated",
		"upload_id", upload.UploadID,
		"bucket", bucket,
		"key", key,
		"total_size", totalSize,
		"chunk_size", chunkSize,
		"parts", len(upload.Parts),
		"concurrency", concurrency)

	return upload, nil
}

// UploadParts uploads parts concurrently
func (cum *ConcurrentUploadManager) UploadParts(uploadID string, reader io.ReaderAt) error {
	upload, err := cum.getUpload(uploadID)
	if err != nil {
		return err
	}

	// Create worker pool
	partsChan := make(chan *UploadPart, len(upload.Parts))
	resultsChan := make(chan *UploadPart, len(upload.Parts))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < upload.Concurrency; i++ {
		wg.Add(1)
		go cum.uploadWorker(upload, reader, partsChan, resultsChan, &wg)
	}

	// Send parts to workers
	for _, part := range upload.Parts {
		if !part.Uploaded {
			partsChan <- part
		}
	}
	close(partsChan)

	// Wait for workers
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var failures int
	for part := range resultsChan {
		if part.Error != nil {
			failures++
			cum.logger.Error("part upload failed",
				"upload_id", uploadID,
				"part", part.PartNumber,
				"error", part.Error)
		} else {
			atomic.AddInt64(&upload.UploadedSize, part.Size)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d parts failed to upload", failures)
	}

	cum.logger.Info("concurrent upload completed",
		"upload_id", uploadID,
		"parts", len(upload.Parts),
		"size", upload.TotalSize,
		"duration", time.Since(upload.StartTime))

	return nil
}

// uploadWorker uploads parts from the queue
func (cum *ConcurrentUploadManager) uploadWorker(upload *ConcurrentUpload, reader io.ReaderAt, parts <-chan *UploadPart, results chan<- *UploadPart, wg *sync.WaitGroup) {
	defer wg.Done()

	for part := range parts {
		// Check if cancelled
		select {
		case <-upload.ctx.Done():
			part.Error = errors.New("upload cancelled")
			results <- part
			continue
		default:
		}

		part.StartTime = time.Now()

		// Read part data
		data := make([]byte, part.Size)
		n, err := reader.ReadAt(data, part.Offset)
		if err != nil && err != io.EOF {
			part.Error = fmt.Errorf("read part data: %w", err)
			results <- part
			continue
		}
		data = data[:n]

		// Calculate ETag (MD5)
		hash := md5.Sum(data)
		part.ETag = hex.EncodeToString(hash[:])

		// Upload part
		partKey := fmt.Sprintf("%s.part.%d", upload.Key, part.PartNumber)
		_, err = cum.backend.Put(upload.ctx, upload.Bucket, partKey, io.NopCloser(newBytesReaderFromBytes(data)), "application/octet-stream", "")
		if err != nil {
			part.Error = fmt.Errorf("upload part: %w", err)
			results <- part
			continue
		}

		part.Uploaded = true
		part.EndTime = time.Now()

		cum.logger.Debug("part uploaded",
			"upload_id", upload.UploadID,
			"part", part.PartNumber,
			"size", part.Size,
			"duration", part.EndTime.Sub(part.StartTime))

		results <- part
	}
}

// GetProgress returns the current upload progress
func (cum *ConcurrentUploadManager) GetProgress(uploadID string) (*UploadProgress, error) {
	upload, err := cum.getUpload(uploadID)
	if err != nil {
		return nil, err
	}

	upload.mu.RLock()
	defer upload.mu.RUnlock()

	progress := &UploadProgress{
		TotalBytes:    upload.TotalSize,
		UploadedBytes: atomic.LoadInt64(&upload.UploadedSize),
		PartsTotal:    len(upload.Parts),
		StartTime:     upload.StartTime,
	}

	// Count completed and failed parts
	for _, part := range upload.Parts {
		if part.Uploaded {
			progress.PartsComplete++
		}
		if part.Error != nil {
			progress.PartsFailed++
		}
	}

	// Calculate metrics
	progress.ElapsedTime = time.Since(upload.StartTime)
	if progress.ElapsedTime.Seconds() > 0 {
		progress.Speed = float64(progress.UploadedBytes) / progress.ElapsedTime.Seconds()
	}

	if progress.Speed > 0 {
		remaining := progress.TotalBytes - progress.UploadedBytes
		progress.ETA = time.Duration(float64(remaining)/progress.Speed) * time.Second
	}

	return progress, nil
}

// CompleteConcurrentUpload finalizes a concurrent upload
func (cum *ConcurrentUploadManager) CompleteConcurrentUpload(uploadID string) (string, error) {
	upload, err := cum.getUpload(uploadID)
	if err != nil {
		return "", err
	}

	// Verify all parts uploaded
	upload.mu.RLock()
	for _, part := range upload.Parts {
		if !part.Uploaded {
			upload.mu.RUnlock()
			return "", fmt.Errorf("part %d not uploaded", part.PartNumber)
		}
		if part.Error != nil {
			upload.mu.RUnlock()
			return "", fmt.Errorf("part %d has error: %w", part.PartNumber, part.Error)
		}
	}
	upload.mu.RUnlock()

	// Sort parts by part number
	sortedParts := make([]*UploadPart, len(upload.Parts))
	copy(sortedParts, upload.Parts)
	sort.Slice(sortedParts, func(i, j int) bool {
		return sortedParts[i].PartNumber < sortedParts[j].PartNumber
	})

	// Combine parts (simplified - in production would stream)
	var combinedData []byte
	for _, part := range sortedParts {
		partKey := fmt.Sprintf("%s.part.%d", upload.Key, part.PartNumber)
		rc, _, _, _, _, err := cum.backend.Get(upload.ctx, upload.Bucket, partKey, "")
		if err != nil {
			return "", fmt.Errorf("read part %d: %w", part.PartNumber, err)
		}

		partData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("read part %d data: %w", part.PartNumber, err)
		}

		combinedData = append(combinedData, partData...)

		// Clean up part
		cum.backend.Delete(upload.ctx, upload.Bucket, partKey)
	}

	// Write combined object
	etag, err := cum.backend.Put(upload.ctx, upload.Bucket, upload.Key, io.NopCloser(newBytesReaderFromBytes(combinedData)), "application/octet-stream", "")
	if err != nil {
		return "", fmt.Errorf("write combined object: %w", err)
	}

	// Clean up
	cum.mu.Lock()
	delete(cum.uploads, uploadID)
	cum.mu.Unlock()

	cum.logger.Info("concurrent upload finalized",
		"upload_id", uploadID,
		"bucket", upload.Bucket,
		"key", upload.Key,
		"size", upload.TotalSize,
		"parts", len(upload.Parts),
		"total_duration", time.Since(upload.StartTime),
		"etag", etag)

	return etag, nil
}

// AbortConcurrentUpload cancels a concurrent upload
func (cum *ConcurrentUploadManager) AbortConcurrentUpload(uploadID string) error {
	upload, err := cum.getUpload(uploadID)
	if err != nil {
		return err
	}

	// Cancel context
	upload.cancel()

	// Clean up parts
	for _, part := range upload.Parts {
		if part.Uploaded {
			partKey := fmt.Sprintf("%s.part.%d", upload.Key, part.PartNumber)
			cum.backend.Delete(context.Background(), upload.Bucket, partKey)
		}
	}

	// Remove upload
	cum.mu.Lock()
	delete(cum.uploads, uploadID)
	cum.mu.Unlock()

	cum.logger.Info("concurrent upload aborted", "upload_id", uploadID)

	return nil
}

// RetryFailedParts retries uploading failed parts
func (cum *ConcurrentUploadManager) RetryFailedParts(uploadID string, reader io.ReaderAt) error {
	upload, err := cum.getUpload(uploadID)
	if err != nil {
		return err
	}

	// Find failed parts
	var failedParts []*UploadPart
	upload.mu.RLock()
	for _, part := range upload.Parts {
		if !part.Uploaded || part.Error != nil {
			// Reset part
			part.Uploaded = false
			part.Error = nil
			failedParts = append(failedParts, part)
		}
	}
	upload.mu.RUnlock()

	if len(failedParts) == 0 {
		return nil
	}

	cum.logger.Info("retrying failed parts",
		"upload_id", uploadID,
		"failed_parts", len(failedParts))

	// Create worker pool for retries
	partsChan := make(chan *UploadPart, len(failedParts))
	resultsChan := make(chan *UploadPart, len(failedParts))

	var wg sync.WaitGroup
	for i := 0; i < upload.Concurrency; i++ {
		wg.Add(1)
		go cum.uploadWorker(upload, reader, partsChan, resultsChan, &wg)
	}

	// Send failed parts
	for _, part := range failedParts {
		partsChan <- part
	}
	close(partsChan)

	// Wait and collect
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var stillFailed int
	for part := range resultsChan {
		if part.Error != nil {
			stillFailed++
		}
	}

	if stillFailed > 0 {
		return fmt.Errorf("%d parts still failed after retry", stillFailed)
	}

	return nil
}

// Helper methods

func (cum *ConcurrentUploadManager) getUpload(uploadID string) (*ConcurrentUpload, error) {
	cum.mu.RLock()
	defer cum.mu.RUnlock()

	upload, ok := cum.uploads[uploadID]
	if !ok {
		return nil, errors.New("upload not found")
	}

	return upload, nil
}

func generateUploadID() string {
	return fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
}

func newBytesReaderFromBytes(data []byte) io.Reader {
	return &bytesReaderSimple{data: data, pos: 0}
}

type bytesReaderSimple struct {
	data []byte
	pos  int
}

func (br *bytesReaderSimple) Read(p []byte) (n int, err error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n = copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}
