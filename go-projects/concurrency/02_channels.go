package concurrency

import (
	"fmt"
	"time"
)

// RunChannels demonstrates channel fundamentals.
func RunChannels() {
	fmt.Println("=== 02 Channels ===\n")

	// ---- 1. Unbuffered channel: synchronous handoff ----
	// Sender blocks until receiver is ready; receiver blocks until sender sends.
	fmt.Println("--- 1: unbuffered channel (synchronous) ---")
	ch := make(chan int) // unbuffered: capacity 0

	// Start a sender in a goroutine, then receive in main
	go func() {
		fmt.Println("  Sender: about to send 42...")
		ch <- 42 // Blocks until main is ready to receive
		fmt.Println("  Sender: 42 sent")
	}()

	time.Sleep(30 * time.Millisecond) // Simulate "work" before receiving
	fmt.Println("  Receiver: waiting...")
	val := <-ch
	fmt.Printf("  Receiver: got %d\n", val)

	// ---- 2. Buffered channel: async up to capacity ----
	// Sender only blocks when buffer is full; receiver only blocks when empty.
	fmt.Println("\n--- 2: buffered channel ---")
	buf := make(chan string, 3) // capacity 3

	buf <- "a" // These do NOT block — buffer has room
	buf <- "b"
	buf <- "c"
	// buf <- "d" // This WOULD block — buffer full

	fmt.Printf("  Buffered channel: len=%d cap=%d\n", len(buf), cap(buf))
	fmt.Println("  Reading:", <-buf)
	fmt.Println("  Reading:", <-buf)
	fmt.Println("  Reading:", <-buf)

	// ---- 3. Channel direction types ----
	// Compile-time enforcement of send-only or receive-only channels.
	fmt.Println("\n--- 3: directional channels ---")

	// sendOnly accepts a send-only channel
	sendOnly := func(ch chan<- int, v int) {
		ch <- v // ok
		// x := <-ch // compile error: cannot receive from send-only channel
	}

	// recvOnly accepts a receive-only channel
	recvOnly := func(ch <-chan int) int {
		// ch <- 1 // compile error: cannot send to receive-only channel
		return <-ch
	}

	dch := make(chan int, 1)
	sendOnly(dch, 99)
	fmt.Printf("  Sent then received: %d\n", recvOnly(dch))

	// ---- 4. Closing channels & ranging ----
	// Close signals "no more values"; readers can detect via comma-ok or range.
	fmt.Println("\n--- 4: close & range ---")
	numbers := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		numbers <- i
	}
	close(numbers) // Close so range knows when to stop

	// Range over channel — exits when channel is closed AND drained
	for n := range numbers {
		fmt.Printf("  %d", n)
	}
	fmt.Println()

	// Comma-ok pattern: detect close without ranging
	fmt.Print("  Comma-ok check on closed channel: ")
	_, ok := <-numbers
	fmt.Printf("ok=%v (false = closed & drained)\n", ok)

	// ---- 5. Nil channel: blocks forever ----
	// Useful in select to disable a case temporarily.
	fmt.Println("\n--- 5: nil channel blocks forever ---")
	var nilCh chan int
	select {
	case nilCh <- 1:
		fmt.Println("  sent (won't happen)")
	case <-nilCh:
		fmt.Println("  received (won't happen)")
	default:
		fmt.Println("  Both cases skipped — nil channel never ready")
	}

	// ---- 6. Channel axioms (quick reference) ----
	fmt.Println("\n--- 6: channel axiom reference ---")
	fmt.Println("  send to nil chan   → blocks forever")
	fmt.Println("  recv from nil chan  → blocks forever")
	fmt.Println("  send to closed chan → PANIC")
	fmt.Println("  recv from closed    → zero value, ok=false (immediate)")
	fmt.Println("  close nil chan      → PANIC")
	fmt.Println("  close closed chan   → PANIC")

	fmt.Println()
}
