// handlers.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// FailureMode represents different ways a backend can fail
type FailureMode struct {
	HealthCheckFails bool          // Health endpoint returns errors
	RequestsFail     bool          // Regular requests fail
	PartialFailure   float64       // Percentage of requests that fail (0.0-1.0)
	SlowResponses    bool          // Responses are artificially slow
	HealthCheckDelay time.Duration // Delay for health checks
}

func (b *Backend) HandleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Check for failure mode
	if b.shouldFailRequest() {
		duration := time.Since(start)
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, 500)
		http.Error(w, "Backend temporarily unavailable", http.StatusInternalServerError)
		return
	}

	// Apply delay
	if delay := b.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	// Apply slow response mode
	if b.FailureMode != nil && b.FailureMode.SlowResponses {
		time.Sleep(2 * time.Second)
	}

	response := map[string]interface{}{
		"message":    "Hello from backend",
		"backend":    fmt.Sprintf("%s:%d", b.Hostname, b.Port),
		"type":       b.Type,
		"timestamp":  time.Now().Format(time.RFC3339),
		"request_id": fmt.Sprintf("%d", b.GetRequestCount()+1),
	}

	// Add payload if specified
	if b.PayloadSize > 0 {
		response["payload"] = strings.Repeat("x", b.PayloadSize)
	}

	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, 200)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (b *Backend) HandleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Apply health check delay if specified
	if b.FailureMode != nil && b.FailureMode.HealthCheckDelay > 0 {
		time.Sleep(b.FailureMode.HealthCheckDelay)
	}

	// Check if health check should fail
	if (b.FailureMode != nil && b.FailureMode.HealthCheckFails) || !b.IsHealthy {
		duration := time.Since(start)
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, 503)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "unhealthy",
			"backend":   fmt.Sprintf("%s:%d", b.Hostname, b.Port),
			"message":   "Health check failed",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, 200)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"backend":   fmt.Sprintf("%s:%d", b.Hostname, b.Port),
		"uptime":    b.GetUptime().String(),
		"requests":  b.GetRequestCount(),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (b *Backend) HandleInfo(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Info endpoint rarely fails, but can be slow
	if b.FailureMode != nil && b.FailureMode.SlowResponses {
		time.Sleep(time.Second)
	}

	info := map[string]interface{}{
		"backend":      fmt.Sprintf("%s:%d", b.Hostname, b.Port),
		"type":         b.Type,
		"uptime":       b.GetUptime().String(),
		"requests":     b.GetRequestCount(),
		"base_delay":   b.BaseDelay.String(),
		"max_delay":    b.MaxDelay.String(),
		"payload_size": b.PayloadSize,
		"error_rate":   b.ErrorRate,
		"is_healthy":   b.IsHealthy,
		"failure_mode": b.FailureMode,
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, 200)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// Control endpoint to change backend behavior during testing
func (b *Backend) HandleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action      string  `json:"action"`       // "fail_health", "fail_requests", "slow", "recover"
		ErrorRate   float64 `json:"error_rate"`   // For partial failures
		HealthDelay int     `json:"health_delay"` // Health check delay in ms
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if b.FailureMode == nil {
		b.FailureMode = &FailureMode{}
	}

	switch req.Action {
	case "fail_health":
		b.FailureMode.HealthCheckFails = true
		b.IsHealthy = false
		log.Printf("[%s:%d] Health checks will now fail", b.Type, b.Port)

	case "fail_requests":
		b.FailureMode.RequestsFail = true
		if req.ErrorRate > 0 {
			b.FailureMode.PartialFailure = req.ErrorRate
		} else {
			b.FailureMode.PartialFailure = 1.0 // 100% failure
		}
		log.Printf("[%s:%d] Requests will now fail (%.1f%% rate)", b.Type, b.Port, b.FailureMode.PartialFailure*100)

	case "slow":
		b.FailureMode.SlowResponses = true
		if req.HealthDelay > 0 {
			b.FailureMode.HealthCheckDelay = time.Duration(req.HealthDelay) * time.Millisecond
		}
		log.Printf("[%s:%d] Responses will now be slow", b.Type, b.Port)

	case "recover":
		b.FailureMode = &FailureMode{}
		b.IsHealthy = true
		log.Printf("[%s:%d] Backend recovered", b.Type, b.Port)

	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"backend": fmt.Sprintf("%s:%d", b.Hostname, b.Port),
		"action":  req.Action,
	})
}
