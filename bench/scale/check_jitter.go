// +build ignore

// check_jitter simulates a real-world check plugin with random 1-5 second execution time.
// Build: go build -o check_jitter bench/scale/check_jitter.go
package main

import (
	"fmt"
	"math/rand"
	"time"
)

func main() {
	delay := time.Duration(1+rand.Intn(5)) * time.Second
	time.Sleep(delay)
	fmt.Printf("OK - simulated check completed in %s | time=%.3fs;5;10;0;10\n", delay, delay.Seconds())
}
