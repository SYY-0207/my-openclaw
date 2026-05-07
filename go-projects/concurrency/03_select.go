package concurrency

import (
	"fmt"
	"time"
)

// RunSelect demonstrates the select statement — Go's concurrency multiplexer.
func RunSelect() {
	fmt.Println("=== 03 Select Statement ===\n")

	// ---- 1. Basic select: wait on multiple channels ----
	fmt.Println("--- 1: basic select — first-ready wins ---")
	fast := make(chan string)
	slow := make(chan string)

	go func() {
		time.Sleep(30 * time.Millisecond)
		fast <- "fast goroutine done"
	}()
	go func() {
		time.Sleep(100 * time.Millisecond)
		slow <- "slow goroutine done"
	}()

	// Listen on both — whichever finishes first is chosen.
	// If both are ready simultaneously, the case is chosen pseudo-randomly.
	for i := 0; i < 2; i++ {
		select {
		case msg := <-fast:
			fmt.Printf("  Got from fast: %s\n", msg)
		case msg := <-slow:
			fmt.Printf("  Got from slow: %s\n", msg)
		}
	}

	// ---- 2. Timeout with time.After ----
	fmt.Println("\n--- 2: timeout ---")
	longTask := make(chan string)
	go func() {
		time.Sleep(200 * time.Millisecond)
		longTask <- "done"
	}()

	select {
	case result := <-longTask:
		fmt.Printf("  Result: %s\n", result)
	case <-time.After(50 * time.Millisecond):
		fmt.Println("  Timed out after 50ms — moving on")
	}

	// ---- 3. Default: non-blocking operations ----
	fmt.Println("\n--- 3: default (non-blocking) ---")
	busy := make(chan int, 1) // buffered so we can put without blocking
	busy <- 1

	// Try to send without blocking
	select {
	case busy <- 2:
		fmt.Println("  Sent 2")
	default:
		fmt.Println("  Channel full — dropped value (non-blocking)")
	}

	// Try to receive without blocking
	select {
	case v := <-busy:
		fmt.Printf("  Received %d\n", v)
	default:
		fmt.Println("  Channel empty — no value available")
	}

	// ---- 4. Select on send and receive together ----
	fmt.Println("\n--- 4: select with both send and receive ---")
	req := make(chan int)
	resp := make(chan int)

	go func() {
		val := <-req
		resp <- val * val
	}()

	select {
	case req <- 7: // Try to send request
	case <-resp: // Never happens first since req hasn't been sent yet
	}
	select {
	case result := <-resp:
		fmt.Printf("  7^2 = %d\n", result)
	}

	// ---- 5. Ticker: periodic work with select ----
	fmt.Println("\n--- 5: ticker ---")
	ticker := time.NewTicker(50 * time.Millisecond)
	done := make(chan bool)

	go func() {
		for {
			select {
			case t := <-ticker.C:
				fmt.Printf("  Tick at %02d:%02d.%03d\n",
					t.Minute(), t.Second(), t.Nanosecond()/1_000_000)
			case <-done:
				return
			}
		}
	}()

	time.Sleep(130 * time.Millisecond)
	ticker.Stop() // Stop the ticker to stop the goroutine's CPU usage
	done <- true
	close(done)

	// ---- 6. Heartbeat pattern ----
	fmt.Println("\n--- 6: heartbeat (health signal) ---")
	heartbeat := make(chan struct{})
	work := make(chan int)

	go func() {
		for i := 1; i <= 3; i++ {
			select {
			case heartbeat <- struct{}{}:
			default:
			}
			time.Sleep(40 * time.Millisecond)
			work <- i
		}
		close(work)
	}()

	for {
		select {
		case _, ok := <-heartbeat:
			if ok {
				fmt.Println("  heartbeat: goroutine alive")
			}
		case v, ok := <-work:
			if !ok {
				fmt.Println("  work channel closed — done")
				return
			}
			fmt.Printf("  work received: %d\n", v)
		}
	}

	fmt.Println()
}
