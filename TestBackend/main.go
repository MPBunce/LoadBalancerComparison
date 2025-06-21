package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	var (
		port        = flag.Int("port", 3000, "Port to listen on")
		backendType = flag.String("type", "balanced", "Backend type (fast, slow, heavy, failing, balanced)")
		baseDelay   = flag.Duration("delay", 0, "Base delay for responses (e.g., 100ms)")
		maxDelay    = flag.Duration("max-delay", 0, "Maximum delay for variable responses (e.g., 500ms)")
		payloadSize = flag.Int("size", 0, "Payload size in bytes for heavy endpoints")
		errorRate   = flag.Float64("error-rate", 0.0, "Error rate (0.0 to 1.0)")
	)
	flag.Parse()

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// Set defaults based on backend type
	switch *backendType {
	case "fast":
		if *baseDelay == 0 {
			*baseDelay = 1 * time.Millisecond
		}
		if *payloadSize == 0 {
			*payloadSize = 100
		}
	case "slow":
		if *baseDelay == 0 {
			*baseDelay = 100 * time.Millisecond
		}
		if *maxDelay == 0 {
			*maxDelay = 500 * time.Millisecond
		}
	case "heavy":
		if *payloadSize == 0 {
			*payloadSize = 1024 * 1024 // 1MB
		}
		if *baseDelay == 0 {
			*baseDelay = 50 * time.Millisecond
		}
	case "failing":
		if *errorRate == 0.0 {
			*errorRate = 0.2 // 20% error rate
		}
		if *baseDelay == 0 {
			*baseDelay = 10 * time.Millisecond
		}
	}

	backend := NewBackend(*port, *backendType, *baseDelay, *maxDelay, *payloadSize, *errorRate, hostname)

	// Setup routes
	http.HandleFunc("/", backend.HandleRoot)
	http.HandleFunc("/health", backend.HandleHealth)
	http.HandleFunc("/fast", backend.HandleFast)
	http.HandleFunc("/slow", backend.HandleSlow)
	http.HandleFunc("/heavy", backend.HandleHeavy)
	http.HandleFunc("/fail", backend.HandleFail)
	http.HandleFunc("/info", backend.HandleInfo)
	http.HandleFunc("/cpu", backend.HandleCPU)

	addr := ":" + strconv.Itoa(*port)
	log.Printf("Starting %s backend server on port %d", *backendType, *port)
	log.Printf("Config: delay=%v, max-delay=%v, payload=%d bytes, error-rate=%.1f%%",
		*baseDelay, *maxDelay, *payloadSize, *errorRate*100)
	log.Printf("Visit http://localhost:%d for endpoint overview", *port)

	log.Fatal(http.ListenAndServe(addr, nil))
    
}