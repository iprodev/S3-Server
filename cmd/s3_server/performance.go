// performance.go - Performance utilities (lightweight helpers)
// Main PerformanceManager is in performance_manager.go

package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LatencyTracker tracks request latencies
type LatencyTracker struct {
	mu         sync.RWMutex
	latencies  []time.Duration
	maxSamples int
	sum        time.Duration
	count      uint64
}

func NewLatencyTracker(maxSamples int) *LatencyTracker {
	return &LatencyTracker{
		latencies:  make([]time.Duration, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.sum += latency
	lt.count++

	if len(lt.latencies) >= lt.maxSamples {
		lt.latencies = lt.latencies[1:]
	}
	lt.latencies = append(lt.latencies, latency)
}

func (lt *LatencyTracker) Avg() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if lt.count == 0 {
		return 0
	}
	return lt.sum / time.Duration(lt.count)
}

// RateLimiter simple token bucket
type RateLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(maxTokens, refillRate float64) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	
	return false
}

func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	maxFailures  uint64
	resetTimeout time.Duration
	
	failures    uint64
	lastFailure time.Time
	state       CircuitState
	mu          sync.RWMutex
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func NewCircuitBreaker(maxFailures uint64, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	if state == CircuitOpen {
		cb.mu.RLock()
		elapsed := time.Since(cb.lastFailure)
		cb.mu.RUnlock()
		
		if elapsed > cb.resetTimeout {
			cb.mu.Lock()
			cb.state = CircuitHalfOpen
			cb.mu.Unlock()
		} else {
			return fmt.Errorf("circuit breaker is open")
		}
	}

	err := fn()
	
	if err != nil {
		cb.recordFailure()
		return err
	}
	
	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.state = CircuitClosed
}
