# Go Concurrency Learning Guide

Hands-on examples covering goroutines, channels, and concurrency primitives in Go. Each module is self-contained and runnable.

## Quick Start

```bash
go run . 01      # Goroutines & WaitGroup
go run . 02      # Channels
go run . 03      # Select
go run . 04      # Concurrency patterns
go run . 05      # Context & sync primitives
go run . all     # Everything
```

---

## Module 01 — Goroutines & WaitGroup

### What is a goroutine?

A goroutine is a lightweight thread managed by the Go runtime. It's not an OS thread — Go multiplexes M goroutines onto N OS threads (M:N scheduling). Starting one is as simple as `go fn()`.

Key properties:
- **Cheap**: a few KB of stack that grows/shrinks as needed. You can spawn hundreds of thousands.
- **Cooperative**: goroutines yield at function calls, channel operations, and syscalls — not at arbitrary points.
- **Isolated panics**: a panic in one goroutine does not crash others (as long as you `recover`).

### WaitGroup

`sync.WaitGroup` is the simplest coordination primitive. Think of it as a counter:

1. `wg.Add(n)` — increment the counter (do this **before** launching goroutines)
2. `wg.Done()` — decrement the counter (call with `defer` inside the goroutine)
3. `wg.Wait()` — block until the counter reaches zero

```go
var wg sync.WaitGroup
for i := 0; i < 3; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        // do work
    }(i)
}
wg.Wait()
```

**Common mistake**: calling `wg.Add(1)` inside the goroutine. By then, `wg.Wait()` might already be reached by the main goroutine, and the counter would be 0 — your goroutine never starts.

### Loop variable capture (Go 1.22+)

Before Go 1.22 (February 2024), loop variables were shared across iterations. This was the most famous Go footgun:

```go
// Pre-1.22 TRAP — all goroutines captured the same variable
for i := 0; i < 3; i++ {
    go func() {
        fmt.Println(i) // always printed 3, 3, 3
    }()
}
```

Go 1.22+ fixed this: each iteration now creates a fresh variable. The code above works correctly in this project (Go 1.25). Still, explicitly passing values as arguments makes intent clear and works in any Go version.

**What still races**: shared mutable state. Multiple goroutines reading and writing the same variable without synchronization is always a data race:

```go
// Still a bug in any Go version
shared := 0
for i := 0; i < 3; i++ {
    go func() { shared++ }() // data race!
}
```

### Goroutine crash isolation

