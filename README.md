# Load Balancer Comparison: C vs Go

A performance comparison project implementing the same HTTP load balancer in both C and Go, with comprehensive benchmarking tools to analyze their relative performance characteristics.

## Project Structure

```
load-balancer-comparison/
├── c-loadbalancer/          # C implementation with socket-based networking
├── go-loadbalancer/         # Go implementation using standard library
├── test-backend/            # Simple HTTP backend servers for testing
├── benchmarks/              # Load testing tools and configuration
└── scripts/                 # Automation and comparison utilities
```

## Features

**Both load balancers implement:**
- Round-robin load balancing
- Health checking of backend servers
- HTTP request forwarding
- Connection pooling
- Basic metrics collection

**Benchmarking includes:**
- Concurrent request handling
- Latency measurements (p50, p95, p99)
- Throughput comparison
- Memory usage analysis
- CPU utilization tracking

## Quick Start

```bash
# Build everything
make all

# Start test backend servers
cd test-backend && ./start-backends.sh

# Run C load balancer
./bin/c-loadbalancer -p 8080 -b backends.conf

# Run Go load balancer
./bin/go-loadbalancer -port 8080 -backends backends.conf

# Run benchmarks
make benchmark
```

## Benchmark Results

Results are automatically generated in `benchmarks/results/` with comparison charts showing:
- Requests per second
- Response time percentiles  
- Memory usage over time
- CPU utilization

## Goals

This project explores the practical performance differences between C and Go for network-intensive applications, providing real-world data on:
- Raw performance characteristics
- Development complexity trade-offs
- Memory efficiency
- Concurrency handling approaches