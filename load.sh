#!/bin/bash

# ═══════════════════════════════════════════════════════════════════════════
# load.sh — pprof Demo Load Generator
# Fires concurrent HTTP requests to trigger visible spikes in Grafana and pprof
# ═══════════════════════════════════════════════════════════════════════════

URL="http://localhost:8000"

print_usage() {
  echo ""
  echo "Usage: ./load.sh [mode]"
  echo ""
  echo "  Single-shot modes (run once, exit):"
  echo "    cpu        200 concurrent requests to /cpu-profile?n=42"
  echo "               Effect: Significant CPU spike"
  echo ""
  echo "    heavy-cpu  500 concurrent requests to /cpu-profile?n=45"
  echo "               Effect: Extreme CPU load — will hit 100% on multi-core"
  echo ""
  echo "    mem        50 concurrent requests to /mem-profile"
  echo "               Each = 5MB stored in global slice (never freed)"
  echo "               Effect: +250MB staircase pattern in Grafana memory"
  echo ""
  echo "    goroutine  100 concurrent requests to /goroutine-profile"
  echo "               Each = 1 goroutine blocked on channel forever"
  echo "               Effect: goroutine count climbs +100 and stays high"
  echo ""
  echo "    service    10 concurrent requests to /service-call"
  echo "               Each = calls /slow-data (10s sleep) with no timeout"
  echo "               Effect: 10 goroutines stuck for 10s + p99 latency spike"
  echo ""
  echo "    mutex      50 concurrent requests to /mutex-profile"
  echo "               Each = holds global mutex for 100ms"
  echo "               Effect: high latency + visible in mutex pprof profile"
  echo ""
  echo "    block      50 concurrent requests to /block-profile"
  echo "               Each = blocks on channel for 2 seconds"
  echo "               Effect: visible in block pprof profile"
  echo ""
  echo "    alloc      200 rapid requests to /high-alloc"
  echo "               Each = allocates 10MB short-lived objects"
  echo "               Effect: GC pressure — see alloc_space profile + trace"
  echo ""
  echo "    cleanup    Calls /mem-reset to clear memory leak between demo runs"
  echo ""
  echo "    all        Combined chaos: cpu + mem + goroutine + mutex (30-50 each)"
  echo "               Effect: every Grafana panel spikes simultaneously"
  echo ""
  echo "  Continuous modes (loop until Ctrl+C):"
  echo "    watch cpu       Repeat cpu load every 20s — sustained Grafana pattern"
  echo "    watch mem       Repeat mem load every 10s — growing staircase"
  echo "    watch goroutine Repeat goroutine load every 15s"
  echo "    watch mutex     Repeat mutex load every 10s"
  echo ""
  echo "Examples:"
  echo "  ./load.sh cpu            # fire CPU spike once"
  echo "  ./load.sh watch mem      # keep stacking memory leak every 10s"
  echo "  ./load.sh cleanup        # reset memory before next demo run"
  echo ""
}

if [ -z "$1" ]; then
  print_usage
  exit 1
fi

