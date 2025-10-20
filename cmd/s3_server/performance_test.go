// performance_test.go - Tests for performance optimizations
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test ConnectionPool
func TestConnectionPool(t *testing.T) {
	pool := NewConnectionPool(10, 60*time.Second)
	
	// Get client for same host multiple times
	client1 := pool.GetClient("http://localhost:9001")
	client2 := pool.GetClient("http://localhost:9001")
	
	if client1 != client2 {
		t.Error("Expected same client instance for same host")
	}
	
	// Check stats
	stats := pool.Stats()
	if stats.Created < 1 {
		t.Error("Expected at least one client created")
	}
	if stats.Reused < 1 {
		t.Error("Expected at least one client reused")
	}
	
	pool.Close()
}

// Test RequestCoalescer
func TestRequestCoalescer(t *testing.T) {
	coalescer := NewRequestCoalescer()
	
	callCount := int32(0)
	var wg sync.WaitGroup
	
	// Launch 10 concurrent requests with same key
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			result, err := coalescer.Do("test-key", func() (interface{}, error) {
				atomic.AddInt32(&callCount, 1)
				time.Sleep(50 * time.Millisecond) // Simulate work
				return "result", nil
			})
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != "result" {
				t.Errorf("Expected 'result', got %v", result)
			}
		}()
	}
	
	wg.Wait()
	
	// Should only call function once despite 10 requests
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
	
	stats := coalescer.Stats()
	if stats.CoalescedRequests < 9 {
		t.Error("Expected at least 9 coalesced requests")
	}
}

// Test RequestCoalescer with context
func TestRequestCoalescerWithContext(t *testing.T) {
	coalescer := NewRequestCoalescer()
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// Test context cancellation
	_, err := coalescer.DoWithContext(ctx, "slow-key", func(ctx context.Context) (interface{}, error) {
		time.Sleep(200 * time.Millisecond) // Longer than context timeout
		return "result", nil
	})
	
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}

// Test AdaptiveRateLimiter
func TestAdaptiveRateLimiter(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(10, 5, 50) // 10 req/s initial
	
	// Allow some requests
	allowed := 0
	for i := 0; i < 20; i++ {
		if limiter.Allow() {
			allowed++
		}
	}
	
	// Should allow approximately 10 requests initially
	if allowed < 5 || allowed > 15 {
		t.Errorf("Expected ~10 allowed requests, got %d", allowed)
	}
	
	stats := limiter.Stats()
	if stats.Allowed == 0 {
		t.Error("Expected some allowed requests")
	}
}

// Test AdaptiveRateLimiter adaptation
func TestRateLimiterAdaptation(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(100, 50, 200)
	
	initialStats := limiter.Stats()
	initialMax := initialStats.MaxTokens
	
	// Adapt up
	limiter.AdaptUp(0.5) // Increase by 50%
	
	upStats := limiter.Stats()
	if upStats.MaxTokens <= initialMax {
		t.Error("Expected max tokens to increase after adapt up")
	}
	
	// Adapt down
	limiter.AdaptDown(0.3) // Decrease by 30%
	
	downStats := limiter.Stats()
	if downStats.MaxTokens >= upStats.MaxTokens {
		t.Error("Expected max tokens to decrease after adapt down")
	}
}

// Test QueryCache
func TestQueryCache(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)
	
	result := &ListResult{
		Objects: []ObjectInfo{
			{Key: "test1.txt", Size: 100},
			{Key: "test2.txt", Size: 200},
		},
		NextMarker: "",
		CachedAt:   time.Now(),
	}
	
	// Set result
	cache.SetListResult("bucket1", "prefix/", "", 100, result)
	
	// Get result
	retrieved, ok := cache.GetListResult("bucket1", "prefix/", "", 100)
	if !ok {
		t.Error("Expected cache hit")
	}
	
	if len(retrieved.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(retrieved.Objects))
	}
	
	stats := cache.Stats()
	if stats.Hits == 0 {
		t.Error("Expected at least one cache hit")
	}
}

// Test QueryCache with coalescing
func TestQueryCacheWithCoalescing(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)
	
	callCount := int32(0)
	var wg sync.WaitGroup
	
	// Launch multiple concurrent list requests
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ctx := context.Background()
			_, err := cache.GetListResultWithFetch(ctx, "bucket1", "prefix/", "", 100,
				func(ctx context.Context) (*ListResult, error) {
					atomic.AddInt32(&callCount, 1)
					time.Sleep(50 * time.Millisecond)
					return &ListResult{
						Objects: []ObjectInfo{{Key: "test.txt", Size: 100}},
					}, nil
				})
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}
	
	wg.Wait()
	
	// Should only fetch once due to coalescing
	if callCount != 1 {
		t.Errorf("Expected 1 backend call, got %d", callCount)
	}
}

// Test HeadResultCache
func TestHeadResultCache(t *testing.T) {
	cache := NewHeadResultCache(10, 5*time.Minute)
	
	result := &HeadResult{
		ContentType:  "text/plain",
		ETag:         "abc123",
		Size:         1024,
		Exists:       true,
		LastModified: time.Now(),
	}
	
	// Set result
	cache.Set("bucket1", "test.txt", result)
	
	// Get result
	retrieved, ok := cache.Get("bucket1", "test.txt")
	if !ok {
		t.Error("Expected cache hit")
	}
	
	if retrieved.ETag != "abc123" {
		t.Errorf("Expected ETag 'abc123', got %s", retrieved.ETag)
	}
	
	stats := cache.Stats()
	if stats.Hits == 0 || stats.Sets == 0 {
		t.Error("Expected cache activity")
	}
}

