package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

func (b *Backend) HandleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), 200)
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "healthy", "backend": "%s", "port": %d}`, b.Type, b.Port)
}

func (b *Backend) HandleFast(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := 200

	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), status)
	}()

	if b.ShouldFail() {
		status = 500
		http.Error(w, "Internal Server Error", status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"endpoint": "fast", "backend": "%s:%d", "timestamp": %d}`,
		b.Type, b.Port, time.Now().Unix())
}

func (b *Backend) HandleSlow(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := 200

	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), status)
	}()

	// Apply delay
	delay := b.GetDelay()
	time.Sleep(delay)

	if b.ShouldFail() {
		status = 500
		http.Error(w, "Internal Server Error", status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"endpoint": "slow", "backend": "%s:%d", "delay_ms": %d, "timestamp": %d}`,
		b.Type, b.Port, delay/time.Millisecond, time.Now().Unix())
}

func (b *Backend) HandleHeavy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := 200

	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), status)
	}()

	if b.ShouldFail() {
		status = 500
		http.Error(w, "Internal Server Error", status)
		return
	}

	delay := b.GetDelay()
	if delay > 0 {
		time.Sleep(delay)
	}

	w.Header().Set("Content-Type", "application/json")
	payload := GeneratePayload(b.PayloadSize)
	w.Write([]byte(payload))
}

func (b *Backend) HandleFail(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := 200

	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), status)
	}()

	// This endpoint has a higher failure rate
	if rand.Float64() < 0.3 { // 30% failure rate
		status = 500
		http.Error(w, "Simulated server error", status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"endpoint": "fail", "backend": "%s:%d", "message": "Lucky! No error this time"}`,
		b.Type, b.Port)
}

func (b *Backend) HandleInfo(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), 200)
	}()

	uptime := b.GetUptime()
	info := map[string]interface{}{
		"backend_type":    b.Type,
		"port":           b.Port,
		"hostname":       b.Hostname,
		"request_count":  b.GetRequestCount(),
		"uptime_seconds": int(uptime.Seconds()),
		"base_delay_ms":  int(b.BaseDelay / time.Millisecond),
		"max_delay_ms":   int(b.MaxDelay / time.Millisecond),
		"payload_size":   b.PayloadSize,
		"error_rate":     b.ErrorRate,
		"start_time":     b.StartTime.Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (b *Backend) HandleCPU(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := 200

	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), status)
	}()

	if b.ShouldFail() {
		status = 500
		http.Error(w, "Internal Server Error", status)
		return
	}

	// CPU intensive task - calculate fibonacci
	n := 35 // Adjust this to make it more/less CPU intensive
	result := Fibonacci(n)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"endpoint": "cpu", "backend": "%s:%d", "fibonacci_%d": %d, "duration_ms": %d}`,
		b.Type, b.Port, n, result, time.Since(start)/time.Millisecond)
}

func (b *Backend) HandleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start), 200)
	}()

	w.Header().Set("Content-Type", "text/html")
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Test Backend - %s:%d</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .endpoint { margin: 10px 0; }
        .endpoint a { text-decoration: none; color: #0066cc; }
        .endpoint a:hover { text-decoration: underline; }
        .info { background: #f5f5f5; padding: 15px; border-radius: 5px; margin: 20px 0; }
    </style>
</head>
<body>
    <h1>Test Backend Server</h1>
    <div class="info">
        <strong>Backend Type:</strong> %s<br>
        <strong>Port:</strong> %d<br>
        <strong>Requests Served:</strong> %d<br>
        <strong>Uptime:</strong> %v
    </div>
    
    <h2>Available Endpoints:</h2>
    <div class="endpoint"><a href="/health">/health</a> - Health check endpoint</div>
    <div class="endpoint"><a href="/fast">/fast</a> - Quick response</div>
    <div class="endpoint"><a href="/slow">/slow</a> - Delayed response</div>
    <div class="endpoint"><a href="/heavy">/heavy</a> - Large payload response</div>
    <div class="endpoint"><a href="/fail">/fail</a> - May return errors</div>
    <div class="endpoint"><a href="/info">/info</a> - Backend information (JSON)</div>
    <div class="endpoint"><a href="/cpu">/cpu</a> - CPU intensive task</div>
</body>
</html>`, 
		b.Type, b.Port, b.Type, b.Port, b.GetRequestCount(), b.GetUptime())

	fmt.Fprint(w, html)
}