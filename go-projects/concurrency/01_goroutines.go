package concurrency

import (
	"fmt"
	"sync"
	"time"
)

// RunGoroutines demonstrates the basics of goroutines and sync.WaitGroup.
func RunGoroutines() {
	fmt.Println("=== 01 Goroutines & WaitGroup ===\n")

	// ---- 1. Launching a goroutine ----
	// `go fn()` starts a new goroutine — a lightweight thread managed by the Go runtime.
	// The caller does NOT wait unless you synchronize explicitly.
	fmt.Println("--- 1a: goroutine runs concurrently ---")
	go func() {
		fmt.Println("  Hello from goroutine!")
	}()
	time.Sleep(50 * time.Millisecond) // Give the goroutine time to print (NOT for production use)

	// ---- 1b: Without synchronization, main may exit first ----
	fmt.Println("--- 1b: without sync, goroutine may never print ---")
	go func() {
		fmt.Println("  This might not appear (or might — race!)")
	}()
	// No sleep this time — often the program exits before the goroutine runs.

	// ---- 2. sync.WaitGroup — the standard way to wait for goroutines ----
	fmt.Println("\n--- 2: sync.WaitGroup ---")
	var wg sync.WaitGroup // Zero value is ready to use

	for i := 1; i <= 3; i++ {
		wg.Add(1) // Increment counter BEFORE launching the goroutine
		go func(id int) {
			defer wg.Done() // Decrement counter when done (use defer for safety)
			fmt.Printf("  Worker %d: working...\n", id)
			time.Sleep(30 * time.Millisecond)
			fmt.Printf("  Worker %d: done\n", id)
		}(i) // Pass i as argument — captures current value
	}

	wg.Wait() // Block until counter reaches zero
	fmt.Println("  All workers finished")

	// ---- 3. Closure trap: loop variable capture ----
	// Go 1.22+ (2024) fixed this: each loop iteration now creates a new variable.
	// The bug is gone in this codebase (Go 1.25), but you WILL see it in older code.
	fmt.Println("\n--- 3: closure trap (Go 1.22+ fixed this) ---")
	fmt.Println("  Go 1.22+: each iteration gets its own variable — no more trap:")
	for i := 1; i <= 3; i++ {
		go func() {
			fmt.Printf("    Fine now: i = %d\n", i)
		}()
	}
	time.Sleep(50 * time.Millisecond)

	fmt.Println("  Still, passing as argument is explicit and self-documenting:")
	for i := 1; i <= 3; i++ {
		go func(n int) {
			fmt.Printf("    With arg: i = %d\n", n)
		}(i)
	}
	time.Sleep(50 * time.Millisecond)

	// Demonstrate the pre-1.22 trap using a mutable dereference (still a real bug)
	fmt.Println("  Mutable state shared across goroutines — still a real race:")
	shared := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shared++ // data race — multiple goroutines mutate without synchronization
		}()
	}
	wg.Wait()
	fmt.Printf("  Shared counter (racy — value may vary): %d\n", shared)

	// ---- 4. goroutines are M:N scheduled — cheap, not free ----
	fmt.Println("\n--- 4: spawning many goroutines ---")
	const N = 100
	wg.Add(N)
	start := time.Now()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
		}()
	}
	wg.Wait()
	fmt.Printf("  %d goroutines completed in %v\n", N, time.Since(start))

	// ---- 5. Named goroutines with recover (real teams do this) ----
	fmt.Println("\n--- 5: goroutine crash isolation ---")
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("  Recovered from panic: %v\n", r)
			}
		}()
		panic("something went wrong in this goroutine")
	}()
	wg.Wait()
	fmt.Println("  Main is still alive — one goroutine's panic doesn't crash the whole program")

	fmt.Println()
}