A panic inside a goroutine kills only that goroutine — not the whole program. But if you don't `recover`, the panic propagates up the goroutine's stack and crashes the program anyway. Always `recover` in goroutines you don't control the lifetime of:

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("recovered: %v", r)
        }
    }()
    // risky work
}()
```

---

## Module 02 — Channels

### The mental model

A channel is a **typed conduit** — you send and receive values of a specific type through it. Think of it as a thread-safe queue with blocking semantics.

```go
ch := make(chan int)      // unbuffered: capacity 0
ch := make(chan int, 10)  // buffered: capacity 10
```

### Unbuffered channels (synchronous handoff)

An unbuffered channel has capacity 0. Every send **must** be paired with a simultaneous receive:

```
Sender:  ch <- 42         // blocks until receiver is ready
Receiver: val := <-ch     // blocks until sender sends
```

Both goroutines meet at the channel. This is called a **rendezvous** — it synchronizes two goroutines without any other mechanism.

Use unbuffered channels when you need the synchronization guarantee: the sender knows the receiver got the value.

### Buffered channels (async queue)

A buffered channel has a fixed capacity. Sends block only when the buffer is full; receives block only when the buffer is empty:

```go
ch := make(chan string, 3)
ch <- "a"  // doesn't block
ch <- "b"  // doesn't block
ch <- "c"  // doesn't block
ch <- "d"  // blocks — buffer full
```

Use buffered channels when:
- You know the number of values ahead of time (worker pool job queue)
- You want to decouple producer and consumer timing
- You're rate-limiting (a buffered channel of capacity N is a semaphore)

### Directional channels (compile-time safety)

Go lets you restrict channel direction in function signatures:

```go
func sendOnly(ch chan<- int, v int) { ch <- v }   // can only send
func recvOnly(ch <-chan int) int { return <-ch }   // can only receive
```

The compiler enforces this. You can pass a bidirectional channel to a directional parameter — Go automatically converts it — but never the reverse.

### Closing and ranging

`close(ch)` signals "no more values." Sending to a closed channel panics. Receiving from a closed, drained channel returns the zero value immediately with `ok == false`.

```go
close(ch)
v, ok := <-ch   // v == 0, ok == false
```

`range` over a channel reads until the channel is closed AND drained:

```go
for v := range ch {
    // process v
}
// loop exits when ch is closed and all values are consumed
```

**Rule of thumb**: only the sender should close a channel. Closing from the receiver risks a panic if the sender is still sending.

### Channel axioms (memorize these)

| Operation | nil channel | closed channel |
|-----------|-------------|----------------|
| Send | blocks forever | **PANIC** |
| Receive | blocks forever | zero value, immediate |
| Close | **PANIC** | **PANIC** |

This makes nil channels useful in `select`: set a channel to nil to disable that case.

---

## Module 03 — Select

### What select does

`select` is Go's concurrency multiplexer. It waits on multiple channel operations and picks the first one that's ready. If multiple are ready, it picks pseudo-randomly (to prevent starvation).

```go
select {
case v := <-ch1:
    // ch1 had a value
case ch2 <- x:
    // sent x to ch2
case <-time.After(1 * time.Second):
    // timeout
}
```

### Timeout pattern

`time.After` returns a channel that fires after the given duration. Combined with select, you get a deadline on any channel operation:

```go
select {
case result := <-slowOperation:
    fmt.Println(result)
case <-time.After(5 * time.Second):
    fmt.Println("timed out")
}
```

**Beware**: `time.After` creates a timer that won't be garbage-collected until it fires. In a loop, use `time.NewTimer` and `timer.Stop()` instead to avoid memory leaks.

### Non-blocking operations (default)

A `select` with a `default` case never blocks:

```go
select {
case ch <- value:
    // sent
default:
    // channel full, drop or handle
}
```

This is how you do non-blocking sends, receives, and "try-lock" semantics in Go.

### Ticker (periodic work)

`time.NewTicker` returns a channel that fires at regular intervals. Always call `ticker.Stop()` to release resources:

```go
ticker := time.NewTicker(500 * time.Millisecond)
defer ticker.Stop()
for {
    select {
    case <-ticker.C:
        // do periodic work
    case <-done:
        return
    }
}
```

### Heartbeat

A heartbeat is a signal sent on every iteration of a goroutine's work loop. The consumer watches both the heartbeat and the work channel. If heartbeats stop arriving, the goroutine is stalled:

```go
for {
    select {
    case <-heartbeat:
        // goroutine is alive
    case work := <-workChan:
        // process
    case <-time.After(timeout):
        // goroutine is dead
    }
}
```

---

## Module 04 — Concurrency Patterns

### Worker pool

A fixed number of worker goroutines pull jobs from a shared channel. This bounds concurrency and reuses goroutines:

```
            ┌──────────┐
  jobs ───▶ │ worker 1 │ ───▶ results
            │ worker 2 │
            │ worker 3 │
            └──────────┘
```

Key steps:
1. Create a buffered `jobs` channel
2. Launch N workers, each doing `for job := range jobs`
3. Send all jobs, then `close(jobs)` — the workers will drain and exit
4. `wg.Wait()` for workers, then `close(results)`

Closing the jobs channel is the signal to workers that there's no more work.

### Fan-out

One producer, many consumers. All consumers read from the same channel — the Go runtime distributes values across them. This is the simplest way to parallelize independent work.

```
   producer ──┬──▶ consumer 1
              ├──▶ consumer 2
              └──▶ consumer 3
```

### Fan-in (multiplexing)

Many producers, one consumer. Two approaches:

**Select on known channels** (fixed number):
```go
select {
case v := <-ch1:
case v := <-ch2:
}
```

**Merge pattern** (dynamic number): spin up one goroutine per input channel, all feeding into a single output channel. Close the output when all feeders finish:

```go
func merge(channels ...<-chan int) <-chan int {
    out := make(chan int)
    var wg sync.WaitGroup
    for _, ch := range channels {
        wg.Add(1)
        go func(c <-chan int) {
            defer wg.Done()
            for v := range c { out <- v }
        }(ch)
    }
    go func() { wg.Wait(); close(out) }()
    return out
}
```

### Pipeline

Pipeline stages are connected by channels. Each stage receives from an input channel, processes, and sends to an output channel. Stages run concurrently — while stage N processes value X, stage N+1 processes value X-1.

```
gen() ──chan──▶ square() ──chan──▶ consumer
```

Each stage follows the same signature: `func(in <-chan T) <-chan U`. This makes them composable:

```go
nums := gen(2, 3, 4, 5)
squares := square(nums)
result := sum(squares)
```

### Semaphore (rate limiting)

A buffered channel of empty structs acts as a counting semaphore:

```go
sem := make(chan struct{}, 3) // max 3 concurrent
sem <- struct{}{}             // acquire
<-sem                         // release
```

Each goroutine acquires a slot before starting work. When all slots are taken, additional goroutines block on `sem <-`.

### Done channel (graceful shutdown)

Closing a channel broadcasts to all readers simultaneously — every blocked receive on that channel unblocks immediately. This gives you a one-shot shutdown signal:

```go
done := make(chan struct{})

