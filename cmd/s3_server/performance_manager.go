// performance_manager.go - Integrates all performance optimizations
package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// PerformanceManager coordinates all performance optimizations
type PerformanceManager struct {
	// Core components
	connPool          *ConnectionPool
	requestCoalescer  *RequestCoalescer
	rateLimiter       *AdaptiveRateLimiter
	perBucketLimiter  *PerBucketRateLimiter
	
	// Cache components
	cacheManager      *CacheManager
	objCache          *ObjectRequestCache
	queryCache        *QueryCache
	headCache         *HeadResultCache
	prefetchCache     *PrefetchCache
	
	// Buffer management
	bufferPool        *BufferPool
	bytesBufferPool   *BytesBufferPool
	
	// Compression
	compressionHandler *CompressionHandler
	
	// Statistics
	stats             PerformanceStats
	lastStatsUpdate   time.Time
	mu                sync.RWMutex
	
	// Configuration
	config            PerformanceConfig
}

type PerformanceConfig struct {
	// Cache settings
	EnableCache           bool
	MetadataCacheMB       int64
	DataCacheMB           int64
	MaxObjectCacheKB      int64
	CacheTTL              time.Duration
	
	// Query cache settings
	EnableQueryCache      bool
	QueryCacheMB          int64
	QueryCacheTTL         time.Duration
	
	// Prefetch settings
	EnablePrefetch        bool
	PrefetchWorkers       int
	
	// Rate limiting
	EnableRateLimiting    bool
	InitialRateLimit      int64
	MinRateLimit          int64
	MaxRateLimit          int64
	PerBucketLimiting     bool
	
	// Compression
	EnableCompression     bool
	CompressionMinSize    int64
	CompressionLevel      int
	
	// Connection pooling
	MaxIdleConnections    int
	IdleConnectionTimeout time.Duration
}

type PerformanceStats struct {
	// Cache stats
	MetadataCacheHitRate  float64
	DataCacheHitRate      float64
	QueryCacheHitRate     float64
	HeadCacheHitRate      float64
	
	// Request stats
	RequestsCoalesced     uint64
	RequestsSaved         uint64
	
	// Rate limiting stats
	RateLimitAccepted     uint64
	RateLimitRejected     uint64
	RateLimitAcceptRate   float64
	
	// Compression stats
	CompressionRatio      float64
	BytesSavedCompression uint64
	
	// Performance metrics
	AvgLatency            time.Duration
	P50Latency            time.Duration
	P95Latency            time.Duration
	P99Latency            time.Duration
	
	// System metrics
	MemoryUsageMB         uint64
	GoroutineCount        int
	HeapAllocMB           uint64
	
	// Throughput
	RequestsPerSecond     float64
	BytesPerSecond        float64
}

// DefaultPerformanceConfig returns sensible defaults
func DefaultPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		// Cache
		EnableCache:           true,
		MetadataCacheMB:       128,  // 128MB for metadata
		DataCacheMB:           512,  // 512MB for small objects
		MaxObjectCacheKB:      256,  // Cache objects up to 256KB
		CacheTTL:              5 * time.Minute,
		
		// Query cache
		EnableQueryCache:      true,
		QueryCacheMB:          64,   // 64MB for query results
		QueryCacheTTL:         2 * time.Minute,
		
		// Prefetch
		EnablePrefetch:        false, // Disabled by default
		PrefetchWorkers:       4,
		
		// Rate limiting
		EnableRateLimiting:    true,
		InitialRateLimit:      1000,  // 1000 req/s
		MinRateLimit:          100,   // 100 req/s min
		MaxRateLimit:          10000, // 10k req/s max
		PerBucketLimiting:     false,
		
		// Compression
		EnableCompression:     true,
		CompressionMinSize:    1024,      // 1KB minimum
		CompressionLevel:      gzip.BestSpeed, // Fast compression
		
		// Connection pooling
		MaxIdleConnections:    100,
		IdleConnectionTimeout: 90 * time.Second,
	}
}

