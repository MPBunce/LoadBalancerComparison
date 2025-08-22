package main

import (
	"log"
)

func main() {
	// Configuration
	config := &Config{
		Port:                "3030",
		HealthCheckInterval: 30, // seconds
		MaxRetries:          3,
		Algorithm:           "weighted", // "round-robin", "weighted", "least-connections"
	}

	// Create load balancer
	lb := NewLoadBalancer(config)

	// Add 5 backends with different weights
	backends := []BackendConfig{
		{URL: "http://localhost:3001", Weight: 1},
		{URL: "http://localhost:3002", Weight: 2},
		{URL: "http://localhost:3003", Weight: 3},
		{URL: "http://localhost:3004", Weight: 4},
		{URL: "http://localhost:3005", Weight: 5},
	}

	for _, backend := range backends {
		if err := lb.AddBackend(backend.URL, backend.Weight); err != nil {
			log.Fatalf("Failed to add backend %s: %v", backend.URL, err)
		}
	}

	// Start the load balancer
	log.Printf("Starting load balancer on port %s with %s algorithm", config.Port, config.Algorithm)
	lb.Start()
}
