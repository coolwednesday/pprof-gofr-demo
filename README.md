# GoFr Pprof & Performance Demo

This project provides a comprehensive environment to learn, simulate, and debug real-world performance issues in Go applications using **pprof**, **Grafana**, and **GoFr**. 

This demo is designed to be used as a hands-on lab or a reference for performance optimization videos.

---

## 🚀 Quick Start

### 1. Prerequisites
- **Go**: 1.25 or later
- **Docker & Docker Compose**: For running Prometheus and Grafana
- **Apache Benchmark (ab)**: (Optional) or use the provided `load.sh` script.

### 2. Start Infrastructure
Launch the observability stack (Prometheus and Grafana):
```bash
docker-compose up -d
```

### 3. Run the Application
Install dependencies and start the Go app:
```bash
go run main.go
```
- **App URL**: [http://localhost:8000](http://localhost:8000)
- **GoFr Metrics**: [http://localhost:2121/metrics](http://localhost:2121/metrics)
- **pprof Endpoints**: [http://localhost:2121/debug/pprof/](http://localhost:2121/debug/pprof/)

---

## 🔍 pprof Exploration Guide

Use the following commands to profile different aspects of the application.

### Phase 1: CPU Profile (Recursive Fibonacci)
**The Problem**: High CPU usage due to inefficient algorithm.
1. **Load**: `./load.sh cpu` (Calls `/cpu-profile?n=40`)
2. **Profile**:
   ```bash
   go tool pprof http://localhost:2121/debug/pprof/profile?seconds=20
   ```
3. **Analysis**: inside pprof, type `top` to find the CPU hog, or `list recursiveFib` to see the line-by-line cost.

### Phase 2: Heap Profile (Memory Leak)
**The Problem**: Global variable holding onto memory.
1. **Load**: `./load.sh mem` (Calls `/mem-profile`)
2. **Profile**:
   ```bash
   go tool pprof http://localhost:2121/debug/pprof/heap
   ```
3. **Analysis**: Look for `leakStorage`. Type `top` or check `-inuse_space`.

### Phase 3: Goroutine Profile (Leaks)
**The Problem**: Goroutines blocked on channels that never receive data.
1. **Load**: `./load.sh goroutine`
2. **Profile**:
   ```bash
   go tool pprof http://localhost:2121/debug/pprof/goroutine
   ```
3. **Analysis**: Notice the ever-increasing count of goroutines in `runtime.gopark`.

### Phase 4: Mutex Contention
**The Problem**: Holding a lock during a slow operation.
1. **Load**: `./load.sh mutex`
2. **Profile**:
   ```bash
   go tool pprof http://localhost:2121/debug/pprof/mutex
   ```
3. **Analysis**: Identifies which lock is causing threads to wait.

### Phase 5: Block Profile
**The Problem**: Channel blocking.
1. **Load**: `./load.sh block`
2. **Profile**:
   ```bash
   go tool pprof http://localhost:2121/debug/pprof/block
   ```

---

## 📊 Grafana Visualization

Access Grafana at [http://localhost:3000](http://localhost:3000) (Login: `admin` / `admin`).

### Performance Dashboard
Go to **Dashboards** -> **GoFr pprof Demo - Performance Dashboard**.
- **CPU Usage**: Watch for spikes during the CPU lab.
- **Memory (RSS)**: Look for the "staircase" pattern in the Memory lab.
- **Goroutines**: Monitor the "Zombie" goroutines climbing.
- **HTTP Latency**: Correlate with Mutex contention.

---

## 🛠 Load Tester Cheat Sheet

Use `./load.sh` with the following arguments:
- `cpu`: Trigger CPU load.
- `mem`: Trigger memory leak.
- `goroutine`: Trigger goroutine leak.
- `mutex`: Trigger lock contention.
- `block`: Trigger channel blocking.
- `all`: Simulation of a production nightmare!

---

## 📺 YouTube Reference
This repository is featured in the [Go Performance Mastering Video](https://youtube.com/link-to-be-added).

---

## 📜 License
This project is licensed under the MIT License.
