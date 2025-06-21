package main

import (
	"encoding/json"
	"math/rand"
	"strings"
	"time"
)

func GeneratePayload(size int) string {
	if size <= 0 {
		return `{"message": "Hello from backend", "status": "ok"}`
	}

	// Generate a JSON payload of approximately the requested size
	data := make(map[string]interface{})
	data["message"] = "Hello from backend"
	data["status"] = "ok"
	data["timestamp"] = time.Now().Unix()

	// Fill with dummy data to reach target size
	remaining := size - 100 // Account for other fields
	if remaining > 0 {
		chunks := remaining / 50
		dummyData := make([]string, chunks)
		for i := range dummyData {
			dummyData[i] = strings.Repeat("x", 50)
		}
		data["padding"] = dummyData
	}

	jsonData, _ := json.Marshal(data)
	return string(jsonData)
}

func Fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return Fibonacci(n-1) + Fibonacci(n-2)
}

func init() {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
}