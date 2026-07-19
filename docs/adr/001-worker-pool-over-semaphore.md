# 001 — Worker Pool Over Semaphore

## Status
Accepted

## Context
Needed to fetch health data for 10,000 GPUs concurrently without spawning 10k goroutines.

## Decision
Used a fixed worker pool of 100 goroutines reading from a jobs channel rather than a semaphore throttling 10k goroutines.

## Consequences
- Predictable memory usage regardless of fleet size
- Goroutine count stays bounded at workerPoolSize
- Order of results is non-deterministic (sorted client-side)