// Test ObjectRequestCache
func TestObjectRequestCache(t *testing.T) {
	cacheManager := NewCacheManager(true, 10, 10, 256, 5*time.Minute)
	objCache := NewObjectRequestCache(cacheManager)
	
	fetchCount := int32(0)
	
	ctx := context.Background()
	
	// First fetch - cache miss
	data1, err := objCache.GetObjectData(ctx, "bucket1", "test.txt",
		func(ctx context.Context) ([]byte, error) {
			atomic.AddInt32(&fetchCount, 1)
			return []byte("test data"), nil
		})
	
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if string(data1) != "test data" {
		t.Errorf("Expected 'test data', got %s", string(data1))
	}
	
	// Second fetch - cache hit
	data2, err := objCache.GetObjectData(ctx, "bucket1", "test.txt",
		func(ctx context.Context) ([]byte, error) {
			atomic.AddInt32(&fetchCount, 1)
			return []byte("test data"), nil
		})
	
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if string(data2) != "test data" {
		t.Errorf("Expected 'test data', got %s", string(data2))
	}
	
	// Should only fetch once
	if fetchCount != 1 {
		t.Errorf("Expected 1 fetch, got %d", fetchCount)
	}
	
	stats := objCache.Stats()
	if stats.CacheHits == 0 {
		t.Error("Expected at least one cache hit")
	}
}

// Test PerformanceManager initialization
func TestPerformanceManagerInit(t *testing.T) {
	config := DefaultPerformanceConfig()
	pm := NewPerformanceManager(config)
	defer pm.Close()
	
	if pm.connPool == nil {
		t.Error("Connection pool not initialized")
	}
	
	if pm.requestCoalescer == nil {
		t.Error("Request coalescer not initialized")
	}
	
	if pm.cacheManager == nil {
		t.Error("Cache manager not initialized")
	}
	
	health := pm.HealthCheck()
	if !health["connection_pool"] {
		t.Error("Connection pool health check failed")
	}
}

// Test PerformanceManager rate limiting
func TestPerformanceManagerRateLimit(t *testing.T) {
	config := DefaultPerformanceConfig()
	config.InitialRateLimit = 10
	pm := NewPerformanceManager(config)
	defer pm.Close()
	
	allowed := 0
	for i := 0; i < 20; i++ {
		if pm.CheckRateLimit("bucket1") {
			allowed++
		}
	}
	
	// Should rate limit after initial tokens
	if allowed >= 20 {
		t.Error("Expected rate limiting to reject some requests")
	}
}

// Test PerformanceManager GetObject
func TestPerformanceManagerGetObject(t *testing.T) {
	config := DefaultPerformanceConfig()
	pm := NewPerformanceManager(config)
	defer pm.Close()
	
	ctx := context.Background()
	fetchCount := int32(0)
	
	fetcher := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&fetchCount, 1)
		return []byte("object data"), nil
	}
	
	// First get - cache miss
	data1, err := pm.GetObject(ctx, "bucket1", "test.txt", fetcher)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if string(data1) != "object data" {
		t.Errorf("Expected 'object data', got %s", string(data1))
	}
	
	// Second get - cache hit
	data2, err := pm.GetObject(ctx, "bucket1", "test.txt", fetcher)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if string(data2) != "object data" {
		t.Errorf("Expected 'object data', got %s", string(data2))
	}
	
	// Should only fetch once due to caching
	if fetchCount != 1 {
		t.Errorf("Expected 1 fetch, got %d", fetchCount)
	}
	
	// Check stats
	stats := pm.Stats()
	if stats.DataCacheHitRate == 0 {
		t.Error("Expected some cache hits")
	}
}

// Test compression
func TestCompressBuffer(t *testing.T) {
	data := []byte(strings.Repeat("test data ", 1000))
	
	compressed, err := CompressBuffer(data, 6)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	
	if len(compressed) >= len(data) {
		t.Error("Compressed data should be smaller")
	}
	
	decompressed, err := DecompressBuffer(compressed)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}
	
	if string(decompressed) != string(data) {
		t.Error("Decompressed data doesn't match original")
	}
}

// Test StreamCompressor
func TestStreamCompressor(t *testing.T) {
	data := []byte(strings.Repeat("test data ", 1000))
	reader := strings.NewReader(string(data))
	
	compressor := NewStreamCompressor(reader, 6)
	defer compressor.Close()
	
	compressed, err := io.ReadAll(compressor)
	if err != nil {
		t.Fatalf("Failed to read compressed stream: %v", err)
	}
	
	if len(compressed) >= len(data) {
		t.Error("Compressed stream should be smaller")
	}
}

// Benchmark FastCache
func BenchmarkFastCacheSet(b *testing.B) {
	cache := NewFastCache(1024*1024*100, 5*time.Minute) // 100MB
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%10000)
		cache.Set(key, []byte("value"), 100)
	}
}

func BenchmarkFastCacheGet(b *testing.B) {
	cache := NewFastCache(1024*1024*100, 5*time.Minute) // 100MB
	
	// Pre-populate
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, []byte("value"), 100)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%10000)
		cache.Get(key)
	}
}

// Benchmark RequestCoalescer
func BenchmarkRequestCoalescer(b *testing.B) {
	coalescer := NewRequestCoalescer()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			coalescer.Do(key, func() (interface{}, error) {
				return "result", nil
			})
			i++
		}
	})
}

// Benchmark BufferPool
func BenchmarkBufferPoolGetPut(b *testing.B) {
	pool := NewBufferPool()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(4096)
		pool.Put(buf)
	}
}

// Benchmark compression
func BenchmarkCompressionGzip(b *testing.B) {
	data := []byte(strings.Repeat("test data ", 1000))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompressBuffer(data, 6)
	}
}
