# Top-level Makefile for Load Balancer Comparison Project

# Create bin directory if it doesn't exist
$(shell mkdir -p bin)

build:
	cd C-LoadBalancer && make install
	cd Go-LoadBalancer && go build -o ../bin/Go-LoadBalancer
	cd TestBackend && go build -o ../bin/TestBackend

run-c:
	./Scripts/run_backends.sh
	./Scripts/test_loadbalancer_C.sh

run-go:
	./Scripts/run_backends.sh
	./Scripts/test_loadbalancer_Go.sh

stop:
	pkill -f "C-LoadBalancer" || true
	pkill -f "Go-LoadBalancer" || true
	pkill -f "TestBackend" || true
	pkill -f "load-generator" || true

clean:
	cd C-LoadBalancer && make clean
	rm -f bin/*
	rm -f *.log

.PHONY: build run-c run-go stop clean