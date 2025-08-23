package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func (b *Backend) HandleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lrw := &loggingResponseWriter{ResponseWriter: w}

	if b.ShouldFail() {
		http.Error(lrw, "Internal Server Error", http.StatusInternalServerError)
		duration := time.Since(start)
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, lrw.status)
		log.Printf("Response size: %d bytes, duration: %v", lrw.size, duration)
		return
	}

	// Build response payload
	info := map[string]interface{}{
		"method":       r.Method,
		"path":         r.URL.Path,
		"remote_addr":  r.RemoteAddr,
		"backend_type": b.Type,
		"port":         b.Port,
		"hostname":     b.Hostname,
		"request_id":   b.GetRequestCount() + 1,
		"timestamp":    time.Now().Unix(),
	}

	// Send JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(lrw).Encode(info)

	// Log after
	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, lrw.status)
	log.Printf("Response size: %d bytes, duration: %v", lrw.size, duration)
}

func (b *Backend) HandleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lrw := &loggingResponseWriter{ResponseWriter: w}

	if b.ShouldFail() {
		http.Error(lrw, "Internal Server Error", http.StatusInternalServerError)
		duration := time.Since(start)
		b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, lrw.status)
		log.Printf("Response size: %d bytes, duration: %v", lrw.size, duration)
		return
	}

	payload := map[string]interface{}{
		"status":  "healthy",
		"backend": b.Type,
		"port":    b.Port,
		"time":    time.Now().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(lrw).Encode(payload)

	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, lrw.status)
	log.Printf("Response size: %d bytes, duration: %v", lrw.size, duration)
}

func (b *Backend) HandleInfo(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lrw := &loggingResponseWriter{ResponseWriter: w}

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
    <div class="endpoint"><a href="/">/</a> - Request info (JSON)</div>
    <div class="endpoint"><a href="/health">/health</a> - Health check (may fail)</div>
    <div class="endpoint"><a href="/info">/info</a> - Backend information (HTML)</div>
</body>
</html>`,
		b.Type, b.Port, b.Type, b.Port, b.GetRequestCount(), b.GetUptime())

	fmt.Fprint(lrw, html)

	duration := time.Since(start)
	b.LogRequest(r.Method, r.URL.Path, r.RemoteAddr, duration, lrw.status)
	log.Printf("Response size: %d bytes, duration: %v", lrw.size, duration)
}
