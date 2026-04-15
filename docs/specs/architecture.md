# Architecture Overview

## Purpose

lostromos exports Prometheus-style metrics for policy violations discovered from diverse inputs in a Kubernetes cluster.

## Design Goals

In priority order:

1. **Minimize Kubernetes API server load** — the most important goal. All decisions involving API server interaction (watch scope, read strategies, caching) must treat this as the primary constraint.
2. **Minimize memory usage** — informer caches and state tables should be bounded; stale entries must be evicted.
3. **Minimize CPU usage** — least important; CPU may be traded to keep API server load and memory down.

## Logging

Logging uses the standard library `log/slog` package. The log level is controlled by [`--verbose`](command.md):

- **Default**: warnings and errors only (`slog.LevelWarn`)
- **Verbose** (`--verbose`): all levels enabled (`slog.LevelDebug`)

## Source Layout

| Path | Purpose |
|------|---------|
| `cmd/` | Entry point and command-line parsing — see [command spec](command.md) |
| `config/` | Configuration struct and loading logic — see [command spec](command.md) |
| `input/constraint/` | Gatekeeper constraint watcher input — see [constraint input spec](constraint-input.md) |
| `metrics/` | Metrics handler — see [metrics spec](metrics.md) |