case "$1" in

  cpu)
    echo "══════════════════════════════════════════════════════"
    echo "CPU LOAD — /cpu-profile?n=42"
    echo "Firing 200 concurrent requests..."
    echo "══════════════════════════════════════════════════════"
    for i in {1..200}; do
      curl -s "$URL/cpu-profile?n=42" > /dev/null &
    done
    echo "Load started."
    ;;

  heavy-cpu)
    echo "══════════════════════════════════════════════════════"
    echo "HEAVY CPU LOAD — /cpu-profile?n=45"
    echo "Firing 500 concurrent requests..."
    echo "══════════════════════════════════════════════════════"
    for i in {1..500}; do
      curl -s "$URL/cpu-profile?n=45" > /dev/null &
    done
    echo "Heavy load started."
    ;;

  mem)
    echo "══════════════════════════════════════════════════════"
    echo "MEMORY LOAD — /mem-profile"
    echo "Firing 50 concurrent requests..."
    echo "Each request = 5MB stored globally (never freed by GC)"
    echo "Expected: +250MB in Grafana memory panel (staircase pattern)"
    echo "Also watch: 'mem_leak_size_bytes' gauge panel grow"
    echo "══════════════════════════════════════════════════════"
    for i in {1..50}; do
      curl -s "$URL/mem-profile" > /dev/null &
    done
    echo "Load started. Open Grafana → http://localhost:3000"
    echo "Watch the 'Memory Usage' and 'Memory Leak Gauge' panels."
    ;;

  goroutine)
    echo "══════════════════════════════════════════════════════"
    echo "GOROUTINE LOAD — /goroutine-profile"
    echo "Firing 100 concurrent requests..."
    echo "Each request = 1 goroutine stuck on unbuffered channel (forever)"
    echo "Expected: goroutine count climbs by 100 and NEVER drops"
    echo "══════════════════════════════════════════════════════"
    for i in {1..100}; do
      curl -s "$URL/goroutine-profile" > /dev/null &
    done
    echo "Load started. Open Grafana → http://localhost:3000"
    echo "Watch the 'Goroutine Count' panel — count goes up and stays up."
    ;;

  service)
    echo "══════════════════════════════════════════════════════"
    echo "SERVICE TIMEOUT LOAD — /service-call"
    echo "Firing 10 concurrent requests..."
    echo "Each request = calls /slow-data (which sleeps 10s) with NO timeout"
    echo "Expected: 10 goroutines hang for 10s visible in goroutine count"
    echo "Also visible: p99 HTTP latency spike (10,000ms+)"
    echo "══════════════════════════════════════════════════════"
    for i in {1..10}; do
      curl -s "$URL/service-call" > /dev/null &
    done
    echo "Load started. Open Grafana → http://localhost:3000"
    echo "Watch 'Goroutine Count' and 'HTTP Latency p99' panels."
    ;;

  mutex)
    echo "══════════════════════════════════════════════════════"
    echo "MUTEX LOAD — /mutex-profile"
    echo "Firing 50 concurrent requests..."
    echo "Each request = holds global mutex for 100ms"
    echo "Expected: high HTTP latency (800ms+) despite LOW CPU"
    echo "This is the 'mysterious slowness' pattern — low CPU, high latency"
    echo "══════════════════════════════════════════════════════"
    for i in {1..50}; do
      curl -s "$URL/mutex-profile" > /dev/null &
    done
    echo "Load started."
    echo "Profile: go tool pprof http://localhost:2121/debug/pprof/mutex"
    ;;

  block)
    echo "══════════════════════════════════════════════════════"
    echo "BLOCK LOAD — /block-profile"
    echo "Firing 50 concurrent requests..."
    echo "Each request = blocks on channel for 2 seconds"
    echo "Expected: visible in block pprof profile"
    echo "══════════════════════════════════════════════════════"
    for i in {1..50}; do
      curl -s "$URL/block-profile" > /dev/null &
    done
    echo "Load started."
    echo "Profile: go tool pprof http://localhost:2121/debug/pprof/block"
    ;;

  alloc)
    echo "══════════════════════════════════════════════════════"
    echo "GC PRESSURE LOAD — /high-alloc"
    echo "Firing 200 rapid requests..."
    echo "Each request = 10MB short-lived allocations (not leaked)"
    echo "Expected: high allocation rate → frequent GC cycles"
    echo "══════════════════════════════════════════════════════"
    for i in {1..200}; do
      curl -s "$URL/high-alloc" > /dev/null &
    done
    echo "Load started."
    echo "Profile: go tool pprof --alloc_space http://localhost:2121/debug/pprof/heap"
    echo "Trace:   curl -o trace.out 'http://localhost:2121/debug/pprof/trace?seconds=5'"
    echo "         go tool trace trace.out"
    ;;

  cleanup)
    echo "══════════════════════════════════════════════════════"
    echo "CLEANUP — calling /mem-reset"
    echo "This clears the global leakStorage slice and forces GC."
    echo "Use this between demo runs to start with fresh memory."
    echo "══════════════════════════════════════════════════════"
    result=$(curl -s "$URL/mem-reset")
    echo "Result: $result"
    echo "Memory cleared. Grafana memory panel should drop. Ready for next run."
    ;;

  all)
    echo "══════════════════════════════════════════════════════"
    echo "CHAOS MODE — all bugs firing simultaneously"
    echo "CPU (30) + Memory (30) + Goroutine (50) + Mutex (30)"
    echo "Watch every Grafana panel spike at once!"
    echo "══════════════════════════════════════════════════════"
    echo "Firing CPU load (30 concurrent, n=42)..."
    for i in {1..30}; do curl -s "$URL/cpu-profile?n=42" > /dev/null & done
    echo "Firing Memory load (30 concurrent)..."
    for i in {1..30}; do curl -s "$URL/mem-profile" > /dev/null & done
    echo "Firing Goroutine leak (50 concurrent)..."
    for i in {1..50}; do curl -s "$URL/goroutine-profile" > /dev/null & done
    echo "Firing Mutex contention (30 concurrent)..."
    for i in {1..30}; do curl -s "$URL/mutex-profile" > /dev/null & done
    echo ""
    echo "All load started. Open Grafana → http://localhost:3000"
    echo "Watch: CPU, Memory, Goroutine Count, and HTTP Latency all spike."
    ;;

  watch)
    if [ -z "$2" ]; then
      echo "Usage: ./load.sh watch <mode>"
      echo "Example: ./load.sh watch mem"
      echo "Available modes: cpu, mem, goroutine, mutex"
      exit 1
    fi
    # Validate the sub-mode
    case "$2" in
      cpu|mem|goroutine|mutex|block|alloc) ;;
      *)
        echo "Unknown watch mode: $2"
        echo "Available modes: cpu, mem, goroutine, mutex, block, alloc"
        exit 1
        ;;
    esac
    echo "══════════════════════════════════════════════════════"
    echo "WATCH MODE — looping '$2' load every 15 seconds"
    echo "Press Ctrl+C to stop"
    echo "══════════════════════════════════════════════════════"
    while true; do
      bash "$0" "$2"
      echo ""
      echo "─── Next run in 15 seconds (Ctrl+C to stop) ───"
      sleep 15
    done
    ;;

  *)
    echo "Unknown mode: $1"
    print_usage
    exit 1
    ;;
esac
