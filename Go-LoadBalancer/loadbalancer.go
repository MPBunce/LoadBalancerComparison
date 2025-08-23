package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// LoadBalancer represents the main load balancer
type LoadBalancer struct {
	config     *Config
	serverPool *ServerPool
}

// NewLoadBalancer creates a new load balancer instance
func NewLoadBalancer(config *Config) *LoadBalancer {
	algorithm := CreateAlgorithm(config.Algorithm)

	return &LoadBalancer{
		config:     config,
		serverPool: NewServerPool(algorithm),
	}
}

// AddBackend adds a backend server to the load balancer
func (lb *LoadBalancer) AddBackend(serverURL string, weight int) error {
	backend, err := NewBackend(serverURL, weight)
	if err != nil {
		return fmt.Errorf("failed to create backend %s: %v", serverURL, err)
	}

	// Customize the proxy error handler
	backend.ReverseProxy.ErrorHandler = lb.createErrorHandler(backend)

	lb.serverPool.AddBackend(backend)
	return nil
}

func (lb *LoadBalancer) createErrorHandler(backend *Backend) func(http.ResponseWriter, *http.Request, error) {
	return func(writer http.ResponseWriter, request *http.Request, e error) {
		retries := getRetryFromContext(request)

		// Record the error for circuit breaker
		backend.RecordError()

		// Enhanced error logging with more context
		errorType := "CONNECTION_ERROR"
		if strings.Contains(e.Error(), "timeout") {
			errorType = "TIMEOUT_ERROR"
		} else if strings.Contains(e.Error(), "refused") {
			errorType = "CONNECTION_REFUSED"
		}

		log.Printf(
			"[ERROR] 🚨 %s %s from %s → backend %s failed: %s (attempt %d/%d, consecutive errors: %d, error_type: %s)",
			request.Method, request.URL.Path, request.RemoteAddr,
			backend.URL.String(), e.Error(), retries+1, lb.config.MaxRetries,
			backend.GetConsecutiveErrors(), errorType,
		)

		if backend.IsCircuitOpen() {
			log.Printf("🔌 [CIRCUIT] Circuit breaker OPENED for backend %s (threshold reached: %d errors)",
				backend.URL.String(), backend.GetConsecutiveErrors())
		}

		if retries < lb.config.MaxRetries {
			log.Printf(
				"🔄 [RETRY] Attempting reroute for %s %s (attempt %d/%d) - looking for alternative backend",
				request.Method, request.URL.Path, retries+1, lb.config.MaxRetries,
			)

			// Show available alternatives
			alternatives := lb.serverPool.GetAvailableBackends()
			if len(alternatives) > 0 {
				var altUrls []string
				for _, alt := range alternatives {
					if alt != backend { // Don't include the failing backend
						altUrls = append(altUrls, alt.URL.String())
					}
				}
				if len(altUrls) > 0 {
					log.Printf("📋 [RETRY] Available alternatives: %s", strings.Join(altUrls, ", "))
				} else {
					log.Printf("⚠️ [RETRY] No healthy alternatives available!")
				}
			}

			time.Sleep(10 * time.Millisecond)
			ctx := context.WithValue(request.Context(), retryKey, retries+1)
			lb.loadBalance(writer, request.WithContext(ctx))
			return
		}

		log.Printf("❌ [FAIL] Max retries exceeded for %s %s, returning 503 (no healthy backends available)",
			request.Method, request.URL.Path)
		http.Error(writer, "Service not available", http.StatusServiceUnavailable)
	}
}

// Retry key for context
type contextKey string

const retryKey contextKey = "retry"

// getRetryFromContext returns the retry count from context
func getRetryFromContext(r *http.Request) int {
	if retry, ok := r.Context().Value(retryKey).(int); ok {
		return retry
	}
	return 0
}

// ResponseRecorder wraps http.ResponseWriter to track response status for circuit breaker
type ResponseRecorder struct {
	http.ResponseWriter
	backend    *Backend
	statusCode int
}