// Producer checks done before each send
go func() {
    for {
        select {
        case ch <- value:
        case <-done:
            return  // stop producing
        }
    }
}()

// Consumer cancels when satisfied
close(done)  // unblocks ALL goroutines reading from done
```

Important: `close(done)` is a one-shot operation. Use `sync.Once` or a flag if multiple goroutines might close it.

---

## Module 05 — Advanced

### Context

`context.Context` is Go's standard way to carry deadlines, cancellation signals, and request-scoped values across API boundaries.

**context.WithCancel**: parent can cancel children:
```go
ctx, cancel := context.WithCancel(context.Background())
go worker(ctx)
cancel()  // all goroutines holding ctx see ctx.Done() close
```

**context.WithTimeout**: auto-cancel after a duration:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()  // always call cancel to free timer resources
```

**context.WithDeadline**: auto-cancel at a specific time. Functionally equivalent to WithTimeout — use whichever reads better for your use case.

**context.WithValue**: attach request-scoped data. Use sparingly and only for data that crosses API boundaries (request IDs, auth tokens, tracing headers). Do NOT use as a general-purpose bag of parameters.

**Cancellation propagation**: a derived context is cancelled when its parent is cancelled. Timeout contexts are cancelled when their parent is cancelled, even if the timeout hasn't expired.

**Rules**:
- Context is always the **first parameter** of a function: `func Do(ctx context.Context, ...)`
- Never store a context in a struct. Pass it explicitly down the call chain.
- `context.Background()` is the root — use it only in `main`, tests, and init.
- `context.TODO()` is a placeholder when you haven't decided which context to use yet.
- Use a custom type for context keys to avoid collisions between packages.

### Mutex vs Channels

Go's proverb: **"Do not communicate by sharing memory; share memory by communicating."**

But mutexes still have their place. Here's how to choose:

| Use channels when | Use mutex/atomic when |
|---|---|
| Passing ownership of data between goroutines | Multiple fields must stay consistent together |
| Coordinating / signalling (done, timeout, tick) | Read-mostly data under high contention (RWMutex) |
| Serializing access to a map (one goroutine owns it) | Simple counters and flags (atomic — lock-free) |

**sync.Mutex**: exclusive lock. Only one goroutine can hold it at a time.

```go
mu.Lock()
counter.value++  // critical section
mu.Unlock()
```

Always prefer `defer mu.Unlock()` right after `Lock()` to ensure unlock on panic.

**sync.RWMutex**: many readers OR one writer. Readers use `RLock/RUnlock`, writers use `Lock/Unlock`. Much faster than a regular Mutex for read-heavy workloads.

### Atomic operations

`sync/atomic` provides lock-free operations on integers and pointers. They're faster than mutexes for simple operations but can't protect multi-variable invariants.

```go
var counter int64
atomic.AddInt64(&counter, 1)         // increment
val := atomic.LoadInt64(&counter)    // read
atomic.StoreInt64(&counter, 0)       // write

// CAS — compare and swap (conditional update)
swapped := atomic.CompareAndSwapInt64(&counter, oldVal, newVal)
```

CAS is the building block for lock-free data structures. It atomically checks if the value equals `oldVal`, and only if so, sets it to `newVal`.

### sync.Once

Execute a function exactly once, even across many goroutines. Perfect for lazy initialization:

```go
var once sync.Once
once.Do(func() { /* runs exactly once */ })
```

Even if multiple goroutines call `Do` simultaneously, the function runs only once, and all callers block until it completes.

### sync.Cond

A condition variable: goroutines wait for a condition to become true. A broadcaster wakes them all:

