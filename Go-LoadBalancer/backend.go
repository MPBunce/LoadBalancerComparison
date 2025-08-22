package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
)

// Backend represents a backend server
type Backend struct {
	URL          *url.URL
	alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
	Weight       int
	connections  int64
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

// NewBackend creates a new backend instance
func NewBackend(serverURL string, weight int) (*Backend, error) {
	url, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	return &Backend{
		URL:          url,
		alive:        true,
		ReverseProxy: proxy,
		Weight:       weight,
	}, nil
}