package main

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// CacheEntry represents a cached item
type CacheEntry struct {
	Key        string
	Value      interface{}
	Size       int64
	ExpiresAt  time.Time
	AccessedAt time.Time
	element    *list.Element
}

// FastCache is a high-performance in-memory cache with LRU eviction
type FastCache struct {
	maxSize    int64
	currentSize int64
	ttl        time.Duration
	
	mu      sync.RWMutex
	items   map[string]*CacheEntry
	lruList *list.List
	
	// Statistics
	hits   uint64
	misses uint64
	evictions uint64
	sets   uint64
}

// NewFastCache creates a new cache with specified max size and TTL
func NewFastCache(maxSizeBytes int64, ttl time.Duration) *FastCache {
	fc := &FastCache{
		maxSize: maxSizeBytes,
		ttl:     ttl,
		items:   make(map[string]*CacheEntry),
		lruList: list.New(),
	}
	
	// Start cleanup goroutine
	go fc.cleanupLoop()
	
	return fc
}

// Get retrieves an item from cache
func (fc *FastCache) Get(key string) (interface{}, bool) {
	fc.mu.RLock()
	entry, exists := fc.items[key]
	fc.mu.RUnlock()
	
	if !exists {
		atomic.AddUint64(&fc.misses, 1)
		return nil, false
	}
	
	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		atomic.AddUint64(&fc.misses, 1)
		fc.Delete(key)
		return nil, false
	}
	
	// Update access time and move to front (hot item)
	fc.mu.Lock()
	entry.AccessedAt = time.Now()
	fc.lruList.MoveToFront(entry.element)
	fc.mu.Unlock()
	
	atomic.AddUint64(&fc.hits, 1)
	return entry.Value, true
}

// Set adds an item to cache
func (fc *FastCache) Set(key string, value interface{}, size int64) {
	atomic.AddUint64(&fc.sets, 1)
	
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	// Check if key already exists
	if existing, exists := fc.items[key]; exists {
		// Update existing entry
		atomic.AddInt64(&fc.currentSize, -existing.Size)
		existing.Value = value
		existing.Size = size
		existing.ExpiresAt = time.Now().Add(fc.ttl)
		existing.AccessedAt = time.Now()
		atomic.AddInt64(&fc.currentSize, size)
		fc.lruList.MoveToFront(existing.element)
		return
	}
	
	// Evict items if necessary
	for atomic.LoadInt64(&fc.currentSize)+size > fc.maxSize && fc.lruList.Len() > 0 {
		fc.evictOldest()
	}
	
	// Add new entry
	entry := &CacheEntry{
		Key:        key,
		Value:      value,
		Size:       size,
		ExpiresAt:  time.Now().Add(fc.ttl),
		AccessedAt: time.Now(),
	}
	
	entry.element = fc.lruList.PushFront(entry)
	fc.items[key] = entry
	atomic.AddInt64(&fc.currentSize, size)
}

// Delete removes an item from cache
func (fc *FastCache) Delete(key string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	if entry, exists := fc.items[key]; exists {
		fc.lruList.Remove(entry.element)
		delete(fc.items, key)
		atomic.AddInt64(&fc.currentSize, -entry.Size)
	}
}

// evictOldest removes the least recently used item
func (fc *FastCache) evictOldest() {
	elem := fc.lruList.Back()
	if elem == nil {
		return
	}
	
	entry := elem.Value.(*CacheEntry)
	fc.lruList.Remove(elem)
	delete(fc.items, entry.Key)
	atomic.AddInt64(&fc.currentSize, -entry.Size)
	atomic.AddUint64(&fc.evictions, 1)
}

// cleanupLoop periodically removes expired entries
func (fc *FastCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		fc.cleanup()
	}
}

// cleanup removes expired entries
func (fc *FastCache) cleanup() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	now := time.Now()
	var toDelete []string
	
	for key, entry := range fc.items {
		if now.After(entry.ExpiresAt) {
			toDelete = append(toDelete, key)
		}
	}
	
	for _, key := range toDelete {
		if entry, exists := fc.items[key]; exists {
			fc.lruList.Remove(entry.element)
			delete(fc.items, key)
			atomic.AddInt64(&fc.currentSize, -entry.Size)
		}
	}
}

// Stats returns cache statistics
func (fc *FastCache) Stats() CacheStats {
	fc.mu.RLock()
	itemCount := len(fc.items)
	fc.mu.RUnlock()
	
	hits := atomic.LoadUint64(&fc.hits)
	misses := atomic.LoadUint64(&fc.misses)
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	
	return CacheStats{
		Items:      itemCount,
		Size:       atomic.LoadInt64(&fc.currentSize),
		MaxSize:    fc.maxSize,
		Hits:       hits,
		Misses:     misses,
		Evictions:  atomic.LoadUint64(&fc.evictions),
		Sets:       atomic.LoadUint64(&fc.sets),
		HitRate:    hitRate,
	}
}

// Clear removes all items from cache
func (fc *FastCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	fc.items = make(map[string]*CacheEntry)
	fc.lruList.Init()
	atomic.StoreInt64(&fc.currentSize, 0)
}

type CacheStats struct {
	Items      int
	Size       int64
	MaxSize    int64
	Hits       uint64
	Misses     uint64
	Evictions  uint64
	Sets       uint64
	HitRate    float64
}

// MetadataCache caches object metadata
type MetadataCache struct {
	cache *FastCache
}

// NewMetadataCache creates a metadata cache
func NewMetadataCache(maxSizeMB int64, ttl time.Duration) *MetadataCache {
	return &MetadataCache{
		cache: NewFastCache(maxSizeMB*1024*1024, ttl),
	}
}

