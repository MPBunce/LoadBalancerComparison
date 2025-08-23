package main

import (
	"fmt"
	"log"
	"net/http"
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
	log.Printf("➕ [POOL] Added backend: %s (weight: %d)", backend.URL.String(), backend.Weight)
}

// NextPeer returns the next available backend (including circuit breaker check)
func (s *ServerPool) NextPeer() *Backend {
	s.mux.RLock()
	backends := make([]*Backend, len(s.backends))
	copy(backends, s.backends)
	s.mux.RUnlock()

	backend := s.algorithm.NextBackend(backends)
	if backend == nil {
		log.Printf("❌ [POOL] No available backends")
		return nil
	}

	if backend.IsAvailable() {
		log.Printf("🎯 [ROUTE] Selected backend: %s (connections: %d, weight: %d, errors: %d)",
			backend.URL.String(), backend.GetConnections(), backend.Weight, backend.GetConsecutiveErrors())
	} else if backend.IsCircuitOpen() {
		log.Printf("🔒 [ROUTE] Backend %s circuit breaker is OPEN, looking for alternative", backend.URL.String())
		// Try to find another available backend
		return s.findAlternativeBackend(backends, backend)
	} else {
		log.Printf("⚠️ [ROUTE] Warning: chosen backend %s is DOWN, request may fail", backend.URL.String())
	}

	return backend
}

// NextAvailablePeer returns the next available backend, respecting circuit breakers
func (s *ServerPool) NextAvailablePeer() *Backend {
	s.mux.RLock()
	backends := make([]*Backend, len(s.backends))
	copy(backends, s.backends)
	s.mux.RUnlock()

	// Filter only available backends (alive and circuit not open)
	availableBackends := make([]*Backend, 0)
	unavailableReasons := make([]string, 0)

	for _, backend := range backends {
		if backend.IsAvailable() {
			availableBackends = append(availableBackends, backend)
		} else {
			reason := "DOWN"
			if backend.IsAlive() && backend.IsCircuitOpen() {
				reason = "CIRCUIT_OPEN"
			} else if !backend.IsAlive() && backend.IsCircuitOpen() {
				reason = "DOWN+CIRCUIT_OPEN"
			}
			unavailableReasons = append(unavailableReasons,
				backend.URL.String()+":"+reason)
		}
	}

	if len(availableBackends) == 0 {
		log.Printf("❌ [POOL] No available backends - unavailable: [%s]",
			joinStrings(unavailableReasons, ", "))
		return nil
	}

	// Log the available pool
	var availableUrls []string
	for _, b := range availableBackends {
		status := "HEALTHY"
		if b.GetConsecutiveErrors() > 0 {
			status = "DEGRADED"
		}
		availableUrls = append(availableUrls, b.URL.String()+":"+status)
	}

	log.Printf("📋 [POOL] Available backends: [%s] (%d/%d available)",
		joinStrings(availableUrls, ", "), len(availableBackends), len(backends))

	// Use the load balancing algorithm on available backends
	backend := s.algorithm.NextBackend(availableBackends)
	if backend != nil {
		healthStatus := "✅"
		if backend.GetConsecutiveErrors() > 0 {
			healthStatus = "⚠️"
		}
		log.Printf("%s [ROUTE] Selected backend: %s (connections: %d, weight: %d, errors: %d)",
			healthStatus, backend.URL.String(), backend.GetConnections(),
			backend.Weight, backend.GetConsecutiveErrors())
	}

	return backend
}

// GetAvailableBackends returns all currently available backends
func (s *ServerPool) GetAvailableBackends() []*Backend {
	s.mux.RLock()
	defer s.mux.RUnlock()

	availableBackends := make([]*Backend, 0)
	for _, backend := range s.backends {
		if backend.IsAvailable() {
			availableBackends = append(availableBackends, backend)
		}
	}
	return availableBackends
}

// GetPoolSummary returns a quick summary of pool status
func (s *ServerPool) GetPoolSummary() map[string]int {
	s.mux.RLock()
	defer s.mux.RUnlock()

	total := len(s.backends)
	alive := 0
	available := 0
	circuitsClosed := 0

	for _, backend := range s.backends {
		if backend.IsAlive() {
			alive++
		}
		if backend.IsAvailable() {
			available++
		}
		if !backend.IsCircuitOpen() {
			circuitsClosed++
		}
	}

	return map[string]int{
		"total":           total,
		"alive":           alive,
		"available":       available,
		"circuits_closed": circuitsClosed,
	}
}

