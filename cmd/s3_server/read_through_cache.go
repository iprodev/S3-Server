// read_through_cache.go - Read-through caching layer for S3 operations
package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

// ReadThroughCache implements read-through caching for object data
type ReadThroughCache struct {
	cache          *CacheManager
	bufferPool     *BufferPool
	mu             sync.RWMutex
	inFlightLoads  map[string]*sync.WaitGroup
	loadingMu      sync.Mutex
}

// NewReadThroughCache creates a new read-through cache
func NewReadThroughCache(cache *CacheManager, bufferPool *BufferPool) *ReadThroughCache {
	return &ReadThroughCache{
		cache:         cache,
		bufferPool:    bufferPool,
		inFlightLoads: make(map[string]*sync.WaitGroup),
	}
}

// GetWithLoader attempts to get data from cache, loading on miss
func (rtc *ReadThroughCache) GetWithLoader(
	ctx context.Context,
	bucket, key string,
	loader func() ([]byte, error),
) ([]byte, error) {
	cacheKey := bucket + "/" + key

	// Try cache first
	if data, ok := rtc.cache.GetData(bucket, key); ok {
		return data, nil
	}

	// Check if another goroutine is already loading this key
	rtc.loadingMu.Lock()
	wg, loading := rtc.inFlightLoads[cacheKey]
	if loading {
		// Wait for the in-flight load to complete
		rtc.loadingMu.Unlock()
		wg.Wait()
		
		// Try cache again after load completes
		if data, ok := rtc.cache.GetData(bucket, key); ok {
			return data, nil
		}
		
		// If still not in cache, fall through to load
	} else {
		// We're the first to load this key
		wg = &sync.WaitGroup{}
		wg.Add(1)
		rtc.inFlightLoads[cacheKey] = wg
		rtc.loadingMu.Unlock()
		
		defer func() {
			rtc.loadingMu.Lock()
			delete(rtc.inFlightLoads, cacheKey)
			rtc.loadingMu.Unlock()
			wg.Done()
		}()
	}

	// Load from backend
	data, err := loader()
	if err != nil {
		return nil, err
	}

	// Store in cache
	rtc.cache.SetData(bucket, key, data)

	return data, nil
}

// AdaptivePrefetcher predicts and prefetches objects
// Using ReadThrough prefix to avoid conflicts with query_cache.go
type AdaptivePrefetcher struct {
	cache          *ReadThroughCache
	accessPattern  *ReadThroughAccessPattern
	prefetchQueue  chan ReadThroughPrefetchRequest
	workers        int
	mu             sync.RWMutex
}

type ReadThroughPrefetchRequest struct {
	Bucket string
	Key    string
	Loader func() ([]byte, error)
}

type ReadThroughAccessPattern struct {
	mu              sync.RWMutex
	sequentialReads map[string][]string // bucket -> recent keys
	maxHistory      int
}

func NewReadThroughAccessPattern(maxHistory int) *ReadThroughAccessPattern {
	return &ReadThroughAccessPattern{
		sequentialReads: make(map[string][]string),
		maxHistory:      maxHistory,
	}
}

func (ap *ReadThroughAccessPattern) RecordAccess(bucket, key string) []string {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	// Add to history
	history := ap.sequentialReads[bucket]
	history = append(history, key)
	
	// Keep only recent history
	if len(history) > ap.maxHistory {
		history = history[len(history)-ap.maxHistory:]
	}
	
	ap.sequentialReads[bucket] = history

	// Predict next keys (simple sequential pattern)
	if len(history) >= 2 {
		return ap.predictNext(history)
	}
	
	return nil
}

func (ap *ReadThroughAccessPattern) predictNext(history []string) []string {
	// Simple pattern: if accessing file001, file002, predict file003
	// This is a placeholder for more sophisticated ML-based prediction
	
	if len(history) < 2 {
		return nil
	}
	
	// For now, just return empty - can be enhanced with pattern matching
	return nil
}

func NewAdaptivePrefetcher(cache *ReadThroughCache, workers int) *AdaptivePrefetcher {
	ap := &AdaptivePrefetcher{
		cache:         cache,
		accessPattern: NewReadThroughAccessPattern(100),
		prefetchQueue: make(chan ReadThroughPrefetchRequest, 1000),
		workers:       workers,
	}
	
	// Start prefetch workers
	for i := 0; i < workers; i++ {
		go ap.prefetchWorker()
	}
	
	return ap
}

func (ap *AdaptivePrefetcher) prefetchWorker() {
	for req := range ap.prefetchQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = ap.cache.GetWithLoader(ctx, req.Bucket, req.Key, req.Loader)
		cancel()
	}
}

func (ap *AdaptivePrefetcher) RecordAccess(bucket, key string) {
	predictions := ap.accessPattern.RecordAccess(bucket, key)
	
	// Queue predicted keys for prefetch
	for _, predictedKey := range predictions {
		select {
		case ap.prefetchQueue <- ReadThroughPrefetchRequest{
			Bucket: bucket,
			Key:    predictedKey,
		}:
		default:
			// Queue full, skip
		}
	}
}