// NewPerformanceManager creates a performance manager
func NewPerformanceManager(config PerformanceConfig) *PerformanceManager {
	pm := &PerformanceManager{
		config:           config,
		lastStatsUpdate:  time.Now(),
	}
	
	// Initialize connection pool
	pm.connPool = NewConnectionPool(
		config.MaxIdleConnections,
		config.IdleConnectionTimeout,
	)
	
	// Initialize request coalescer
	pm.requestCoalescer = NewRequestCoalescer()
	
	// Initialize rate limiters
	if config.EnableRateLimiting {
		pm.rateLimiter = NewAdaptiveRateLimiter(
			config.InitialRateLimit,
			config.MinRateLimit,
			config.MaxRateLimit,
		)
		
		if config.PerBucketLimiting {
			pm.perBucketLimiter = NewPerBucketRateLimiter(
				config.InitialRateLimit,
				config.MinRateLimit,
				config.MaxRateLimit,
			)
		}
	}
	
	// Initialize caches
	if config.EnableCache {
		pm.cacheManager = NewCacheManager(
			true,
			config.MetadataCacheMB,
			config.DataCacheMB,
			config.MaxObjectCacheKB,
			config.CacheTTL,
		)
		
		pm.objCache = NewObjectRequestCache(pm.cacheManager)
		pm.headCache = NewHeadResultCache(config.MetadataCacheMB/2, config.CacheTTL)
	}
	
	if config.EnableQueryCache {
		pm.queryCache = NewQueryCache(config.QueryCacheMB, config.QueryCacheTTL)
	}
	
	if config.EnablePrefetch && pm.cacheManager != nil {
		pm.prefetchCache = NewPrefetchCache(pm.cacheManager, config.PrefetchWorkers)
	}
	
	// Initialize buffer pools
	pm.bufferPool = GetGlobalBufferPool()
	pm.bytesBufferPool = GetGlobalBytesBufferPool()
	
	// Start stats collection
	go pm.collectStats()
	
	return pm
}

// CheckRateLimit checks if request should be allowed
func (pm *PerformanceManager) CheckRateLimit(bucket string) bool {
	if !pm.config.EnableRateLimiting {
		return true
	}
	
	if pm.config.PerBucketLimiting && pm.perBucketLimiter != nil {
		return pm.perBucketLimiter.Allow(bucket)
	}
	
	if pm.rateLimiter != nil {
		return pm.rateLimiter.Allow()
	}
	
	return true
}

// WaitRateLimit waits until request can proceed
func (pm *PerformanceManager) WaitRateLimit(ctx context.Context, bucket string) error {
	if !pm.config.EnableRateLimiting {
		return nil
	}
	
	if pm.config.PerBucketLimiting && pm.perBucketLimiter != nil {
		return pm.perBucketLimiter.Wait(ctx, bucket)
	}
	
	if pm.rateLimiter != nil {
		return pm.rateLimiter.Wait(ctx)
	}
	
	return nil
}

