package main

import (
	"log"
	"net"
	"net/url"
	"sync"
	"time"
)

// ServerPool holds information about reachable backends
type ServerPool struct {
	backends  []*Backend
	algorithm LoadBalancingAlgorithm
	mux       sync.RWMutex
}

// NewServerPool creates a new server pool
func NewServerPool(algorithm LoadBalancingAlgorithm) *ServerPool {
	return &ServerPool{
		backends:  make([]*Backend, 0),
		algorithm: algorithm,
	}
}

// AddBackend adds a backend to the server pool
func (s *ServerPool) AddBackend(backend *Backend) {
	s.mux.Lock()
	s.backends = append(s.backends, backend)
	s.mux.Unlock()
	log.Printf("Added backend: %s (weight: %d)", backend.URL.String(), backend.Weight)
}

// NextPeer returns the next available backend
func (s *ServerPool) NextPeer() *Backend {
	s.mux.RLock()
	backends := make([]*Backend, len(s.backends))
	copy(backends, s.backends)
	s.mux.RUnlock()
	
	return s.algorithm.NextBackend(backends)
}

// GetBackends returns a copy of the backends slice
func (s *ServerPool) GetBackends() []*Backend {
	s.mux.RLock()
	backends := make([]*Backend, len(s.backends))
	copy(backends, s.backends)
	s.mux.RUnlock()
	return backends
}

// HealthCheck pings the backends and updates the status
func (s *ServerPool) HealthCheck() {
	backends := s.GetBackends()
	
	var wg sync.WaitGroup
	for _, b := range backends {
		wg.Add(1)
		go func(backend *Backend) {
			defer wg.Done()
			alive := isBackendAlive(backend.URL)
			wasAlive := backend.IsAlive()
			backend.SetAlive(alive)
			
			status := "up"
			if !alive {
				status = "down"
			}
			
			// Only log if status changed
			if alive != wasAlive {
				log.Printf("Backend %s status changed: %s", backend.URL.String(), status)
			}
		}(b)
	}
	wg.Wait()
}

// GetStats returns statistics about the server pool
func (s *ServerPool) GetStats() map[string]interface{} {
	backends := s.GetBackends()
	
	stats := map[string]interface{}{
		"algorithm":      s.algorithm.Name(),
		"total_backends": len(backends),
		"alive_backends": 0,
		"backends":       make([]map[string]interface{}, 0),
	}
	
	aliveCount := 0
	for _, backend := range backends {
		alive := backend.IsAlive()
		if alive {
			aliveCount++
		}
		
		backendInfo := map[string]interface{}{
			"url":         backend.URL.String(),
			"status":      map[bool]string{true: "up", false: "down"}[alive],
			"connections": backend.GetConnections(),
			"weight":      backend.Weight,
		}
		
		stats["backends"] = append(stats["backends"].([]map[string]interface{}), backendInfo)
	}
	
	stats["alive_backends"] = aliveCount
	return stats
}

// isBackendAlive checks whether a backend is alive by establishing a TCP connection
func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
