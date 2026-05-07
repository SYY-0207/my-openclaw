package main

import (
	"fmt"
	"go-projects/concurrency"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <module>")
		fmt.Println()
		fmt.Println("Modules:")
		fmt.Println("  01  Goroutines & WaitGroup")
		fmt.Println("  02  Channels (unbuffered, buffered, directional, close, range)")
		fmt.Println("  03  Select (multiplexing, timeout, default, ticker, heartbeat)")
		fmt.Println("  04  Patterns (worker pool, fan-in, fan-out, pipeline, semaphore, done)")
		fmt.Println("  05  Advanced (context, mutex, atomic, Once, Cond, Pool, or-done)")
		fmt.Println("  all  Run all modules sequentially")
		return
	}

	switch os.Args[1] {
	case "01":
		concurrency.RunGoroutines()
	case "02":
		concurrency.RunChannels()
	case "03":
		concurrency.RunSelect()
	case "04":
		concurrency.RunPatterns()
	case "05":
		concurrency.RunAdvanced()
	case "all":
		concurrency.RunGoroutines()
		concurrency.RunChannels()
		concurrency.RunSelect()
		concurrency.RunPatterns()
		concurrency.RunAdvanced()
	default:
		fmt.Printf("Unknown module: %s\n", os.Args[1])
	}
}