type ObjectMetadata struct {
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	Exists       bool
}

func (mc *MetadataCache) Get(bucket, key string) (*ObjectMetadata, bool) {
	cacheKey := bucket + "/" + key
	value, ok := mc.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}
	
	metadata := value.(*ObjectMetadata)
	return metadata, true
}

func (mc *MetadataCache) Set(bucket, key string, metadata *ObjectMetadata) {
	cacheKey := bucket + "/" + key
	// Approximate size: 200 bytes for metadata struct
	mc.cache.Set(cacheKey, metadata, 200)
}

func (mc *MetadataCache) Delete(bucket, key string) {
	cacheKey := bucket + "/" + key
	mc.cache.Delete(cacheKey)
}

func (mc *MetadataCache) Stats() CacheStats {
	return mc.cache.Stats()
}

// DataCache caches small object data
type DataCache struct {
	cache      *FastCache
	maxObjSize int64 // Maximum size of objects to cache
}

// NewDataCache creates a data cache for small objects
func NewDataCache(maxSizeMB int64, maxObjectSizeKB int64, ttl time.Duration) *DataCache {
	return &DataCache{
		cache:      NewFastCache(maxSizeMB*1024*1024, ttl),
		maxObjSize: maxObjectSizeKB * 1024,
	}
}

func (dc *DataCache) Get(bucket, key string) ([]byte, bool) {
	cacheKey := bucket + "/" + key
	value, ok := dc.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}
	
	data := value.([]byte)
	return data, true
}

func (dc *DataCache) Set(bucket, key string, data []byte) bool {
	if int64(len(data)) > dc.maxObjSize {
		return false // Object too large to cache
	}
	
	cacheKey := bucket + "/" + key
	// Make a copy to avoid external modifications
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	dc.cache.Set(cacheKey, dataCopy, int64(len(dataCopy)))
	return true
}

func (dc *DataCache) Delete(bucket, key string) {
	cacheKey := bucket + "/" + key
	dc.cache.Delete(cacheKey)
}

func (dc *DataCache) Stats() CacheStats {
	return dc.cache.Stats()
}

// ShardedCache provides a cache with reduced lock contention
type ShardedCache struct {
	shards    []*FastCache
	shardMask uint32
}

// NewShardedCache creates a cache with multiple shards
func NewShardedCache(numShards int, maxSizeBytesPerShard int64, ttl time.Duration) *ShardedCache {
	// Ensure numShards is power of 2
	if numShards&(numShards-1) != 0 {
		panic("numShards must be power of 2")
	}
	
	sc := &ShardedCache{
		shards:    make([]*FastCache, numShards),
		shardMask: uint32(numShards - 1),
	}
	
	for i := 0; i < numShards; i++ {
		sc.shards[i] = NewFastCache(maxSizeBytesPerShard, ttl)
	}
	
	return sc
}

func (sc *ShardedCache) getShard(key string) *FastCache {
	hash := fnv32(key)
	return sc.shards[hash&sc.shardMask]
}

func (sc *ShardedCache) Get(key string) (interface{}, bool) {
	return sc.getShard(key).Get(key)
}

func (sc *ShardedCache) Set(key string, value interface{}, size int64) {
	sc.getShard(key).Set(key, value, size)
}

func (sc *ShardedCache) Delete(key string) {
	sc.getShard(key).Delete(key)
}

func (sc *ShardedCache) Stats() []CacheStats {
	stats := make([]CacheStats, len(sc.shards))
	for i, shard := range sc.shards {
		stats[i] = shard.Stats()
	}
	return stats
}

// fnv32 is a fast hash function
func fnv32(key string) uint32 {
	hash := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}

// CacheManager manages all caches
type CacheManager struct {
	metadataCache *MetadataCache
	dataCache     *DataCache
	enabled       bool
}

// NewCacheManager creates a cache manager
func NewCacheManager(enabled bool, metadataSizeMB, dataSizeMB, maxObjectSizeKB int64, ttl time.Duration) *CacheManager {
	if !enabled {
		return &CacheManager{enabled: false}
	}
	
	return &CacheManager{
		metadataCache: NewMetadataCache(metadataSizeMB, ttl),
		dataCache:     NewDataCache(dataSizeMB, maxObjectSizeKB, ttl),
		enabled:       true,
	}
}

func (cm *CacheManager) GetMetadata(bucket, key string) (*ObjectMetadata, bool) {
	if !cm.enabled {
		return nil, false
	}
	return cm.metadataCache.Get(bucket, key)
}

func (cm *CacheManager) SetMetadata(bucket, key string, metadata *ObjectMetadata) {
	if !cm.enabled {
		return
	}
	cm.metadataCache.Set(bucket, key, metadata)
}

func (cm *CacheManager) GetData(bucket, key string) ([]byte, bool) {
	if !cm.enabled {
		return nil, false
	}
	return cm.dataCache.Get(bucket, key)
}

func (cm *CacheManager) SetData(bucket, key string, data []byte) {
	if !cm.enabled {
		return
	}
	cm.dataCache.Set(bucket, key, data)
}

func (cm *CacheManager) Invalidate(bucket, key string) {
	if !cm.enabled {
		return
	}
	cm.metadataCache.Delete(bucket, key)
	cm.dataCache.Delete(bucket, key)
}

func (cm *CacheManager) Stats() (CacheStats, CacheStats) {
	if !cm.enabled {
		return CacheStats{}, CacheStats{}
	}
	return cm.metadataCache.Stats(), cm.dataCache.Stats()
}