// WriteHeader captures the status code and records success/failure
func (rr *ResponseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode

	// Enhanced status code handling with better logging
	if statusCode >= 500 && statusCode < 600 {
		rr.backend.RecordError()

		errorCategory := "SERVER_ERROR"
		switch statusCode {
		case 500:
			errorCategory = "INTERNAL_SERVER_ERROR"
		case 502:
			errorCategory = "BAD_GATEWAY"
		case 503:
			errorCategory = "SERVICE_UNAVAILABLE"
		case 504:
			errorCategory = "GATEWAY_TIMEOUT"
		}

		log.Printf("🔴 [ERROR] Backend %s returned %d (%s) - consecutive errors: %d",
			rr.backend.URL.String(), statusCode, errorCategory, rr.backend.GetConsecutiveErrors())

		if rr.backend.IsCircuitOpen() {
			log.Printf("🔌 [CIRCUIT] Circuit breaker OPENED for backend %s after %d consecutive errors",
				rr.backend.URL.String(), rr.backend.GetConsecutiveErrors())
		}
	} else if statusCode >= 200 && statusCode < 400 {
		wasInError := rr.backend.GetConsecutiveErrors() > 0
		rr.backend.RecordSuccess()

		if wasInError {
			log.Printf("✅ [RECOVERY] Backend %s recovered! Status: %d (errors reset to 0)",
				rr.backend.URL.String(), statusCode)
		}
	} else if statusCode >= 400 && statusCode < 500 {
		// Client errors don't count as backend failures
		log.Printf("⚠️ [CLIENT_ERROR] Backend %s returned %d (client error, not backend failure)",
			rr.backend.URL.String(), statusCode)
	}

	rr.ResponseWriter.WriteHeader(statusCode)
}

func (lb *LoadBalancer) loadBalance(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	retryCount := getRetryFromContext(r)

	// Use NextAvailablePeer to respect circuit breakers
	peer := lb.serverPool.NextAvailablePeer()
	clientIP := r.RemoteAddr

	if peer != nil {
		peer.AddConnection()
		defer peer.RemoveConnection()

		// Create response recorder to track status codes
		recorder := &ResponseRecorder{
			ResponseWriter: w,
			backend:        peer,
		}

		// Enhanced request logging with health vs request status distinction
		healthStatus := "✅ HEALTHY"
		if !peer.IsAlive() {
			healthStatus = "🔴 DOWN"
		}

		circuitStatus := "🔓 CLOSED"
		if peer.IsCircuitOpen() {
			circuitStatus = "🔒 OPEN"
		} else if peer.GetConsecutiveErrors() > 0 {
			circuitStatus = fmt.Sprintf("⚠️ DEGRADED (%d errors)", peer.GetConsecutiveErrors())
		}

		retryInfo := ""
		if retryCount > 0 {
			retryInfo = fmt.Sprintf(" [RETRY %d/%d]", retryCount, lb.config.MaxRetries)
		}

		log.Printf(
			"🎯 [ROUTE]%s %s %s from %s → backend %s (connections=%d, weight=%d, health=%s, circuit=%s)",
			retryInfo, r.Method, r.URL.Path, clientIP,
			peer.URL.String(),
			peer.GetConnections(),
			peer.Weight,
			healthStatus,
			circuitStatus,
		)

		peer.ReverseProxy.ServeHTTP(recorder, r)

		// Enhanced response logging with success/failure indication
		duration := time.Since(start)
		statusInfo := ""
		statusEmoji := "✅"

		if recorder.statusCode != 0 {
			statusInfo = fmt.Sprintf("[%d]", recorder.statusCode)
			if recorder.statusCode >= 500 {
				statusEmoji = "🔴"
			} else if recorder.statusCode >= 400 {
				statusEmoji = "⚠️"
			}
		}

		log.Printf(
			"%s [RESPONSE] %s %s served by %s in %v %s",
			statusEmoji, r.Method, r.URL.Path, peer.URL.String(), duration, statusInfo,
		)
		return
	}

	// Enhanced failure logging with pool status
	poolStats := lb.serverPool.GetPoolSummary()
	log.Printf("❌ [FAIL] No available backend for %s %s from %s", r.Method, r.URL.Path, clientIP)
	log.Printf("📊 [POOL_STATUS] Total: %d, Alive: %d, Available: %d (circuits closed: %d)",
		poolStats["total"], poolStats["alive"], poolStats["available"], poolStats["circuits_closed"])

	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}