```go
mu.Lock()
for !ready {  // always check in a loop (spurious wakeups)
    cond.Wait()  // atomically unlocks mu and suspends; re-locks on wake
}
mu.Unlock()

// Elsewhere:
cond.Broadcast()  // wake all waiters
cond.Signal()     // wake one
```

The `for !condition` loop is mandatory — not just for spurious wakeups, but because another goroutine might grab the resource between wake and lock.

### sync.Pool

A cache of temporary objects for reuse. Reduces GC pressure by recycling allocations:

```go
var pool = sync.Pool{
    New: func() any { return make([]byte, 0, 1024) },
}
buf := pool.Get().([]byte)
// ... use buf ...
pool.Put(buf)  // return for reuse
```

Objects in a pool can be collected by GC at any time — don't assume they'll survive. Use for short-lived buffers, not for persistent state.

### Or-Done pattern

Wraps a stream channel with done-channel semantics. Lets you consume any channel with cancellation support:

```go
func orDone(done <-chan struct{}, in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for {
            select {
            case v, ok := <-in:
                if !ok { return }
                out <- v
            case <-done:
                return
            }
        }
    }()
    return out
}

for v := range orDone(done, stream) {
    // process
}
```

### Error group pattern

Launch multiple goroutines and collect the first error. The `golang.org/x/sync/errgroup` package does this with context integration, but the manual version shows the principle:

```go
errCh := make(chan error, len(tasks))  // buffered so senders don't block
var wg sync.WaitGroup
for _, task := range tasks {
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := task(); err != nil {
            errCh <- err
        }
    }()
}
wg.Wait()
close(errCh)
// First error (or nil) is in errCh
```

### Select fairness

When multiple cases in a select are ready simultaneously, Go chooses **pseudo-randomly** — not in declaration order. This prevents a busy channel from starving slower ones. The random distribution is approximately uniform.

---

## Decision Flowchart

```
Need concurrency?
│
├─ Passing data between goroutines? ──▶ Channels
│   ├─ Need sync handoff? ──▶ Unbuffered channel
│   ├─ Want to decouple timing? ──▶ Buffered channel
│   └─ One-to-many? ──▶ Fan-out (one producer, N consumers)
│
├─ Coordinating goroutines?
│   ├─ Wait for all to finish? ──▶ sync.WaitGroup
│   ├─ Periodic work? ──▶ time.Ticker + select
│   ├─ Graceful shutdown? ──▶ Done channel (close to broadcast)
│   └─ Cancellation/deadlines? ──▶ context.Context
│
├─ Protecting shared state?
│   ├─ Simple counter/flag? ──▶ atomic
│   ├─ Complex invariant? ──▶ sync.Mutex
│   ├─ Read-mostly map? ──▶ sync.RWMutex
│   └─ One-time init? ──▶ sync.Once
│
├─ Limiting concurrency?
│   ├─ Fixed worker count? ──▶ Worker pool
│   └─ Ad-hoc limit? ──▶ Buffered channel as semaphore
│
└─ Multiple async results to merge? ──▶ Fan-in (select or merge pattern)
```

---

## Common Pitfalls

1. **Goroutine leak**: starting a goroutine that never exits (e.g., sending to a channel no one reads from). Always ensure goroutines have an exit path — usually via a done channel or context.

2. **Closing from the wrong side**: only the sender should close a channel. A receiver closing can panic if a sender tries to send.

3. **time.After in a loop**: creates a new timer on each iteration that lives until it fires. For long-running loops, use `time.NewTimer` and `Reset` instead.

4. **Copying a sync.Mutex**: `sync.Mutex`, `sync.WaitGroup`, etc. must never be copied (passed by value). Go vet catches this.

5. **Holding a lock during I/O**: never hold a mutex while doing I/O, making a network call, or waiting on a channel — it serializes your entire program.

6. **Empty select**: `select {}` blocks forever with no allocation. Useful for keeping main alive when all work is in background goroutines.

7. **Forgetting defer unlock**: if a function panics between Lock and Unlock, the mutex is deadlocked forever. Always `defer mu.Unlock()`.

## References

- [The Go Memory Model](https://go.dev/ref/mem)
- [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency)
- [Go Concurrency Patterns (Pike, 2012)](https://go.dev/talks/2012/concurrency.slide)
- [Advanced Go Concurrency Patterns (Pike, 2013)](https://go.dev/talks/2013/advconc.slide)
- [Go by Example: Goroutines & Channels](https://gobyexample.com/goroutines)
