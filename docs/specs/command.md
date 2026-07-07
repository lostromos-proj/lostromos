# lostromos: Configuration & Invocation Interface

## Overview

This document describes the configuration and invocation interfaces for lostromos. It covers command-line flags and environment variables only. It does not cover internal implementation details or business logic.

## Source Layout

| Path | Purpose |
|------|---------|
| `cmd/` | Entry point and command-line parsing |
| `config/` | Configuration struct and loading logic (flags + env vars) |

See [architecture overview](architecture.md) for the full source layout.

## Invocation

The binary accepts only optional flags. No positional arguments are allowed. Invoking without flags runs with all defaults.

## Command-Line Flags

Flags may be written in either of these equivalent forms:

```
--flag value
--flag=value
```

Boolean flags additionally support bare form, which implies `true`:

```
--disable-constraint-input    # equivalent to --disable-constraint-input=true
--verbose                     # equivalent to --verbose=true
```

### Flag Reference

| Flag | Default | Description |
|------|---------|-------------|
| `--disable-constraint-input` | `false` | Disable the input that watches constraint objects |
| `--verbose` | `false` | Enable verbose logging |
| `--kubeconfig` | _(none)_ | Path to a kubeconfig file; omit to use in-cluster config (falls back to default system location) |
| `--metrics-host` | `0.0.0.0` | Interface to bind for the metrics HTTP endpoint |
| `--metrics-port` | `9090` | Port to bind for the metrics HTTP endpoint |
| `--metrics-path` | `/metrics` | URL path for serving metrics |
| `--help` | — | Print flag and environment variable information, including defaults, then exit immediately |

## Environment Variables

Each flag has an equivalent environment variable. The name is derived by replacing hyphens with underscores and uppercasing. If a flag is explicitly set on the command line, it takes precedence over the corresponding environment variable.

| Environment Variable | Equivalent Flag |
|----------------------|----------------|
| `DISABLE_CONSTRAINT_INPUT` | `--disable-constraint-input` |
| `VERBOSE` | `--verbose` |
| `KUBECONFIG` | `--kubeconfig` |
| `METRICS_HOST` | `--metrics-host` |
| `METRICS_PORT` | `--metrics-port` |
| `METRICS_PATH` | `--metrics-path` |

## Precedence

1. Command-line flag (highest)
2. Environment variable
3. Built-in default (lowest)

## Examples

Run with all defaults (in-cluster):
```sh
lostromos
```

Use a local kubeconfig and enable verbose logging:
```sh
lostromos --kubeconfig=$HOME/.kube/config --verbose
```

Override metrics endpoint via environment:
```sh
METRICS_PORT=8080 lostromos
```

Flag takes precedence over environment variable:
```sh
METRICS_PORT=8080 lostromos --metrics-port=9090
# effective port: 9090
```