// Smart cache warmer for frequently accessed objects
type CacheWarmer struct {
	cache      *CacheManager
	heatMap    *HeatMap
	warmerFunc func(bucket, key string) ([]byte, error)
	mu         sync.RWMutex
}

type HeatMap struct {
	mu          sync.RWMutex
	accessCount map[string]uint64
	lastAccess  map[string]time.Time
}

func NewHeatMap() *HeatMap {
	return &HeatMap{
		accessCount: make(map[string]uint64),
		lastAccess:  make(map[string]time.Time),
	}
}

func (hm *HeatMap) RecordAccess(bucket, key string) {
	cacheKey := bucket + "/" + key
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	hm.accessCount[cacheKey]++
	hm.lastAccess[cacheKey] = time.Now()
}

func (hm *HeatMap) GetHotKeys(minAccess uint64, recency time.Duration) []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	now := time.Now()
	var hotKeys []string
	
	for key, count := range hm.accessCount {
		if count >= minAccess {
			if lastAccess, ok := hm.lastAccess[key]; ok {
				if now.Sub(lastAccess) <= recency {
					hotKeys = append(hotKeys, key)
				}
			}
		}
	}
	
	return hotKeys
}

func (hm *HeatMap) Cleanup(olderThan time.Duration) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	now := time.Now()
	for key, lastAccess := range hm.lastAccess {
		if now.Sub(lastAccess) > olderThan {
			delete(hm.accessCount, key)
			delete(hm.lastAccess, key)
		}
	}
}

func NewCacheWarmer(cache *CacheManager, warmerFunc func(bucket, key string) ([]byte, error)) *CacheWarmer {
	cw := &CacheWarmer{
		cache:      cache,
		heatMap:    NewHeatMap(),
		warmerFunc: warmerFunc,
	}
	
	// Start periodic warming
	go cw.periodicWarmup()
	
	// Start periodic cleanup
	go cw.periodicCleanup()
	
	return cw
}

func (cw *CacheWarmer) RecordAccess(bucket, key string) {
	cw.heatMap.RecordAccess(bucket, key)
}

func (cw *CacheWarmer) periodicWarmup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		cw.warmupHotKeys()
	}
}

func (cw *CacheWarmer) warmupHotKeys() {
	// Get keys accessed at least 10 times in last 15 minutes
	hotKeys := cw.heatMap.GetHotKeys(10, 15*time.Minute)
	
	for _, cacheKey := range hotKeys {
		// Parse bucket/key
		bucket, key := parseCacheKey(cacheKey)
		
		// Check if already in cache
		if _, ok := cw.cache.GetData(bucket, key); ok {
			continue
		}
		
		// Warm up cache
		if cw.warmerFunc != nil {
			if data, err := cw.warmerFunc(bucket, key); err == nil {
				cw.cache.SetData(bucket, key, data)
			}
		}
	}
}

func (cw *CacheWarmer) periodicCleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		cw.heatMap.Cleanup(1 * time.Hour)
	}
}

func parseCacheKey(cacheKey string) (bucket, key string) {
	// Simple split on first /
	for i, c := range cacheKey {
		if c == '/' {
			return cacheKey[:i], cacheKey[i+1:]
		}
	}
	return cacheKey, ""
}

// CompressionCache caches compressed versions of objects
type CompressionCache struct {
	cache      *FastCache
	bufferPool *BufferPool
}

func NewCompressionCache(maxSizeMB int64, ttl time.Duration, bufferPool *BufferPool) *CompressionCache {
	return &CompressionCache{
		cache:      NewFastCache(maxSizeMB*1024*1024, ttl),
		bufferPool: bufferPool,
	}
}

// ETagCache caches ETags to avoid recomputation
type ETagCache struct {
	cache *FastCache
}

func NewETagCache(maxEntries int64, ttl time.Duration) *ETagCache {
	// Assume each ETag entry is ~100 bytes
	maxSize := maxEntries * 100
	return &ETagCache{
		cache: NewFastCache(maxSize, ttl),
	}
}

func (ec *ETagCache) Get(bucket, key string) (string, bool) {
	cacheKey := bucket + "/" + key
	if val, ok := ec.cache.Get(cacheKey); ok {
		return val.(string), true
	}
	return "", false
}

func (ec *ETagCache) Set(bucket, key, etag string) {
	cacheKey := bucket + "/" + key
	ec.cache.Set(cacheKey, etag, 100)
}

// ComputeETag computes MD5 hash for ETag
func ComputeETag(data []byte) string {
	hash := md5.Sum(data)
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))
}

// StreamingETag computes ETag from a reader without loading into memory
func StreamingETag(r io.Reader) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, r); err != nil {
		return "", err
	}
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(hash.Sum(nil))), nil
}
