// rate_limiter.go - Adaptive rate limiting to prevent overload
package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveRateLimiter implements token bucket with adaptive limits
type AdaptiveRateLimiter struct {
	tokens         int64
	maxTokens      int64
	refillRate     int64 // tokens per second
	lastRefill     int64 // unix nano
	mu             sync.Mutex
	
	// Adaptive settings
	minTokens      int64
	maxTokensLimit int64
	errorRate      float64
	latencyP99     time.Duration
	
	// Statistics
	allowed        uint64
	rejected       uint64
	adaptations    uint64
}

// NewAdaptiveRateLimiter creates a rate limiter with adaptive capacity
func NewAdaptiveRateLimiter(initialRate, minRate, maxRate int64) *AdaptiveRateLimiter {
	now := time.Now().UnixNano()
	return &AdaptiveRateLimiter{
		tokens:         initialRate,
		maxTokens:      initialRate,
		refillRate:     initialRate,
		lastRefill:     now,
		minTokens:      minRate,
		maxTokensLimit: maxRate,
	}
}

// Allow checks if a request can proceed
func (rl *AdaptiveRateLimiter) Allow() bool {
	rl.refill()
	
	if atomic.LoadInt64(&rl.tokens) > 0 {
		atomic.AddInt64(&rl.tokens, -1)
		atomic.AddUint64(&rl.allowed, 1)
		return true
	}
	
	atomic.AddUint64(&rl.rejected, 1)
	return false
}

// AllowN checks if N requests can proceed
func (rl *AdaptiveRateLimiter) AllowN(n int64) bool {
	rl.refill()
	
	for {
		current := atomic.LoadInt64(&rl.tokens)
		if current < n {
			atomic.AddUint64(&rl.rejected, 1)
			return false
		}
		
		if atomic.CompareAndSwapInt64(&rl.tokens, current, current-n) {
			atomic.AddUint64(&rl.allowed, 1)
			return true
		}
	}
}

// Wait blocks until a token is available or context is cancelled
func (rl *AdaptiveRateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Retry after brief pause
		}
	}
}

// refill adds tokens based on elapsed time
func (rl *AdaptiveRateLimiter) refill() {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&rl.lastRefill)
	elapsed := now - last
	
	if elapsed < int64(time.Millisecond) {
		return // Too soon to refill
	}
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Double-check after lock
	if atomic.LoadInt64(&rl.lastRefill) != last {
		return // Another goroutine already refilled
	}
	
	// Calculate tokens to add
	rate := atomic.LoadInt64(&rl.refillRate)
	tokensToAdd := (elapsed * rate) / int64(time.Second)
	
	if tokensToAdd > 0 {
		current := atomic.LoadInt64(&rl.tokens)
		maxTokens := atomic.LoadInt64(&rl.maxTokens)
		newTokens := current + tokensToAdd
		
		if newTokens > maxTokens {
			newTokens = maxTokens
		}
		
		atomic.StoreInt64(&rl.tokens, newTokens)
		atomic.StoreInt64(&rl.lastRefill, now)
	}
}

// AdaptUp increases capacity based on good performance
func (rl *AdaptiveRateLimiter) AdaptUp(factor float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	current := atomic.LoadInt64(&rl.maxTokens)
	newLimit := int64(float64(current) * (1.0 + factor))
	
	if newLimit > rl.maxTokensLimit {
		newLimit = rl.maxTokensLimit
	}
	
	if newLimit > current {
		atomic.StoreInt64(&rl.maxTokens, newLimit)
		atomic.StoreInt64(&rl.refillRate, newLimit)
		atomic.AddUint64(&rl.adaptations, 1)
	}
}

// AdaptDown decreases capacity based on poor performance
func (rl *AdaptiveRateLimiter) AdaptDown(factor float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	current := atomic.LoadInt64(&rl.maxTokens)
	newLimit := int64(float64(current) * (1.0 - factor))
	
	if newLimit < rl.minTokens {
		newLimit = rl.minTokens
	}
	
	if newLimit < current {
		atomic.StoreInt64(&rl.maxTokens, newLimit)
		atomic.StoreInt64(&rl.refillRate, newLimit)
		atomic.AddUint64(&rl.adaptations, 1)
	}
}

