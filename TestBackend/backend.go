package main

import (
	"log"
	"math/rand"
	"sync/atomic"
	"time"
)

type Backend struct {
	Port         int
	Type         string
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	PayloadSize  int
	ErrorRate    float64
	RequestCount int64
	StartTime    time.Time
	Hostname     string
}

func NewBackend(port int, backendType string, baseDelay, maxDelay time.Duration, 
	payloadSize int, errorRate float64, hostname string) *Backend {
	return &Backend{
		Port:        port,
		Type:        backendType,
		BaseDelay:   baseDelay,
		MaxDelay:    maxDelay,
		PayloadSize: payloadSize,
		ErrorRate:   errorRate,
		StartTime:   time.Now(),
		Hostname:    hostname,
	}
}

func (b *Backend) LogRequest(method, path, remoteAddr string, duration time.Duration, status int) {
	count := atomic.AddInt64(&b.RequestCount, 1)
	log.Printf("[%s:%d] #%d %s %s from %s -> %d (%v)",
		b.Type, b.Port, count, method, path, remoteAddr, status, duration)
}

func (b *Backend) ShouldFail() bool {
	return rand.Float64() < b.ErrorRate
}

func (b *Backend) GetDelay() time.Duration {
	if b.MaxDelay == 0 {
		return b.BaseDelay
	}
	// Random delay between BaseDelay and MaxDelay
	diff := b.MaxDelay - b.BaseDelay
	return b.BaseDelay + time.Duration(rand.Int63n(int64(diff)))
}

func (b *Backend) GetRequestCount() int64 {
	return atomic.LoadInt64(&b.RequestCount)
}

func (b *Backend) GetUptime() time.Duration {
	return time.Since(b.StartTime)
}