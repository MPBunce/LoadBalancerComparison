# Load Balancer Comparison: C vs Go

A performance comparison project implementing the same HTTP load balancer in both C and Go, with comprehensive benchmarking tools to analyze their relative performance characteristics.

## Project Structure

```
load-balancer-comparison/
├── C-LoadBalancer/          # C implementation with socket-based networking
├── Go-LoadBalancer/         # Go implementation using standard library
├── TestBackend/             # Simple HTTP backend servers for testing
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
make build

# Start test backend servers
cd TestBackend && ./start-backends.sh

# Run C load balancer
make run-c
# OR manually: ./bin/C-LoadBalancer

# Run Go load balancer  
make run-go
# OR manually: ./bin/Go-LoadBalancer

# Run test backend
make run-backend
# OR manually: ./bin/TestBackend

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