// UpdateMetrics updates rate limiter based on system metrics
func (rl *AdaptiveRateLimiter) UpdateMetrics(errorRate float64, latencyP99 time.Duration) {
	rl.mu.Lock()
	rl.errorRate = errorRate
	rl.latencyP99 = latencyP99
	rl.mu.Unlock()
	
	// Adapt based on error rate
	if errorRate > 0.05 { // >5% errors
		rl.AdaptDown(0.2) // Reduce by 20%
	} else if errorRate > 0.01 { // >1% errors
		rl.AdaptDown(0.1) // Reduce by 10%
	} else if errorRate < 0.001 && latencyP99 < 100*time.Millisecond {
		rl.AdaptUp(0.1) // Increase by 10% if performing well
	}
}

// Stats returns rate limiter statistics
func (rl *AdaptiveRateLimiter) Stats() RateLimiterStats {
	allowed := atomic.LoadUint64(&rl.allowed)
	rejected := atomic.LoadUint64(&rl.rejected)
	total := allowed + rejected
	
	acceptRate := float64(0)
	if total > 0 {
		acceptRate = float64(allowed) / float64(total)
	}
	
	return RateLimiterStats{
		CurrentTokens: atomic.LoadInt64(&rl.tokens),
		MaxTokens:     atomic.LoadInt64(&rl.maxTokens),
		RefillRate:    atomic.LoadInt64(&rl.refillRate),
		Allowed:       allowed,
		Rejected:      rejected,
		AcceptRate:    acceptRate,
		Adaptations:   atomic.LoadUint64(&rl.adaptations),
	}
}

type RateLimiterStats struct {
	CurrentTokens int64
	MaxTokens     int64
	RefillRate    int64
	Allowed       uint64
	Rejected      uint64
	AcceptRate    float64
	Adaptations   uint64
}

// PerBucketRateLimiter provides per-bucket rate limiting
type PerBucketRateLimiter struct {
	limiters map[string]*AdaptiveRateLimiter
	mu       sync.RWMutex
	defaultRate int64
	minRate     int64
	maxRate     int64
}

// NewPerBucketRateLimiter creates a per-bucket rate limiter
func NewPerBucketRateLimiter(defaultRate, minRate, maxRate int64) *PerBucketRateLimiter {
	return &PerBucketRateLimiter{
		limiters:    make(map[string]*AdaptiveRateLimiter),
		defaultRate: defaultRate,
		minRate:     minRate,
		maxRate:     maxRate,
	}
}

// Allow checks if a request to the bucket can proceed
func (pbrl *PerBucketRateLimiter) Allow(bucket string) bool {
	limiter := pbrl.getLimiter(bucket)
	return limiter.Allow()
}

// Wait blocks until a request to the bucket can proceed
func (pbrl *PerBucketRateLimiter) Wait(ctx context.Context, bucket string) error {
	limiter := pbrl.getLimiter(bucket)
	return limiter.Wait(ctx)
}

// getLimiter returns or creates a rate limiter for a bucket
func (pbrl *PerBucketRateLimiter) getLimiter(bucket string) *AdaptiveRateLimiter {
	pbrl.mu.RLock()
	limiter, exists := pbrl.limiters[bucket]
	pbrl.mu.RUnlock()
	
	if exists {
		return limiter
	}
	
	pbrl.mu.Lock()
	defer pbrl.mu.Unlock()
	
	// Double-check after lock
	if limiter, exists := pbrl.limiters[bucket]; exists {
		return limiter
	}
	
	limiter = NewAdaptiveRateLimiter(pbrl.defaultRate, pbrl.minRate, pbrl.maxRate)
	pbrl.limiters[bucket] = limiter
	
	return limiter
}

// UpdateMetrics updates rate limiter metrics for a bucket
func (pbrl *PerBucketRateLimiter) UpdateMetrics(bucket string, errorRate float64, latencyP99 time.Duration) {
	limiter := pbrl.getLimiter(bucket)
	limiter.UpdateMetrics(errorRate, latencyP99)
}

// Stats returns statistics for all buckets
func (pbrl *PerBucketRateLimiter) Stats() map[string]RateLimiterStats {
	pbrl.mu.RLock()
	defer pbrl.mu.RUnlock()
	
	stats := make(map[string]RateLimiterStats)
	for bucket, limiter := range pbrl.limiters {
		stats[bucket] = limiter.Stats()
	}
	
	return stats
}

// GlobalRateLimiter is the shared instance
var (
	globalRateLimiter *AdaptiveRateLimiter
	rateLimiterOnce   sync.Once
)

// GetGlobalRateLimiter returns the global rate limiter
func GetGlobalRateLimiter() *AdaptiveRateLimiter {
	rateLimiterOnce.Do(func() {
		globalRateLimiter = NewAdaptiveRateLimiter(
			1000,  // Initial 1000 req/s
			100,   // Min 100 req/s
			10000, // Max 10000 req/s
		)
	})
	return globalRateLimiter
}