// GetObject retrieves an object with all optimizations
func (pm *PerformanceManager) GetObject(
	ctx context.Context,
	bucket, key string,
	fetcher func(context.Context) ([]byte, error),
) ([]byte, error) {
	// Check rate limit
	if !pm.CheckRateLimit(bucket) {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Record access pattern for prefetching
	if pm.prefetchCache != nil {
		pm.prefetchCache.RecordAccess(bucket, key)
	}
	
	// Use cache with coalescing
	if pm.objCache != nil {
		return pm.objCache.GetObjectData(ctx, bucket, key, fetcher)
	}
	
	// Fallback to direct fetch
	return fetcher(ctx)
}

// GetObjectMetadata retrieves metadata with caching
func (pm *PerformanceManager) GetObjectMetadata(
	ctx context.Context,
	bucket, key string,
	fetcher func(context.Context) (*ObjectMetadata, error),
) (*ObjectMetadata, error) {
	// Check rate limit
	if !pm.CheckRateLimit(bucket) {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Try HEAD cache first
	if pm.headCache != nil {
		if result, ok := pm.headCache.Get(bucket, key); ok {
			return &ObjectMetadata{
				Size:         result.Size,
				ETag:         result.ETag,
				ContentType:  result.ContentType,
				LastModified: result.LastModified,
				Exists:       result.Exists,
			}, nil
		}
	}
	
	// Use object cache with coalescing
	if pm.objCache != nil {
		return pm.objCache.GetObjectMetadata(ctx, bucket, key, fetcher)
	}
	
	// Fallback to direct fetch
	return fetcher(ctx)
}

// ListObjects retrieves list with query caching
func (pm *PerformanceManager) ListObjects(
	ctx context.Context,
	bucket, prefix, marker string,
	maxKeys int,
	fetcher func(context.Context) (*ListResult, error),
) (*ListResult, error) {
	// Check rate limit
	if !pm.CheckRateLimit(bucket) {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Use query cache if enabled
	if pm.queryCache != nil {
		return pm.queryCache.GetListResultWithFetch(ctx, bucket, prefix, marker, maxKeys, fetcher)
	}
	
	// Fallback to direct fetch
	return fetcher(ctx)
}

// InvalidateObject invalidates all caches for an object
func (pm *PerformanceManager) InvalidateObject(bucket, key string) {
	if pm.objCache != nil {
		pm.objCache.Invalidate(bucket, key)
	}
	
	if pm.headCache != nil {
		pm.headCache.Invalidate(bucket, key)
	}
	
	if pm.queryCache != nil {
		pm.queryCache.InvalidateBucket(bucket)
	}
}

// collectStats periodically collects performance statistics
func (pm *PerformanceManager) collectStats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		pm.updateStats()
	}
}

// updateStats updates performance statistics
func (pm *PerformanceManager) updateStats() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	stats := PerformanceStats{}
	
	// Cache stats
	if pm.cacheManager != nil {
		metaStats, dataStats := pm.cacheManager.Stats()
		
		if metaStats.Hits+metaStats.Misses > 0 {
			stats.MetadataCacheHitRate = float64(metaStats.Hits) / float64(metaStats.Hits+metaStats.Misses)
		}
		
		if dataStats.Hits+dataStats.Misses > 0 {
			stats.DataCacheHitRate = float64(dataStats.Hits) / float64(dataStats.Hits+dataStats.Misses)
		}
	}
	
	if pm.queryCache != nil {
		qStats := pm.queryCache.Stats()
		if qStats.Hits+qStats.Misses > 0 {
			stats.QueryCacheHitRate = float64(qStats.Hits) / float64(qStats.Hits+qStats.Misses)
		}
	}
	
	if pm.headCache != nil {
		hStats := pm.headCache.Stats()
		if hStats.Hits+hStats.Misses > 0 {
			stats.HeadCacheHitRate = float64(hStats.Hits) / float64(hStats.Hits+hStats.Misses)
		}
	}
	
	// Request coalescing stats
	if pm.requestCoalescer != nil {
		cStats := pm.requestCoalescer.Stats()
		stats.RequestsCoalesced = cStats.CoalescedRequests
		stats.RequestsSaved = cStats.SavedRequests
	}
	
	// Rate limiting stats
	if pm.rateLimiter != nil {
		rlStats := pm.rateLimiter.Stats()
		stats.RateLimitAccepted = rlStats.Allowed
		stats.RateLimitRejected = rlStats.Rejected
		stats.RateLimitAcceptRate = rlStats.AcceptRate
	}
	
	// Compression stats
	if pm.compressionHandler != nil {
		compStats := pm.compressionHandler.Stats()
		stats.CompressionRatio = compStats.CompressionRatio
		stats.BytesSavedCompression = compStats.BytesIn - compStats.BytesOut
	}
	
	// System metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats.MemoryUsageMB = m.Sys / 1024 / 1024
	stats.HeapAllocMB = m.HeapAlloc / 1024 / 1024
	stats.GoroutineCount = runtime.NumGoroutine()
	
	pm.stats = stats
	pm.lastStatsUpdate = time.Now()
}

// Stats returns current performance statistics
func (pm *PerformanceManager) Stats() PerformanceStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.stats
}

// GetConnectionPool returns the connection pool
func (pm *PerformanceManager) GetConnectionPool() *ConnectionPool {
	return pm.connPool
}

// GetBufferPool returns the buffer pool
func (pm *PerformanceManager) GetBufferPool() *BufferPool {
	return pm.bufferPool
}

// GetBytesBufferPool returns the bytes buffer pool
func (pm *PerformanceManager) GetBytesBufferPool() *BytesBufferPool {
	return pm.bytesBufferPool
}

// Close shuts down the performance manager
func (pm *PerformanceManager) Close() {
	if pm.connPool != nil {
		pm.connPool.Close()
	}
	
	if pm.prefetchCache != nil {
		pm.prefetchCache.Close()
	}
}

// OptimizeForWorkload adjusts settings based on workload type
func (pm *PerformanceManager) OptimizeForWorkload(workloadType string) {
	switch workloadType {
	case "read-heavy":
		// Increase cache sizes
		if pm.rateLimiter != nil {
			pm.rateLimiter.AdaptUp(0.2)
		}
		
	case "write-heavy":
		// Reduce cache TTL, increase buffer sizes
		if pm.rateLimiter != nil {
			pm.rateLimiter.AdaptUp(0.1)
		}
		
	case "mixed":
		// Balanced settings (already configured)
		
	case "bursty":
		// Increase rate limits, enable more coalescing
		if pm.rateLimiter != nil {
			pm.rateLimiter.AdaptUp(0.3)
		}
	}
}

// HealthCheck performs a health check of all components
func (pm *PerformanceManager) HealthCheck() map[string]bool {
	health := make(map[string]bool)
	
	health["connection_pool"] = pm.connPool != nil
	health["rate_limiter"] = pm.rateLimiter != nil
	health["cache_manager"] = pm.cacheManager != nil
	health["query_cache"] = pm.queryCache != nil
	health["buffer_pool"] = pm.bufferPool != nil
	
	return health
}
