package main

type Config struct {
	Port                string
	HealthCheckInterval int    // seconds
	MaxRetries          int
	Algorithm           string // "round-robin", "weighted", "least-connections"
}

type BackendConfig struct {
	URL    string
	Weight int
}