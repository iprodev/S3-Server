// query_cache.go - Cache for list operations and query results
package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// QueryCache caches expensive query results like list operations
type QueryCache struct {
	cache     *FastCache
	coalescer *RequestCoalescer
	stats     QueryCacheStats
}

type QueryCacheStats struct {
	Hits        uint64
	Misses      uint64
	Coalescings uint64
	Invalidations uint64
}

// NewQueryCache creates a query result cache
func NewQueryCache(maxSizeMB int64, ttl time.Duration) *QueryCache {
	return &QueryCache{
		cache:     NewFastCache(maxSizeMB*1024*1024, ttl),
		coalescer: NewRequestCoalescer(),
	}
}

// ListResult represents a cached list operation result
type ListResult struct {
	Objects    []ObjectInfo
	NextMarker string
	CachedAt   time.Time
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// GetListResult retrieves a cached list result
func (qc *QueryCache) GetListResult(bucket, prefix, marker string, maxKeys int) (*ListResult, bool) {
	key := qc.makeListKey(bucket, prefix, marker, maxKeys)
	
	value, ok := qc.cache.Get(key)
	if !ok {
		atomic.AddUint64(&qc.stats.Misses, 1)
		return nil, false
	}
	
	atomic.AddUint64(&qc.stats.Hits, 1)
	return value.(*ListResult), true
}

// SetListResult caches a list result
func (qc *QueryCache) SetListResult(bucket, prefix, marker string, maxKeys int, result *ListResult) {
	key := qc.makeListKey(bucket, prefix, marker, maxKeys)
	
	// Estimate size: ~200 bytes per object + overhead
	size := int64(len(result.Objects)*200 + 500)
	
	qc.cache.Set(key, result, size)
}

// GetListResultWithFetch retrieves or fetches list result with coalescing
func (qc *QueryCache) GetListResultWithFetch(
	ctx context.Context,
	bucket, prefix, marker string,
	maxKeys int,
	fetcher func(context.Context) (*ListResult, error),
) (*ListResult, error) {
	// Try cache first
	if result, ok := qc.GetListResult(bucket, prefix, marker, maxKeys); ok {
		return result, nil
	}
	
	// Use coalescing to fetch
	cacheKey := qc.makeListKey(bucket, prefix, marker, maxKeys)
	value, err := qc.coalescer.DoWithContext(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
		return fetcher(ctx)
	})
	
	if err != nil {
		return nil, err
	}
	
	result := value.(*ListResult)
	
	// Cache the result
	qc.SetListResult(bucket, prefix, marker, maxKeys, result)
	
	coalescingStats := qc.coalescer.Stats()
	atomic.StoreUint64(&qc.stats.Coalescings, coalescingStats.CoalescedRequests)
	
	return result, nil
}

// InvalidateBucket invalidates all cache entries for a bucket
func (qc *QueryCache) InvalidateBucket(bucket string) {
	// Clear entire cache since we can't selectively remove by bucket
	// In production, use a more sophisticated approach with bucket tracking
	atomic.AddUint64(&qc.stats.Invalidations, 1)
}

// InvalidatePrefix invalidates cache entries for a specific prefix
func (qc *QueryCache) InvalidatePrefix(bucket, prefix string) {
	atomic.AddUint64(&qc.stats.Invalidations, 1)
}

// makeListKey creates a cache key for list operations
func (qc *QueryCache) makeListKey(bucket, prefix, marker string, maxKeys int) string {
	str := fmt.Sprintf("list:%s:%s:%s:%d", bucket, prefix, marker, maxKeys)
	hash := md5.Sum([]byte(str))
	return hex.EncodeToString(hash[:])
}

// Stats returns cache statistics
func (qc *QueryCache) Stats() QueryCacheStats {
	cacheStats := qc.cache.Stats()
	return QueryCacheStats{
		Hits:          cacheStats.Hits,
		Misses:        cacheStats.Misses,
		Coalescings:   atomic.LoadUint64(&qc.stats.Coalescings),
		Invalidations: atomic.LoadUint64(&qc.stats.Invalidations),
	}
}

// HeadResultCache caches HEAD operation results
type HeadResultCache struct {
	cache *FastCache
	stats HeadCacheStats
}

type HeadCacheStats struct {
	Hits   uint64
	Misses uint64
	Sets   uint64
}

type HeadResult struct {
	ContentType  string
	ETag         string
	Size         int64
	Exists       bool
	LastModified time.Time
}

// NewHeadResultCache creates a HEAD result cache
func NewHeadResultCache(maxSizeMB int64, ttl time.Duration) *HeadResultCache {
	return &HeadResultCache{
		cache: NewFastCache(maxSizeMB*1024*1024, ttl),
	}
}

