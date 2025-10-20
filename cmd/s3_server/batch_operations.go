package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iProDev/s3_server/miniobject"
)

var (
	ErrBatchTooLarge = errors.New("batch size exceeds maximum allowed")
	ErrEmptyBatch    = errors.New("batch operation list is empty")
)

const (
	MaxBatchSize = 1000 // Maximum objects per batch operation
)

// BatchOperation types
type BatchOperationType string

const (
	BatchDelete  BatchOperationType = "delete"
	BatchCopy    BatchOperationType = "copy"
	BatchMove    BatchOperationType = "move"
	BatchRestore BatchOperationType = "restore"
)

// BatchRequest represents a batch operation request
type BatchRequest struct {
	Operation  BatchOperationType `json:"operation"`
	Operations []BatchItem        `json:"operations"`
	Options    BatchOptions       `json:"options,omitempty"`
}

// BatchItem represents a single item in a batch operation
type BatchItem struct {
	Bucket        string `json:"bucket"`
	Key           string `json:"key"`
	VersionID     string `json:"version_id,omitempty"`
	DestBucket    string `json:"dest_bucket,omitempty"` // For copy/move
	DestKey       string `json:"dest_key,omitempty"`    // For copy/move
}

// BatchOptions configuration options for batch operations
type BatchOptions struct {
	Concurrency   int  `json:"concurrency"`    // Number of parallel operations
	ContinueOnError bool `json:"continue_on_error"` // Continue if individual operations fail
	DryRun        bool `json:"dry_run"`        // Preview without executing
}

// BatchResponse represents the response from a batch operation
type BatchResponse struct {
	JobID        string              `json:"job_id"`
	Operation    BatchOperationType  `json:"operation"`
	TotalItems   int                 `json:"total_items"`
	Successful   int                 `json:"successful"`
	Failed       int                 `json:"failed"`
	Errors       []BatchItemError    `json:"errors,omitempty"`
	Duration     time.Duration       `json:"duration"`
	StartedAt    time.Time           `json:"started_at"`
	CompletedAt  time.Time           `json:"completed_at"`
}

// BatchItemError represents an error for a specific item
type BatchItemError struct {
	Item  BatchItem `json:"item"`
	Error string    `json:"error"`
}

// BatchProcessor processes batch operations
type BatchProcessor struct {
	backend        miniobject.Backend
	versionManager *VersionManager
	logger         *Logger
	jobs           map[string]*BatchJob
	mu             sync.RWMutex
}

// BatchJob represents a running or completed batch job
type BatchJob struct {
	ID          string
	Request     *BatchRequest
	Response    *BatchResponse
	Status      string // running, completed, failed
	mu          sync.RWMutex
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(backend miniobject.Backend, versionManager *VersionManager, logger *Logger) *BatchProcessor {
	return &BatchProcessor{
		backend:        backend,
		versionManager: versionManager,
		logger:         logger,
		jobs:           make(map[string]*BatchJob),
	}
}

// Execute executes a batch operation
func (bp *BatchProcessor) Execute(ctx context.Context, request *BatchRequest) (*BatchResponse, error) {
	// Validate request
	if err := bp.validateRequest(request); err != nil {
		return nil, err
	}
	
	// Set defaults
	if request.Options.Concurrency == 0 {
		request.Options.Concurrency = 10 // Default concurrency
	}
	
	// Create job
	job := &BatchJob{
		ID:      generateJobID(),
		Request: request,
		Status:  "running",
		Response: &BatchResponse{
			JobID:      generateJobID(),
			Operation:  request.Operation,
			TotalItems: len(request.Operations),
			StartedAt:  time.Now(),
		},
	}
	
	// Register job
	bp.mu.Lock()
	bp.jobs[job.ID] = job
	bp.mu.Unlock()
	
	// Execute asynchronously if not dry run
	if request.Options.DryRun {
		return bp.dryRun(ctx, job)
	}
	
	// Execute in goroutine
	go bp.executeJob(ctx, job)
	
	return job.Response, nil
}

// executeJob executes a batch job
func (bp *BatchProcessor) executeJob(ctx context.Context, job *BatchJob) {
	defer func() {
		job.mu.Lock()
		job.Response.CompletedAt = time.Now()
		job.Response.Duration = job.Response.CompletedAt.Sub(job.Response.StartedAt)
		job.Status = "completed"
		job.mu.Unlock()
		
		bp.logger.Info("batch job completed",
			"job_id", job.ID,
			"operation", job.Request.Operation,
			"total", job.Response.TotalItems,
			"successful", job.Response.Successful,
			"failed", job.Response.Failed,
			"duration", job.Response.Duration)
	}()
	
	// Create worker pool
	workers := job.Request.Options.Concurrency
	items := make(chan BatchItem, len(job.Request.Operations))
	results := make(chan *batchResult, len(job.Request.Operations))
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go bp.worker(ctx, job, items, results, &wg)
	}
	
	// Send items to workers
	for _, item := range job.Request.Operations {
		items <- item
	}
	close(items)
	
	// Wait for workers
	go func() {
		wg.Wait()
		close(results)
	}()
	
	// Collect results
	var successful, failed int32
	var errors []BatchItemError
	
	for result := range results {
		if result.err != nil {
			atomic.AddInt32(&failed, 1)
			errors = append(errors, BatchItemError{
				Item:  result.item,
				Error: result.err.Error(),
			})
		} else {
			atomic.AddInt32(&successful, 1)
		}
	}
	
	// Update response
	job.mu.Lock()
	job.Response.Successful = int(successful)
	job.Response.Failed = int(failed)
	job.Response.Errors = errors
	job.mu.Unlock()
}

type batchResult struct {
	item BatchItem
	err  error
}

