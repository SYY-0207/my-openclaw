package concurrency

import (
	"fmt"
	"sync"
	"time"
)

// RunPatterns demonstrates classic Go concurrency patterns.
func RunPatterns() {
	fmt.Println("=== 04 Concurrency Patterns ===\n")

	// ---- 1. Worker Pool — fixed N workers process from a shared job queue ----
	fmt.Println("--- 1: worker pool ---")
	const numWorkers = 3
	const numJobs = 7

	jobs := make(chan int, numJobs)
	results := make(chan string, numJobs)

	// Launch workers
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs { // range exits when channel is closed AND drained
				fmt.Printf("    worker %d: processing job %d\n", workerID, job)
				time.Sleep(50 * time.Millisecond) // simulate work
				results <- fmt.Sprintf("job %d → done by worker %d", job, workerID)
			}
		}(w)
	}

	// Send jobs then close so workers can exit
	for j := 1; j <= numJobs; j++ {
		jobs <- j
	}
	close(jobs)

	// Wait for workers to finish, then close results
	wg.Wait()
	close(results)

	fmt.Println("  Results:")
	for r := range results {
		fmt.Printf("    %s\n", r)
	}

	// ---- 2. Fan-out: one producer → many workers ----
	fmt.Println("\n--- 2: fan-out ---")
	input := make(chan int)

	// 5 workers all read from the same channel — work is distributed
	for i := 0; i < 5; i++ {
		go func(id int) {
			for n := range input {
				fmt.Printf("    consumer %d got: %d\n", id, n)
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}

	for n := 0; n < 5; n++ {
		input <- n
	}
	close(input)
	time.Sleep(60 * time.Millisecond) // Let consumers finish printing

	// ---- 3. Fan-in: many producers → one consumer (multiplexing) ----
	fmt.Println("\n--- 3: fan-in (via select on known channels) ---")
	ch1 := producer(1)
	ch2 := producer(2)

	for i := 0; i < 6; i++ { // 3 values each × 2 producers
		select {
		case v := <-ch1:
			fmt.Printf("    Got from producer 1: %d\n", v)
		case v := <-ch2:
			fmt.Printf("    Got from producer 2: %d\n", v)
		}
	}

	// ---- 3b. Fan-in: dynamic number of channels (merge pattern) ----
	fmt.Println("\n--- 3b: fan-in (merge via goroutines) ---")
	ch3 := producer(3)
	ch4 := producer(4)
	ch5 := producer(5)
	merged := merge(ch3, ch4, ch5)
	for v := range merged {
		fmt.Printf("    Merged: %d\n", v)
	}

	// ---- 4. Pipeline: stage1 → stage2 → stage3 ----
	fmt.Println("\n--- 4: pipeline (gen → square → format) ---")

	gen := func(nums ...int) <-chan int {
		out := make(chan int)
		go func() {
			for _, n := range nums {
				out <- n
			}
			close(out)
		}()
		return out
	}

	square := func(in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			for n := range in {
				out <- n * n
			}
			close(out)
		}()
		return out
	}

	sum := func(in <-chan int) int {
		total := 0
		for n := range in {
			total += n
		}
		return total
	}

	// Compose the pipeline
	nums := gen(2, 3, 4, 5)
	squares := square(nums)
	result := sum(squares)
	fmt.Printf("    Sum of squares of 2,3,4,5 = %d\n", result)

	// ---- 5. Semaphore pattern — limit concurrency without a worker pool ----
	fmt.Println("\n--- 5: semaphore (rate limiting) ---")
	sem := make(chan struct{}, 3) // max 3 concurrent goroutines

	const tasks = 8
	var wg2 sync.WaitGroup
	for i := 1; i <= tasks; i++ {
		wg2.Add(1)
		go func(id int) {
			defer wg2.Done()
			sem <- struct{}{}        // Acquire a slot
			defer func() { <-sem }() // Release the slot

			fmt.Printf("    Task %d: running (slots: %d/%d)\n", id, len(sem), cap(sem))
			time.Sleep(40 * time.Millisecond)
		}(i)
	}
	wg2.Wait()
	close(sem)

	// ---- 6. Done channel pattern — graceful shutdown ----
	fmt.Println("\n--- 6: done channel (broadcast shutdown) ---")
	doneCh := make(chan struct{})
	stream := make(chan int)

	// Producer that respects done
	go func() {
		defer close(stream)
		for i := 1; i <= 100; i++ {
			select {
			case stream <- i:
			case <-doneCh:
				fmt.Println("    Producer: shutting down")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Consumer: read first 4 values then cancel
	count := 0
	for v := range stream {
		fmt.Printf("    Consumer: %d\n", v)
		count++
		if count >= 4 {
			close(doneCh) // close broadcasts to all goroutines reading it
			fmt.Println("    Consumer: sent shutdown signal")
			break // Prevent double-close: loop may receive one more value
			       // before the producer's select picks up the closed doneCh
		}
	}
	time.Sleep(20 * time.Millisecond) // Let the producer print its shutdown message

	fmt.Println()
}

// producer sends 3 integers then closes its channel.
func producer(id int) <-chan int {
	ch := make(chan int)
	go func() {
		defer close(ch)
		for i := 0; i < 3; i++ {
			ch <- id*10 + i
		}
	}()
	return ch
}

// merge fans-in multiple channels into one. A goroutine per channel feeds the
// merged output, and a WaitGroup closes the output once all feeders finish.
func merge(channels ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup

	for _, ch := range channels {
		wg.Add(1)
		go func(c <-chan int) {
			defer wg.Done()
			for v := range c {
				out <- v
			}
		}(ch)
	}

	// Close output after all feeders are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
