package main

import (
	// "context" // ← uncomment this import when applying the service-call or goroutine fix
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"gofr.dev/pkg/gofr"
)

// Global variables for leak simulations
var (
	leakStorage [][]byte
	mu          sync.Mutex
	items       []int
)

const (
	memLeakMetric = "mem_leak_size_bytes"
)

func main() {
	app := gofr.New()

	// Enable Mutex and Block profiling (must be set once at startup)
	// These are OFF by default — they add minimal overhead only when profiling
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	// Register a custom Prometheus gauge for the memory leak demo
	// Visible in Grafana as a dedicated panel to track leak growth
	app.Metrics().NewGauge(memLeakMetric, "Current size of memory leak in bytes")

	// Register external HTTP service (self-referencing for the service-timeout demo)
	// This lets us call /slow-data on ourselves to simulate a slow upstream dependency
	app.AddHTTPService("external-api", "http://localhost:8000")

	// ─────────────────────────────────────────────────────────────────────────
	// SLOW UPSTREAM SIMULATOR
	// Used by /service-call to simulate a laggy dependency (e.g. a slow DB or API)
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/slow-data", func(ctx *gofr.Context) (interface{}, error) {
		time.Sleep(10 * time.Second)
		return "slow data", nil
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BUG #1 ── CPU PROFILE
	//
	// THE BUG:
	//   Recursive Fibonacci has O(2^n) exponential time complexity.
	//   fib(42) requires ~268 million recursive calls.
	//   Under concurrent load, this pins the CPU to 100%+.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh cpu
	//   Watch: CPU panel in Grafana spikes near 100%
	//   Profile: go tool pprof http://localhost:2121/debug/pprof/profile?seconds=30
	//   In pprof: (pprof) top  →  recursiveFib shows ~99% flat time
	//
	// HOW TO FIX (LIVE ON STAGE):
	//   1. Comment out the BUGGY line below
	//   2. Uncomment the FIX line below
	//   3. Save file → go run main.go
	//   4. Run ./load.sh cpu again → CPU drops to near 0%
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/cpu-profile", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Info("Triggering CPU-intensive task")

		n := ctx.Param("n")
		if n == "" {
			n = "42"
		}

		// ══════════════════════════════════════════════════════════════════
		// BUGGY (active) ─ O(2^n) exponential recursive algorithm
		result := recursiveFib(stringToInt(n))
		// ── FIX: comment the line above, uncomment the line below ─────────
		// result := iterativeFib(stringToInt(n)) // O(n) linear — instant ✅
		// ══════════════════════════════════════════════════════════════════

		return result, nil
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BUG #2 ── HEAP / MEMORY PROFILE
	//
	// THE BUG:
	//   Every request allocates 5MB and appends it to a global slice (leakStorage).
	//   Because leakStorage is a global variable, the GC can never collect the data.
	//   Memory grows 5MB per request and never comes back down.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh mem
	//   Watch: Memory panel in Grafana shows a staircase climbing pattern
	//   Also watch: mem_leak_size_bytes gauge panel grows
	//   Profile: go tool pprof --inuse_space http://localhost:2121/debug/pprof/heap
	//   In pprof: (pprof) top  →  main.main.funcX dominates inuse_space
	//
	// HOW TO FIX (LIVE ON STAGE):
	//   1. Comment out the BUGGY block (lines marked BUG)
	//   2. Uncomment the FIX block (lines marked FIX)
	//   3. Save file → go run main.go
	//   4. Run ./load.sh mem again → memory stays flat
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/mem-profile", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Warn("Memory allocation triggered")

		const size = 5 * 1024 * 1024 // 5 MB per request

		// ══════════════════════════════════════════════════════════════════
		// BUGGY (active) ─ allocate and store globally (never freed by GC)
		data := make([]byte, size)
		for i := 0; i < size; i += 1024 { // partial fill to ensure physical allocation
			data[i] = byte(rand.Intn(256))
		}
		mu.Lock()
		leakStorage = append(leakStorage, data) // BUG: global ref prevents GC collection
		currentSize := float64(len(leakStorage) * size)
		mu.Unlock()
		app.Metrics().SetGauge(memLeakMetric, currentSize)
		return fmt.Sprintf("Allocated %d MB total (leaking)", len(leakStorage)*5), nil
		// ── FIX: comment out the BUGGY block above, uncomment below ───────
		// data := make([]byte, size)
		// for i := 0; i < size; i += 1024 {
		// 	data[i] = byte(rand.Intn(256))
		// }
		// // FIX: Process data locally. No global reference → GC can collect ✅
		// _ = data // simulate using the data; it goes out of scope after return
		// return "Processed 5MB locally — memory freed after handler returns", nil
		// ══════════════════════════════════════════════════════════════════
	})

	// Housekeeping endpoint — NOT part of the bug demo.
	// Resets the memory leak between demo runs so you start fresh.
	// Call: ./load.sh cleanup  (or: curl localhost:8000/mem-reset)
	app.GET("/mem-reset", func(ctx *gofr.Context) (interface{}, error) {
		mu.Lock()
		leakStorage = nil // remove all global references
		runtime.GC()      // force GC to immediately reclaim the freed memory
		mu.Unlock()
		app.Metrics().SetGauge(memLeakMetric, 0)
		return "Memory leak cleared and GC forced — ready for fresh demo", nil
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BUG #3 ── GOROUTINE PROFILE
	//
	// THE BUG:
	//   Each request spawns a goroutine that blocks on an unbuffered channel.
	//   Nobody ever sends to that channel, so the goroutine waits forever.
	//   Every request adds 1 permanently-blocked (zombie) goroutine.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh goroutine
	//   Watch: Goroutine Count panel in Grafana climbs by 100 and NEVER drops
	//   Profile: go tool pprof http://localhost:2121/debug/pprof/goroutine
	//   In pprof: (pprof) top  →  runtime.gopark/chanrecv showing hundreds of goroutines
	//   Or: curl localhost:2121/debug/pprof/goroutine?debug=1  →  see full stack traces
	//
	// HOW TO FIX (LIVE ON STAGE):
	//   1. Comment out the BUGGY block below (the bare go func + channel)
	//   2. Uncomment the FIX block below (select + context + timeout)
	//   3. Save file → go run main.go → restart resets goroutine count
	//   4. Run ./load.sh goroutine again → goroutine count stays stable
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/goroutine-profile", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Debug("Spawning goroutine")

		// ══════════════════════════════════════════════════════════════════
		// BUGGY (active) ─ goroutine blocks on channel forever (no sender)
		go func() {
			ch := make(chan int) // unbuffered channel
			<-ch                // BUG: nobody ever sends here → goroutine stuck forever
		}()
		return "Goroutine leaked (blocked on channel with no sender)", nil
		// ── FIX: comment out the BUGGY block above, uncomment below ───────
		// go func(c context.Context) {
		// 	select {
		// 	case <-time.After(5 * time.Second): // FIX: guaranteed exit after 5s ✅
		// 		fmt.Println("Goroutine finished normally")
		// 	case <-c.Done(): // FIX: exits immediately if request is cancelled ✅
		// 		fmt.Println("Goroutine cancelled by context")
		// 	}
		// }(ctx.Context)
		// return "Safe goroutine started — will exit in ≤5s", nil
		// ══════════════════════════════════════════════════════════════════
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BUG #4 ── SERVICE CALL / GOROUTINE LEAK VIA NO TIMEOUT
	//
	// THE BUG:
	//   We call an external service (our own /slow-data) without a deadline.
	//   /slow-data sleeps for 10 seconds before responding.
	//   With no timeout, each request goroutine is stuck waiting for the response.
	//   Under concurrent load: 10 goroutines all wait 10s → visible goroutine spike.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh service
	//   Watch: Goroutine Count and HTTP Latency p99 spike in Grafana
	//   Profile: go tool pprof http://localhost:2121/debug/pprof/goroutine
	//   In pprof: goroutines stuck in net/http transport read
	//
	// HOW TO FIX (LIVE ON STAGE):
	//   1. Comment out the BUGGY block (ctx, no timeout)
	//   2. Uncomment the FIX block (subCtx with 2s timeout)
	//   3. Save file → go run main.go
	//   4. Run ./load.sh service again → requests fail fast (timeout error) at 2s
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/service-call", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Info("Making external service call")

		// ══════════════════════════════════════════════════════════════════
		// BUGGY (active) ─ no timeout on the downstream HTTP call
		resp, err := ctx.GetHTTPService("external-api").Get(ctx, "slow-data", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return "Response received (no timeout — this goroutine was stuck for 10s)", nil
		// ── FIX: comment out the BUGGY block above, uncomment below ───────
		// subCtx, cancel := context.WithTimeout(ctx, 2*time.Second) // FIX: 2s deadline ✅
		// defer cancel() // FIX: always clean up the context to avoid leak ✅
		// resp, err := ctx.GetHTTPService("external-api").Get(subCtx, "slow-data", nil)
		// if err != nil {
		// 	return nil, err // FIX: returns context.DeadlineExceeded — goroutine exits cleanly ✅
		// }
		// defer resp.Body.Close()
		// return "Response received with 2s timeout protection", nil
		// ══════════════════════════════════════════════════════════════════
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BUG #5 ── MUTEX PROFILE
	//
	// THE BUG:
	//   The global mutex (mu) is acquired and then held for 100ms while "working".
	//   Under concurrent load (50 goroutines), all of them fight for the same lock.
	//   49 goroutines queue up and wait → high latency despite low CPU usage.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh mutex
	//   Watch: HTTP latency panel spikes (high latency + low CPU = lock contention)
	//   Profile: go tool pprof http://localhost:2121/debug/pprof/mutex
	//   In pprof: (pprof) top  →  sync.(*Mutex).Lock shows very high contention time
	//
	// HOW TO FIX (LIVE ON STAGE):
	//   1. Comment out the BUGGY block below
	//   2. Uncomment the FIX block (do slow work BEFORE acquiring the lock)
	//   3. Save file → go run main.go
	//   4. Run ./load.sh mutex again → mutex profile shows minimal contention
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/mutex-profile", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Info("Triggering mutex contention")

		// ══════════════════════════════════════════════════════════════════
		// BUGGY (active) ─ lock held during slow work (simulated with sleep)
		mu.Lock()
		defer mu.Unlock()
		time.Sleep(100 * time.Millisecond) // BUG: holding lock during 100ms of "work"
		items = append(items, rand.Int())
		return "Lock held for 100ms — every concurrent request had to wait for this", nil
		// ── FIX: comment out the BUGGY block above, uncomment below ───────
		// n := rand.Int()
		// time.Sleep(100 * time.Millisecond) // FIX: slow work done OUTSIDE the lock ✅
		// mu.Lock()                          // FIX: lock held for nanoseconds only ✅
		// items = append(items, n)
		// mu.Unlock()
		// return "Work done outside lock — lock held for <1 microsecond", nil
		// ══════════════════════════════════════════════════════════════════
	})

	// ─────────────────────────────────────────────────────────────────────────
	// BLOCK PROFILE DEMO
	//
	// PURPOSE:
	//   Demonstrates what the Block profile looks like.
	//   This is NOT a bug — it's intentional blocking to show the profiler.
	//   A goroutine is spawned that sends on a channel after 2 seconds.
	//   The handler blocks waiting to receive from that channel.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh block
	//   Profile: go tool pprof http://localhost:2121/debug/pprof/block
	//   In pprof: (pprof) top  →  runtime.chanrecv shows ~2s blocking time per request
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/block-profile", func(ctx *gofr.Context) (interface{}, error) {
		ctx.Info("Triggering blocking channel operation (2s)")
		ch := make(chan bool)
		go func() {
			time.Sleep(2 * time.Second) // simulates slow I/O or delayed work
			ch <- true
		}()
		<-ch // handler goroutine blocks here for 2 seconds — block profiler captures this
		return "Channel unblocked after 2s — visible in block pprof profile", nil
	})

	// ─────────────────────────────────────────────────────────────────────────
	// GC PRESSURE DEMO
	//
	// PURPOSE:
	//   Creates 10MB of short-lived allocations per request.
	//   These are NOT leaked (no global reference held).
	//   But the high allocation rate causes frequent GC cycles.
	//   Use to demonstrate alloc_space heap profile and trace GC pauses.
	//
	// HOW TO SEE IT:
	//   Run: ./load.sh alloc  (fires many rapid requests)
	//   Profile: go tool pprof --alloc_space http://localhost:2121/debug/pprof/heap
	//   Trace:   curl -o trace.out http://localhost:2121/debug/pprof/trace?seconds=5
	//            go tool trace trace.out  →  see blue GC Stop-The-World bars
	// ─────────────────────────────────────────────────────────────────────────
	app.GET("/high-alloc", func(ctx *gofr.Context) (interface{}, error) {
		for i := 0; i < 10000; i++ {
			_ = make([]byte, 1024) // 1KB × 10000 = 10MB; all eligible for GC immediately
		}
		return "Allocated 10MB in short-lived objects — check alloc_space profile and GC trace", nil
	})

	app.Run()
}

// recursiveFib: O(2^n) exponential time complexity.
// fib(42) requires ~268 million recursive calls.
// This is the BUGGY implementation used in /cpu-profile.
func recursiveFib(n int) int {
	if n <= 1 {
		return n
	}
	return recursiveFib(n-1) + recursiveFib(n-2)
}

// iterativeFib: O(n) linear time complexity.
// fib(42) requires exactly 41 iterations.
// This is the FIX for /cpu-profile — uncomment to use.
func iterativeFib(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

func stringToInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
