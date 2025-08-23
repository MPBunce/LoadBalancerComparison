package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	backend.ReverseProxy.ErrorHandler = lb.createErrorHandler()

	lb.serverPool.AddBackend(backend)
	return nil
}

func (lb *LoadBalancer) createErrorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(writer http.ResponseWriter, request *http.Request, e error) {
		retries := getRetryFromContext(request)

		// Log the error
		log.Printf(
			"[ERROR] %s %s from %s → backend %s failed: %v (attempt %d/%d)",
			request.Method, request.URL.Path, request.RemoteAddr,
			request.Host, e, retries+1, lb.config.MaxRetries,
		)

		if retries < lb.config.MaxRetries {
			log.Printf(
				"[RETRY] Rerouting %s %s due to backend error, attempt %d/%d",
				request.Method, request.URL.Path, retries+1, lb.config.MaxRetries,
			)
			time.Sleep(10 * time.Millisecond)
			ctx := context.WithValue(request.Context(), retryKey, retries+1)
			lb.loadBalance(writer, request.WithContext(ctx))
			return
		}

		log.Printf("[FAIL] Max retries exceeded for %s %s, returning 503", request.Method, request.URL.Path)
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

func (lb *LoadBalancer) loadBalance(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	peer := lb.serverPool.NextPeer()
	clientIP := r.RemoteAddr

	if peer != nil {
		peer.AddConnection()
		defer peer.RemoveConnection()

		// Detailed request log
		log.Printf(
			"[ROUTE] %s %s from %s → backend %s (connections=%d, weight=%d)",
			r.Method, r.URL.Path, clientIP,
			peer.URL.String(),
			peer.GetConnections(),
			peer.Weight,
		)

		peer.ReverseProxy.ServeHTTP(w, r)

		// Response log with time
		log.Printf(
			"[RESPONSE] %s %s served by %s in %v",
			r.Method, r.URL.Path, peer.URL.String(), time.Since(start),
		)
		return
	}

	log.Printf("[FAIL] No available backend for %s %s from %s", r.Method, r.URL.Path, clientIP)
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

// stats endpoint
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
		},
		"timestamp": time.Now().Unix(),
	}

	if err := json.NewEncoder(w).Encode(extendedStats); err != nil {
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

	log.Printf("[START] Load Balancer started at :%s", lb.config.Port)
	log.Printf("[INFO] Health checks available at /health")
	log.Printf("[INFO] Statistics available at /stats")

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
	log.Println("[HEALTH] Running initial health check...")
	lb.serverPool.HealthCheck()

	for range ticker.C {
		log.Println("[HEALTH] Running periodic health check...")
		lb.serverPool.HealthCheck()
	}
}
