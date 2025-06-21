# Top-level Makefile for Load Balancer Comparison Project

# Create bin directory if it doesn't exist
$(shell mkdir -p bin)

all: c-lb go-lb test-tools

c-lb:
	cd C-LoadBalancer && make install

go-lb:
	cd Go-LoadBalancer && go build -o ../bin/Go-LoadBalancer

test-tools:
	cd TestBackend && go build -o ../bin/TestBackend simple-server.go
	cd benchmarks && go build -o ../bin/load-generator load-generator.go

benchmark: all
	./scripts/run-tests.sh

clean:
	cd C-LoadBalancer && make clean
	rm -f bin/*

help:
	@echo "Available targets:"
	@echo "  all        - Build all components"
	@echo "  c-lb       - Build C load balancer"
	@echo "  go-lb      - Build Go load balancer"
	@echo "  test-tools - Build test backend and load generator"
	@echo "  benchmark  - Run performance comparison"
	@echo "  clean      - Remove all build artifacts"
	@echo "  help       - Show this help message"

.PHONY: all c-lb go-lb test-tools benchmark clean help