// healthCheck endpoint
func (lb *LoadBalancer) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := lb.serverPool.GetStats()

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// stats endpoint with enhanced information
func (lb *LoadBalancer) stats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := lb.serverPool.GetStats()

	// Add additional runtime stats
	extendedStats := map[string]interface{}{
		"load_balancer": stats,
		"config": map[string]interface{}{
			"port":                  lb.config.Port,
			"health_check_interval": lb.config.HealthCheckInterval,
			"max_retries":           lb.config.MaxRetries,
			"algorithm":             lb.config.Algorithm,
		},
		"circuit_breaker": map[string]interface{}{
			"max_consecutive_errors":  10, // Default from backend
			"circuit_timeout_seconds": 30, // Default from backend
		},
		"runtime_info": map[string]interface{}{
			"uptime_seconds": time.Since(time.Now()).Seconds(), // You might want to track actual start time
			"total_requests": "N/A",                            // You could add request counters
		},
		"timestamp": time.Now().Unix(),
	}

	if err := json.NewEncoder(w).Encode(extendedStats); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// circuitBreakerStatus endpoint - enhanced with more details
func (lb *LoadBalancer) circuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	backends := lb.serverPool.GetBackends()
	circuitStatus := make(map[string]interface{})

	totalBackends := len(backends)
	availableBackends := 0
	circuitsOpen := 0

	for _, backend := range backends {
		isAvailable := backend.IsAvailable()
		isCircuitOpen := backend.IsCircuitOpen()

		if isAvailable {
			availableBackends++
		}
		if isCircuitOpen {
			circuitsOpen++
		}

		status := map[string]interface{}{
			"url":                backend.URL.String(),
			"consecutive_errors": backend.GetConsecutiveErrors(),
			"circuit_open":       isCircuitOpen,
			"available":          isAvailable,
			"alive":              backend.IsAlive(),
			"connections":        backend.GetConnections(),
			"weight":             backend.Weight,
		}
		circuitStatus[backend.URL.String()] = status
	}

	response := map[string]interface{}{
		"circuit_breakers": circuitStatus,
		"summary": map[string]interface{}{
			"total_backends":     totalBackends,
			"available_backends": availableBackends,
			"circuits_open":      circuitsOpen,
			"circuits_closed":    totalBackends - circuitsOpen,
			"health_percentage":  float64(availableBackends) / float64(totalBackends) * 100,
		},
		"timestamp": time.Now().Unix(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// Start starts the load balancer server
func (lb *LoadBalancer) Start() {
	// Create a server mux
	mux := http.NewServeMux()
	mux.HandleFunc("/health", lb.healthCheck)
	mux.HandleFunc("/stats", lb.stats)
	mux.HandleFunc("/circuit-breakers", lb.circuitBreakerStatus)
	mux.HandleFunc("/", lb.loadBalance)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", lb.config.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start health checking
	go lb.healthChecking()

	log.Printf("🚀 [START] Load Balancer started at :%s with %s algorithm", lb.config.Port, lb.config.Algorithm)
	log.Printf("🏥 [INFO] Health checks available at /health")
	log.Printf("📊 [INFO] Statistics available at /stats")
	log.Printf("🔌 [INFO] Circuit breaker status available at /circuit-breakers")
	log.Printf("⚙️ [CONFIG] Max retries: %d, Health check interval: %ds",
		lb.config.MaxRetries, lb.config.HealthCheckInterval)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// healthChecking runs periodic health checks on backends
func (lb *LoadBalancer) healthChecking() {
	interval := time.Duration(lb.config.HealthCheckInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial health check
	log.Println("🏥 [HEALTH] Running initial health check...")
	lb.serverPool.HealthCheck()

	for range ticker.C {
		log.Println("🏥 [HEALTH] Running periodic health check...")
		lb.serverPool.HealthCheck()
	}
}
