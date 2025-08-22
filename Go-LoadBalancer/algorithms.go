package main

import (
	"sync"
	"sync/atomic"
)

// LoadBalancingAlgorithm defines the interface for load balancing algorithms
type LoadBalancingAlgorithm interface {
	NextBackend(backends []*Backend) *Backend
	Name() string
}

// RoundRobinAlgorithm implements round-robin load balancing
type RoundRobinAlgorithm struct {
	current uint64
}

func (rr *RoundRobinAlgorithm) Name() string {
	return "Round Robin"
}

func (rr *RoundRobinAlgorithm) NextBackend(backends []*Backend) *Backend {
	alive := getAliveBackends(backends)
	if len(alive) == 0 {
		return nil
	}
	
	next := atomic.AddUint64(&rr.current, 1)
	return alive[(next-1)%uint64(len(alive))]
}

// WeightedRoundRobinAlgorithm implements weighted round-robin load balancing
type WeightedRoundRobinAlgorithm struct {
	currentWeights map[*Backend]int
	mux            sync.Mutex
}

func NewWeightedRoundRobinAlgorithm() *WeightedRoundRobinAlgorithm {
	return &WeightedRoundRobinAlgorithm{
		currentWeights: make(map[*Backend]int),
	}
}

func (wrr *WeightedRoundRobinAlgorithm) Name() string {
	return "Weighted Round Robin"
}

func (wrr *WeightedRoundRobinAlgorithm) NextBackend(backends []*Backend) *Backend {
	wrr.mux.Lock()
	defer wrr.mux.Unlock()
	
	alive := getAliveBackends(backends)
	if len(alive) == 0 {
		return nil
	}
	
	// Initialize current weights if not exists
	for _, backend := range alive {
		if _, exists := wrr.currentWeights[backend]; !exists {
			wrr.currentWeights[backend] = 0
		}
	}
	
	// Find backend with highest current weight
	var selected *Backend
	maxWeight := -1
	totalWeight := 0
	
	for _, backend := range alive {
		weight := backend.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}
		totalWeight += weight
		wrr.currentWeights[backend] += weight
		
		if wrr.currentWeights[backend] > maxWeight {
			maxWeight = wrr.currentWeights[backend]
			selected = backend
		}
	}
	
	if selected != nil {
		wrr.currentWeights[selected] -= totalWeight
	}
	
	return selected
}

// LeastConnectionsAlgorithm implements least connections load balancing
type LeastConnectionsAlgorithm struct{}

func (lc *LeastConnectionsAlgorithm) Name() string {
	return "Least Connections"
}

func (lc *LeastConnectionsAlgorithm) NextBackend(backends []*Backend) *Backend {
	var selected *Backend
	minConnections := int64(-1)
	
	for _, backend := range backends {
		if !backend.IsAlive() {
			continue
		}
		
		connections := backend.GetConnections()
		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = backend
		}
	}
	
	return selected
}

// Helper function to get alive backends
func getAliveBackends(backends []*Backend) []*Backend {
	alive := make([]*Backend, 0)
	for _, backend := range backends {
		if backend.IsAlive() {
			alive = append(alive, backend)
		}
	}
	return alive
}

// CreateAlgorithm creates the specified algorithm
func CreateAlgorithm(algorithmType string) LoadBalancingAlgorithm {
	switch algorithmType {
	case "weighted":
		return NewWeightedRoundRobinAlgorithm()
	case "least-connections":
		return &LeastConnectionsAlgorithm{}
	default:
		return &RoundRobinAlgorithm{}
	}
}