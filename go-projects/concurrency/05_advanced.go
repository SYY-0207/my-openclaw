package concurrency

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// RunAdvanced demonstrates context, mutex, atomic, and other sync primitives.
func RunAdvanced() {
	fmt.Println("=== 05 Advanced: Context & Sync Primitives ===\n")

	// ========================================================================
	// PART A: context.Context — cancellation, deadlines, values
	// ========================================================================

	// ---- 1. context.WithCancel ----
	fmt.Println("--- A1: context.WithCancel ---")
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-time.After(100 * time.Millisecond):
			fmt.Println("  Worker: finished normally")
		case <-ctx.Done():
			fmt.Printf("  Worker: cancelled: %v\n", ctx.Err())
		}
	}()

	time.Sleep(20 * time.Millisecond)
	cancel() // Signal cancellation to all goroutines that hold this ctx
	time.Sleep(20 * time.Millisecond)

	// ---- 2. context.WithTimeout ----
	fmt.Println("\n--- A2: context.WithTimeout ---")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel2() // Always call cancel to release resources (even after timeout)

	select {
	case <-time.After(200 * time.Millisecond):
		fmt.Println("  Work completed")
	case <-ctx2.Done():
		fmt.Printf("  Timeout: %v\n", ctx2.Err())
	}

	// ---- 3. context.WithDeadline (absolute time) ----
	fmt.Println("\n--- A3: context.WithDeadline ---")
	deadline := time.Now().Add(60 * time.Millisecond)
	ctx3, cancel3 := context.WithDeadline(context.Background(), deadline)
	defer cancel3()

	select {
	case <-time.After(200 * time.Millisecond):
		fmt.Println("  Work completed")
	case <-ctx3.Done():
		fmt.Printf("  Deadline exceeded: %v\n", ctx3.Err())
	}

	// ---- 4. context.WithValue (use sparingly — request-scoped data only) ----
	fmt.Println("\n--- A4: context.WithValue ---")
	type contextKey string // Use a custom type, not string, to avoid collisions
	const key contextKey = "user-id"
	ctx4 := context.WithValue(context.Background(), key, "user-42")

	userID, ok := ctx4.Value(key).(string)
	if ok {
		fmt.Printf("  Found user-id in context: %s\n", userID)
	}

	// ---- 5. Deriving from parent — cancellation propagates ----
	fmt.Println("\n--- A5: cancellation propagation ---")
	parentCtx, parentCancel := context.WithCancel(context.Background())
	childCtx, _ := context.WithTimeout(parentCtx, 10*time.Second)

	go func() {
		<-childCtx.Done()
		fmt.Printf("  Child done: %v (parent cancel propagated)\n", childCtx.Err())
	}()

	parentCancel() // Cancels the child too
	time.Sleep(20 * time.Millisecond)

	// ========================================================================
	// PART B: Mutex vs Channels — when to use each
	// ========================================================================

	fmt.Println("\n--- B1: sync.Mutex — exclusive access to shared state ---")
	type Counter struct {
		mu    sync.Mutex
		value int
	}
	counter := &Counter{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.mu.Lock()
			counter.value++ // Only one goroutine can execute this at a time
			counter.mu.Unlock()
		}()
	}
	wg.Wait()
	fmt.Printf("  Counter after 100 increments: %d\n", counter.value)

	// ---- B2: RWMutex — many readers, one writer ----
	fmt.Println("\n--- B2: sync.RWMutex ---")
	type Cache struct {
		mu   sync.RWMutex
		data map[string]string
	}
	cache := &Cache{data: make(map[string]string)}
	cache.mu.Lock()
	cache.data["key"] = "value"
	cache.mu.Unlock()

	// 10 concurrent readers — all hold RLock, no contention
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.mu.RLock()
			_ = cache.data["key"]
			cache.mu.RUnlock()
		}(i)
	}
	wg.Wait()
	fmt.Println("  10 concurrent readers completed — no writer blocked")

	// ---- B3: atomic — lock-free counters & flags ----
	fmt.Println("\n--- B3: sync/atomic — lock-free operations ---")
	var atomCounter int64
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&atomCounter, 1)
		}()
	}
	wg.Wait()
	fmt.Printf("  Atomic counter: %d\n", atomic.LoadInt64(&atomCounter))

	// CAS (Compare And Swap) — conditional update
	var flag int64
	swapped := atomic.CompareAndSwapInt64(&flag, 0, 1)
	fmt.Printf("  CAS: swapped=%v (new vale=%d)\n", swapped, atomic.LoadInt64(&flag))
	swapped = atomic.CompareAndSwapInt64(&flag, 0, 2) // Fails — current is 1, not 0
	fmt.Printf("  CAS: swapped=%v (value still=%d)\n", swapped, atomic.LoadInt64(&flag))

	// ---- B4: sync.Once — execute exactly once ----
	fmt.Println("\n--- B4: sync.Once ---")
	var once sync.Once
	initFn := func() { fmt.Println("  Initialized!") }

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			once.Do(initFn) // Only the first call executes initFn
			fmt.Printf("    Call %d passed once.Do\n", n)
		}(i)
	}
	wg.Wait()

	// ---- B5: sync.Cond — wait for a condition ----
	fmt.Println("\n--- B5: sync.Cond — signal/broadcast ---")
	var condMu sync.Mutex
	cond := sync.NewCond(&condMu)
	ready := false

	// 3 waiting goroutines
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			condMu.Lock()
			for !ready { // Always check condition in a loop (spurious wakeups)
				cond.Wait() // Atomically unlocks mu and suspends; re-locks on wake
			}
			fmt.Printf("    Goroutine %d: ready!\n", id)
			condMu.Unlock()
		}(i)
	}

	time.Sleep(30 * time.Millisecond)
	condMu.Lock()
	ready = true
	condMu.Unlock()
	cond.Broadcast() // Wake all waiters (cond.Signal wakes just one)
	wg.Wait()

	// ---- B6: sync.Pool — reuse objects to reduce GC pressure ----
	fmt.Println("\n--- B6: sync.Pool ---")
	var pool = sync.Pool{
		New: func() any {
			fmt.Println("    Pool: allocating new buffer")
			return make([]byte, 0, 1024)
		},
	}

	buf1 := pool.Get().([]byte)
	pool.Put(buf1) // Return to pool for reuse
	buf2 := pool.Get().([]byte) // Should reuse buf1 — no allocation

	if buf2 != nil {
		fmt.Println("    Pool: got buffer (may be recycled)")
	}

	// ========================================================================
	// PART C: Advanced patterns
	// ========================================================================

	// ---- C1: Or-Done — multiplex cancellation & stream ----
	fmt.Println("\n--- C1: or-done pattern ---")
	data := make(chan int)
	doneOr := make(chan struct{})

	go func() {
		defer close(data)
		for i := 1; i <= 10; i++ {
			select {
			case data <- i:
			case <-doneOr:
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Wrapper that adds done-channel semantics to any stream
	orDone := func(done <-chan struct{}, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for {
				select {
				case v, ok := <-in:
					if !ok {
						return
					}
					out <- v
				case <-done:
					return
				}
			}
		}()
		return out
	}

	count := 0
	for v := range orDone(doneOr, data) {
		fmt.Printf("  %d", v)
		count++
		if count >= 4 {
			close(doneOr)
		}
	}
	fmt.Println()

	// ---- C2: Random select distribution — verify it's uniform-ish ----
	fmt.Println("\n--- C2: select fairness demo ---")
	chA, chB := make(chan bool), make(chan bool)
	doneC2 := make(chan struct{})

	go func() {
		for {
			select {
			case chA <- true:
			case chB <- true:
			case <-doneC2:
				close(chA)
				close(chB)
				return
			}
		}
	}()

	var aCount, bCount int64
	const trials = 20
	for i := 0; i < trials; i++ {
		select {
		case <-chA:
			atomic.AddInt64(&aCount, 1)
		case <-chB:
			atomic.AddInt64(&bCount, 1)
		}
	}
	close(doneC2)
	fmt.Printf("  A wins: %d, B wins: %d (out of %d)\n", aCount, bCount, trials)
	fmt.Println("  Note: Select chooses pseudo-randomly when multiple cases are ready")

	// ---- C3: errgroup pattern (manual, without golang.org/x/sync) ----
	fmt.Println("\n--- C3: error group (first error wins) ---")
	errCh := make(chan error, 3) // Buffered so senders don't block

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			d := time.Duration(rand.Intn(80)) * time.Millisecond
			time.Sleep(d)
			if id == 2 {
				errCh <- fmt.Errorf("goroutine %d failed", id)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		fmt.Printf("  First error: %v\n", err)
	} else {
		fmt.Println("  All goroutines succeeded")
	}

	// ========================================================================
	// PART D: Decision guide
	// ========================================================================
	fmt.Println("\n--- Decision guide ---")
	fmt.Println("  Share memory by communicating (channels first):")
	fmt.Println("    → Passing ownership (pipeline, fan-in/out)")
	fmt.Println("    → Coordinating / signalling (done, ticker, timeout)")
	fmt.Println("    → Serializing access (one goroutine owns the map)")
	fmt.Println("  Share memory via synchronization (mutex/atomic):")
	fmt.Println("    → High-contention read-mostly caches (RWMutex)")
	fmt.Println("    → Simple counters and flags (atomic)")
	fmt.Println("    → Complex invariants across multiple fields (Mutex)")

	fmt.Println()
}