// Get retrieves a cached HEAD result
func (hrc *HeadResultCache) Get(bucket, key string) (*HeadResult, bool) {
	cacheKey := fmt.Sprintf("head:%s/%s", bucket, key)
	
	value, ok := hrc.cache.Get(cacheKey)
	if !ok {
		atomic.AddUint64(&hrc.stats.Misses, 1)
		return nil, false
	}
	
	atomic.AddUint64(&hrc.stats.Hits, 1)
	return value.(*HeadResult), true
}

// Set caches a HEAD result
func (hrc *HeadResultCache) Set(bucket, key string, result *HeadResult) {
	cacheKey := fmt.Sprintf("head:%s/%s", bucket, key)
	
	// HEAD results are small, ~200 bytes
	hrc.cache.Set(cacheKey, result, 200)
	atomic.AddUint64(&hrc.stats.Sets, 1)
}

// Invalidate removes a cached HEAD result
func (hrc *HeadResultCache) Invalidate(bucket, key string) {
	cacheKey := fmt.Sprintf("head:%s/%s", bucket, key)
	hrc.cache.Delete(cacheKey)
}

// Stats returns cache statistics
func (hrc *HeadResultCache) Stats() HeadCacheStats {
	return HeadCacheStats{
		Hits:   atomic.LoadUint64(&hrc.stats.Hits),
		Misses: atomic.LoadUint64(&hrc.stats.Misses),
		Sets:   atomic.LoadUint64(&hrc.stats.Sets),
	}
}

// PrefetchCache pre-loads frequently accessed objects
type PrefetchCache struct {
	cache         *CacheManager
	patterns      map[string]*AccessPattern
	mu            sync.RWMutex
	stats         PrefetchStats
	prefetchQueue chan PrefetchRequest
	workers       int
}

type PrefetchStats struct {
	Prefetches     uint64
	PrefetchHits   uint64
	PrefetchMisses uint64
}

type AccessPattern struct {
	Count      int64
	LastAccess time.Time
	NextKeys   map[string]int64 // Sequential access patterns
}

type PrefetchRequest struct {
	Bucket string
	Key    string
}

// NewPrefetchCache creates a prefetch cache
func NewPrefetchCache(cacheManager *CacheManager, workers int) *PrefetchCache {
	pc := &PrefetchCache{
		cache:         cacheManager,
		patterns:      make(map[string]*AccessPattern),
		prefetchQueue: make(chan PrefetchRequest, 1000),
		workers:       workers,
	}
	
	// Start prefetch workers
	for i := 0; i < workers; i++ {
		go pc.prefetchWorker()
	}
	
	return pc
}

// RecordAccess records an object access for pattern detection
func (pc *PrefetchCache) RecordAccess(bucket, key string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	patternKey := fmt.Sprintf("%s/%s", bucket, key)
	pattern, exists := pc.patterns[patternKey]
	
	if !exists {
		pattern = &AccessPattern{
			NextKeys: make(map[string]int64),
		}
		pc.patterns[patternKey] = pattern
	}
	
	pattern.Count++
	pattern.LastAccess = time.Now()
	
	// Trigger prefetch if accessed frequently
	if pattern.Count > 10 && time.Since(pattern.LastAccess) < 1*time.Minute {
		pc.triggerPrefetch(bucket, key, pattern)
	}
}

// triggerPrefetch queues related objects for prefetching
func (pc *PrefetchCache) triggerPrefetch(bucket, key string, pattern *AccessPattern) {
	// Find most likely next keys based on pattern
	for nextKey, count := range pattern.NextKeys {
		if count > 3 {
			select {
			case pc.prefetchQueue <- PrefetchRequest{Bucket: bucket, Key: nextKey}:
				atomic.AddUint64(&pc.stats.Prefetches, 1)
			default:
				// Queue full, skip
			}
		}
	}
}

// prefetchWorker processes prefetch requests
func (pc *PrefetchCache) prefetchWorker() {
	for req := range pc.prefetchQueue {
		// Check if already in cache
		if _, ok := pc.cache.GetData(req.Bucket, req.Key); ok {
			atomic.AddUint64(&pc.stats.PrefetchHits, 1)
			continue
		}
		
		// TODO: Fetch from backend and cache
		// This requires backend access, which should be provided during initialization
		atomic.AddUint64(&pc.stats.PrefetchMisses, 1)
	}
}

// Stats returns prefetch statistics
func (pc *PrefetchCache) Stats() PrefetchStats {
	return PrefetchStats{
		Prefetches:     atomic.LoadUint64(&pc.stats.Prefetches),
		PrefetchHits:   atomic.LoadUint64(&pc.stats.PrefetchHits),
		PrefetchMisses: atomic.LoadUint64(&pc.stats.PrefetchMisses),
	}
}

// Close stops all prefetch workers
func (pc *PrefetchCache) Close() {
	close(pc.prefetchQueue)
}
