package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Backend represents a backend server with circuit breaker functionality
type Backend struct {
	URL          *url.URL
	alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
	Weight       int
	connections  int64

	// Circuit breaker fields
	consecutiveErrors int64
	lastErrorTime     time.Time
	circuitOpen       bool
	circuitMux        sync.RWMutex

	// Configuration
	maxConsecutiveErrors int
	circuitTimeout       time.Duration
}

// SetAlive updates the alive status of the backend
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// IsAlive returns the alive status of the backend
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	b.mux.RUnlock()
	return alive
}

// IsCircuitOpen checks if the circuit breaker is open
func (b *Backend) IsCircuitOpen() bool {
	b.circuitMux.RLock()
	defer b.circuitMux.RUnlock()

	// If circuit is open, check if timeout has passed
	if b.circuitOpen {
		if time.Since(b.lastErrorTime) > b.circuitTimeout {
			b.circuitOpen = false // Reset circuit breaker
			atomic.StoreInt64(&b.consecutiveErrors, 0)
			return false
		}
		return true
	}
	return false
}

// IsAvailable returns true if backend is alive and circuit is not open
func (b *Backend) IsAvailable() bool {
	return b.IsAlive() && !b.IsCircuitOpen()
}

// RecordSuccess resets the consecutive error count
func (b *Backend) RecordSuccess() {
	atomic.StoreInt64(&b.consecutiveErrors, 0)
	b.circuitMux.Lock()
	b.circuitOpen = false
	b.circuitMux.Unlock()
}

// RecordError increments consecutive errors and opens circuit if threshold is reached
func (b *Backend) RecordError() {
	errors := atomic.AddInt64(&b.consecutiveErrors, 1)

	b.circuitMux.Lock()
	b.lastErrorTime = time.Now()

	if errors >= int64(b.maxConsecutiveErrors) {
		b.circuitOpen = true
	}
	b.circuitMux.Unlock()
}

// GetConsecutiveErrors returns the current consecutive error count
func (b *Backend) GetConsecutiveErrors() int64 {
	return atomic.LoadInt64(&b.consecutiveErrors)
}

// AddConnection increments the connection count
func (b *Backend) AddConnection() {
	atomic.AddInt64(&b.connections, 1)
}

// RemoveConnection decrements the connection count
func (b *Backend) RemoveConnection() {
	atomic.AddInt64(&b.connections, -1)
}

// GetConnections returns the current connection count
func (b *Backend) GetConnections() int64 {
	return atomic.LoadInt64(&b.connections)
}

// NewBackend creates a new backend instance with circuit breaker
func NewBackend(serverURL string, weight int) (*Backend, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	return &Backend{
		URL:          u,
		alive:        true,
		ReverseProxy: proxy,
		Weight:       weight,

		// Circuit breaker defaults
		maxConsecutiveErrors: 10,               // Circuit opens after 10 consecutive 500 errors
		circuitTimeout:       30 * time.Second, // Circuit stays open for 30 seconds
	}, nil
}

// NewBackendWithCircuitConfig creates a backend with custom circuit breaker settings
func NewBackendWithCircuitConfig(serverURL string, weight int, maxErrors int, timeout time.Duration) (*Backend, error) {
	backend, err := NewBackend(serverURL, weight)
	if err != nil {
		return nil, err
	}

	backend.maxConsecutiveErrors = maxErrors
	backend.circuitTimeout = timeout

	return backend, nil
}
