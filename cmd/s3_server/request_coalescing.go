// request_coalescing.go - Merge duplicate concurrent requests
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RequestCoalescer merges duplicate concurrent requests to reduce load
type RequestCoalescer struct {
	inflight map[string]*inflightRequest
	mu       sync.Mutex
	stats    CoalescingStats
}

type CoalescingStats struct {
	TotalRequests    uint64
	CoalescedRequests uint64
	SavedRequests    uint64
	AvgWaitTime      time.Duration
}

type inflightRequest struct {
	done     chan struct{}
	result   interface{}
	err      error
	waiters  int32
	startTime time.Time
}

// NewRequestCoalescer creates a new request coalescer
func NewRequestCoalescer() *RequestCoalescer {
	return &RequestCoalescer{
		inflight: make(map[string]*inflightRequest),
	}
}

// Do executes fn only once for duplicate concurrent calls with the same key
func (rc *RequestCoalescer) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	atomic.AddUint64(&rc.stats.TotalRequests, 1)

	rc.mu.Lock()

	// Check if request is already in flight
	if req, exists := rc.inflight[key]; exists {
		atomic.AddInt32(&req.waiters, 1)
		atomic.AddUint64(&rc.stats.CoalescedRequests, 1)
		atomic.AddUint64(&rc.stats.SavedRequests, 1)
		rc.mu.Unlock()

		// Wait for the in-flight request to complete
		<-req.done
		return req.result, req.err
	}

	// Create new in-flight request
	req := &inflightRequest{
		done:      make(chan struct{}),
		startTime: time.Now(),
	}
	rc.inflight[key] = req
	rc.mu.Unlock()

	// Execute the function
	req.result, req.err = fn()

	// Mark as done and cleanup
	rc.mu.Lock()
	delete(rc.inflight, key)
	rc.mu.Unlock()

	close(req.done)

	return req.result, req.err
}

// DoWithContext executes fn with context support
func (rc *RequestCoalescer) DoWithContext(ctx context.Context, key string, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	atomic.AddUint64(&rc.stats.TotalRequests, 1)

	rc.mu.Lock()

	// Check if request is already in flight
	if req, exists := rc.inflight[key]; exists {
		atomic.AddInt32(&req.waiters, 1)
		atomic.AddUint64(&rc.stats.CoalescedRequests, 1)
		atomic.AddUint64(&rc.stats.SavedRequests, 1)
		rc.mu.Unlock()

		// Wait for the in-flight request to complete or context to cancel
		select {
		case <-req.done:
			return req.result, req.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Create new in-flight request
	req := &inflightRequest{
		done:      make(chan struct{}),
		startTime: time.Now(),
	}
	rc.inflight[key] = req
	rc.mu.Unlock()

	// Execute the function
	req.result, req.err = fn(ctx)

	// Mark as done and cleanup
	rc.mu.Lock()
	delete(rc.inflight, key)
	rc.mu.Unlock()

	close(req.done)

	return req.result, req.err
}

// Stats returns coalescing statistics
func (rc *RequestCoalescer) Stats() CoalescingStats {
	return CoalescingStats{
		TotalRequests:     atomic.LoadUint64(&rc.stats.TotalRequests),
		CoalescedRequests: atomic.LoadUint64(&rc.stats.CoalescedRequests),
		SavedRequests:     atomic.LoadUint64(&rc.stats.SavedRequests),
	}
}

// Clear removes all in-flight requests (use with caution)
func (rc *RequestCoalescer) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.inflight = make(map[string]*inflightRequest)
}

// ObjectRequestCache combines caching with request coalescing
type ObjectRequestCache struct {
	cache     *CacheManager
	coalescer *RequestCoalescer
	stats     ObjectCacheStats
}

type ObjectCacheStats struct {
	CacheHits      uint64
	CacheMisses    uint64
	Coalescings    uint64
	BackendCalls   uint64
}

// NewObjectRequestCache creates an object request cache
func NewObjectRequestCache(cacheManager *CacheManager) *ObjectRequestCache {
	return &ObjectRequestCache{
		cache:     cacheManager,
		coalescer: NewRequestCoalescer(),
	}
}

// GetObjectData retrieves object data with caching and coalescing
func (orc *ObjectRequestCache) GetObjectData(
	ctx context.Context,
	bucket, key string,
	fetcher func(context.Context) ([]byte, error),
) ([]byte, error) {
	// Try cache first
	if data, ok := orc.cache.GetData(bucket, key); ok {
		atomic.AddUint64(&orc.stats.CacheHits, 1)
		return data, nil
	}

	atomic.AddUint64(&orc.stats.CacheMisses, 1)

	// Use request coalescing to fetch from backend
	cacheKey := fmt.Sprintf("obj:%s/%s", bucket, key)
	result, err := orc.coalescer.DoWithContext(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
		atomic.AddUint64(&orc.stats.BackendCalls, 1)
		return fetcher(ctx)
	})

	if err != nil {
		return nil, err
	}

	data := result.([]byte)

	// Store in cache for future requests
	orc.cache.SetData(bucket, key, data)

	return data, nil
}

// GetObjectMetadata retrieves object metadata with caching and coalescing
func (orc *ObjectRequestCache) GetObjectMetadata(
	ctx context.Context,
	bucket, key string,
	fetcher func(context.Context) (*ObjectMetadata, error),
) (*ObjectMetadata, error) {
	// Try cache first
	if metadata, ok := orc.cache.GetMetadata(bucket, key); ok {
		atomic.AddUint64(&orc.stats.CacheHits, 1)
		return metadata, nil
	}

	atomic.AddUint64(&orc.stats.CacheMisses, 1)

	// Use request coalescing to fetch from backend
	cacheKey := fmt.Sprintf("meta:%s/%s", bucket, key)
	result, err := orc.coalescer.DoWithContext(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
		atomic.AddUint64(&orc.stats.BackendCalls, 1)
		return fetcher(ctx)
	})

	if err != nil {
		return nil, err
	}

	metadata := result.(*ObjectMetadata)

	// Store in cache for future requests
	orc.cache.SetMetadata(bucket, key, metadata)

	return metadata, nil
}

// Invalidate removes cached data
func (orc *ObjectRequestCache) Invalidate(bucket, key string) {
	orc.cache.Invalidate(bucket, key)
}

// Stats returns cache statistics
func (orc *ObjectRequestCache) Stats() ObjectCacheStats {
	coalescingStats := orc.coalescer.Stats()
	return ObjectCacheStats{
		CacheHits:    atomic.LoadUint64(&orc.stats.CacheHits),
		CacheMisses:  atomic.LoadUint64(&orc.stats.CacheMisses),
		Coalescings:  coalescingStats.CoalescedRequests,
		BackendCalls: atomic.LoadUint64(&orc.stats.BackendCalls),
	}
}
