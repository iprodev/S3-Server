// connection_pool.go - HTTP connection pooling for backend calls
package main

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages HTTP client connections with pooling
type ConnectionPool struct {
	clients     map[string]*http.Client
	mu          sync.RWMutex
	stats       ConnectionPoolStats
	maxIdleConn int
	idleTimeout time.Duration
}

type ConnectionPoolStats struct {
	ActiveConnections uint64
	IdleConnections   uint64
	Reused            uint64
	Created           uint64
	Closed            uint64
}

// NewConnectionPool creates an optimized HTTP connection pool
func NewConnectionPool(maxIdleConn int, idleTimeout time.Duration) *ConnectionPool {
	return &ConnectionPool{
		clients:     make(map[string]*http.Client),
		maxIdleConn: maxIdleConn,
		idleTimeout: idleTimeout,
	}
}

// GetClient returns or creates an optimized HTTP client for a host
func (cp *ConnectionPool) GetClient(host string) *http.Client {
	cp.mu.RLock()
	client, exists := cp.clients[host]
	cp.mu.RUnlock()

	if exists {
		atomic.AddUint64(&cp.stats.Reused, 1)
		return client
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := cp.clients[host]; exists {
		atomic.AddUint64(&cp.stats.Reused, 1)
		return client
	}

	// Create optimized transport
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        cp.maxIdleConn,
		MaxIdleConnsPerHost: cp.maxIdleConn,
		MaxConnsPerHost:     cp.maxIdleConn * 2,
		IdleConnTimeout:     cp.idleTimeout,

		// Performance optimizations
		DisableCompression:  false,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
		WriteBufferSize:     128 * 1024, // 128KB
		ReadBufferSize:      128 * 1024, // 128KB
		ExpectContinueTimeout: 1 * time.Second,

		// Connection establishment
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true, // IPv4 and IPv6
		}).DialContext,

		// TLS handshake timeout
		TLSHandshakeTimeout: 10 * time.Second,

		// Response header timeout
		ResponseHeaderTimeout: 30 * time.Second,
	}

	client = &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	cp.clients[host] = client
	atomic.AddUint64(&cp.stats.Created, 1)

	return client
}

// Stats returns connection pool statistics
func (cp *ConnectionPool) Stats() ConnectionPoolStats {
	return ConnectionPoolStats{
		ActiveConnections: atomic.LoadUint64(&cp.stats.ActiveConnections),
		IdleConnections:   atomic.LoadUint64(&cp.stats.IdleConnections),
		Reused:            atomic.LoadUint64(&cp.stats.Reused),
		Created:           atomic.LoadUint64(&cp.stats.Created),
		Closed:            atomic.LoadUint64(&cp.stats.Closed),
	}
}

// Close closes all connections in the pool
func (cp *ConnectionPool) Close() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, client := range cp.clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	cp.clients = make(map[string]*http.Client)
}

// Global connection pool
var (
	globalConnPool     *ConnectionPool
	connPoolOnce       sync.Once
)

// GetGlobalConnectionPool returns the global connection pool
func GetGlobalConnectionPool() *ConnectionPool {
	connPoolOnce.Do(func() {
		globalConnPool = NewConnectionPool(100, 90*time.Second)
	})
	return globalConnPool
}