// worker processes batch items
func (bp *BatchProcessor) worker(ctx context.Context, job *BatchJob, items <-chan BatchItem, results chan<- *batchResult, wg *sync.WaitGroup) {
	defer wg.Done()
	
	for item := range items {
		var err error
		
		switch job.Request.Operation {
		case BatchDelete:
			err = bp.deleteItem(ctx, item)
		case BatchCopy:
			err = bp.copyItem(ctx, item)
		case BatchMove:
			err = bp.moveItem(ctx, item)
		case BatchRestore:
			err = bp.restoreItem(ctx, item)
		default:
			err = fmt.Errorf("unknown operation: %s", job.Request.Operation)
		}
		
		results <- &batchResult{item: item, err: err}
		
		// Stop on first error if not continuing
		if err != nil && !job.Request.Options.ContinueOnError {
			break
		}
	}
}

// deleteItem deletes a single item
func (bp *BatchProcessor) deleteItem(ctx context.Context, item BatchItem) error {
	if item.VersionID != "" {
		// Delete specific version - for now just delete normally
		// Full version deletion would need versionManager.DeleteSpecificVersion method
		return bp.backend.Delete(ctx, item.Bucket, item.Key)
	}
	
	// Delete object (creates delete marker if versioned)
	return bp.backend.Delete(ctx, item.Bucket, item.Key)
}

// copyItem copies a single item
func (bp *BatchProcessor) copyItem(ctx context.Context, item BatchItem) error {
	// Read source
	rc, contentType, etag, _, _, err := bp.backend.Get(ctx, item.Bucket, item.Key, "")
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	defer rc.Close()
	
	// Read all data
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}
	
	// Write to destination
	destBucket := item.DestBucket
	if destBucket == "" {
		destBucket = item.Bucket
	}
	
	destKey := item.DestKey
	if destKey == "" {
		destKey = item.Key
	}
	
	_, err = bp.backend.Put(ctx, destBucket, destKey, io.NopCloser(newBytesReader(data)), contentType, "")
	if err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	
	bp.logger.Debug("copied object",
		"source_bucket", item.Bucket,
		"source_key", item.Key,
		"dest_bucket", destBucket,
		"dest_key", destKey,
		"etag", etag)
	
	return nil
}

// moveItem moves a single item
func (bp *BatchProcessor) moveItem(ctx context.Context, item BatchItem) error {
	// Copy first
	if err := bp.copyItem(ctx, item); err != nil {
		return err
	}
	
	// Delete source
	return bp.backend.Delete(ctx, item.Bucket, item.Key)
}

// restoreItem restores a specific version
func (bp *BatchProcessor) restoreItem(ctx context.Context, item BatchItem) error {
	if item.VersionID == "" {
		return errors.New("version_id required for restore operation")
	}
	
	// For now, restore is not fully implemented
	// Would need to copy the specific version to be the latest version
	return errors.New("restore not yet implemented")
}

// dryRun simulates a batch operation
func (bp *BatchProcessor) dryRun(ctx context.Context, job *BatchJob) (*BatchResponse, error) {
	job.Response.CompletedAt = time.Now()
	job.Response.Duration = time.Millisecond // Instant for dry run
	job.Status = "dry_run"
	
	// Validate each item
	var errors []BatchItemError
	
	for _, item := range job.Request.Operations {
		if err := bp.validateItem(item, job.Request.Operation); err != nil {
			errors = append(errors, BatchItemError{
				Item:  item,
				Error: err.Error(),
			})
		}
	}
	
	if len(errors) > 0 {
		job.Response.Failed = len(errors)
		job.Response.Successful = job.Response.TotalItems - len(errors)
		job.Response.Errors = errors
	} else {
		job.Response.Successful = job.Response.TotalItems
	}
	
	return job.Response, nil
}

// GetJob returns the status of a batch job
func (bp *BatchProcessor) GetJob(jobID string) (*BatchJob, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	
	job, ok := bp.jobs[jobID]
	if !ok {
		return nil, errors.New("job not found")
	}
	
	return job, nil
}

// ListJobs returns all batch jobs
func (bp *BatchProcessor) ListJobs() []*BatchJob {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	
	jobs := make([]*BatchJob, 0, len(bp.jobs))
	for _, job := range bp.jobs {
		jobs = append(jobs, job)
	}
	
	return jobs
}

// validateRequest validates a batch request
func (bp *BatchProcessor) validateRequest(request *BatchRequest) error {
	if len(request.Operations) == 0 {
		return ErrEmptyBatch
	}
	
	if len(request.Operations) > MaxBatchSize {
		return ErrBatchTooLarge
	}
	
	// Validate each item
	for _, item := range request.Operations {
		if err := bp.validateItem(item, request.Operation); err != nil {
			return err
		}
	}
	
	return nil
}

// validateItem validates a single batch item
func (bp *BatchProcessor) validateItem(item BatchItem, operation BatchOperationType) error {
	if item.Bucket == "" {
		return errors.New("bucket is required")
	}
	
	if item.Key == "" {
		return errors.New("key is required")
	}
	
	switch operation {
	case BatchCopy, BatchMove:
		if item.DestBucket == "" && item.DestKey == "" {
			return errors.New("dest_bucket or dest_key required for copy/move")
		}
	case BatchRestore:
		if item.VersionID == "" {
			return errors.New("version_id required for restore")
		}
	}
	
	return nil
}

// Helper to generate job ID
func generateJobID() string {
	return fmt.Sprintf("batch-%d", time.Now().UnixNano())
}

// Helper to create bytes reader
func newBytesReader(data []byte) io.Reader {
	return &bytesReader{data: data}
}

type bytesReader struct {
	data []byte
	pos  int
}

func (br *bytesReader) Read(p []byte) (n int, err error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n = copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}