// findAlternativeBackend tries to find an alternative backend when the primary choice is unavailable
func (s *ServerPool) findAlternativeBackend(backends []*Backend, excludeBackend *Backend) *Backend {
	for _, backend := range backends {
		if backend != excludeBackend && backend.IsAvailable() {
			log.Printf("🔄 [ROUTE] Found alternative backend: %s (errors: %d)",
				backend.URL.String(), backend.GetConsecutiveErrors())
			return backend
		}
	}
	log.Printf("❌ [ROUTE] No alternative backends available")
	return nil
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

	log.Printf("🏥 [HEALTH] Checking %d backends...", len(backends))

	for _, b := range backends {
		wg.Add(1)
		go func(backend *Backend) {
			defer wg.Done()
			start := time.Now()
			alive := isBackendAlive(backend.URL)
			latency := time.Since(start)

			wasAlive := backend.IsAlive()
			wasCircuitOpen := backend.IsCircuitOpen()

			backend.SetAlive(alive)

			// Enhanced status reporting
			healthEmoji := "✅"
			healthStatus := "UP"
			if !alive {
				healthEmoji = "🔴"
				healthStatus = "DOWN"
			}

			circuitEmoji := "🔓"
			circuitStatus := "CLOSED"
			if backend.IsCircuitOpen() {
				circuitEmoji = "🔒"
				circuitStatus = "OPEN"
			} else if backend.GetConsecutiveErrors() > 0 {
				circuitEmoji = "⚠️"
				circuitStatus = fmt.Sprintf("DEGRADED (%d errors)", backend.GetConsecutiveErrors())
			}

			// Log status changes prominently
			if alive != wasAlive {
				log.Printf("🔄 [HEALTH] Backend %s status CHANGED: %s%s → %s%s (latency: %v)",
					backend.URL.String(),
					map[bool]string{true: "✅UP", false: "🔴DOWN"}[wasAlive],
					map[bool]string{true: "🔒CIRCUIT_OPEN", false: "🔓CIRCUIT_CLOSED"}[wasCircuitOpen],
					healthEmoji+healthStatus, circuitEmoji+circuitStatus, latency)
			} else {
				// Regular health check log (less prominent)
				log.Printf("🏥 [HEALTH] %s: %s%s, circuit=%s%s (latency: %v)",
					backend.URL.String(), healthEmoji, healthStatus, circuitEmoji, circuitStatus, latency)
			}

			// Circuit breaker recovery logic
			if alive && backend.IsCircuitOpen() {
				log.Printf("🔄 [CIRCUIT] Backend %s is healthy again, circuit may reset on next successful request",
					backend.URL.String())
			}

			// Log if backend becomes available/unavailable
			isAvailableNow := backend.IsAvailable()
			if alive != wasAlive || backend.IsCircuitOpen() != wasCircuitOpen {
				availabilityStatus := "🟢 AVAILABLE"
				if !isAvailableNow {
					availabilityStatus = "🔴 UNAVAILABLE"
				}
				log.Printf("📊 [AVAILABILITY] Backend %s is now: %s", backend.URL.String(), availabilityStatus)
			}
		}(b)
	}
	wg.Wait()

	// Summary after all health checks
	summary := s.GetPoolSummary()
	log.Printf("📊 [HEALTH] Health check complete: %d/%d alive, %d/%d available, %d/%d circuits closed",
		summary["alive"], summary["total"],
		summary["available"], summary["total"],
		summary["circuits_closed"], summary["total"])
}

// GetStats returns statistics about the server pool including circuit breaker info
func (s *ServerPool) GetStats() map[string]interface{} {
	backends := s.GetBackends()
	stats := map[string]interface{}{
		"algorithm":          s.algorithm.Name(),
		"total_backends":     len(backends),
		"alive_backends":     0,
		"available_backends": 0,
		"backends":           make([]map[string]interface{}, 0),
	}

	aliveCount := 0
	availableCount := 0

	for _, backend := range backends {
		alive := backend.IsAlive()
		available := backend.IsAvailable()

		if alive {
			aliveCount++
		}
		if available {
			availableCount++
		}

		status := "down"
		if alive {
			status = "up"
		}
		if backend.IsCircuitOpen() {
			status += " (circuit open)"
		}

		// Enhanced backend info
		backendInfo := map[string]interface{}{
			"url":                backend.URL.String(),
			"status":             status,
			"connections":        backend.GetConnections(),
			"weight":             backend.Weight,
			"consecutive_errors": backend.GetConsecutiveErrors(),
			"circuit_open":       backend.IsCircuitOpen(),
			"available":          available,
			"alive":              alive,
			"health_status":      map[bool]string{true: "healthy", false: "unhealthy"}[alive],
			"circuit_status":     map[bool]string{true: "open", false: "closed"}[backend.IsCircuitOpen()],
		}
		stats["backends"] = append(stats["backends"].([]map[string]interface{}), backendInfo)
	}

	stats["alive_backends"] = aliveCount
	stats["available_backends"] = availableCount
	stats["pool_health_percentage"] = float64(availableCount) / float64(len(backends)) * 100

	return stats
}

// isBackendAlive checks whether a backend is alive by checking health endpoint
func isBackendAlive(u *url.URL) bool {
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	// Check the health endpoint specifically
	healthURL := u.String() + "/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		// If health endpoint fails, try the root endpoint
		resp, err = client.Get(u.String())
		if err != nil {
			return false
		}
	}
	defer resp.Body.Close()

	// Consider only 2xx status codes as alive
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Helper function to join strings (since Go doesn't have a built-in for string slices)
func joinStrings(strs []string, separator string) string {
	if len(strs) == 0 {
		return ""
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += separator + strs[i]
	}
	return result
}
