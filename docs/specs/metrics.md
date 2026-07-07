# Metrics

## Endpoint

Metrics are served in Prometheus exposition format at the HTTP endpoint configured by [`--metrics-host`, `--metrics-port`, and `--metrics-path`](command.md) (defaults: `0.0.0.0:9090/metrics`).

## Violation Gauge

### `violation`

A gauge vector that represents active policy violations discovered by any input. Each unique combination of labels corresponds to one violating resource under one policy.

**Semantics**

- When a violation is active, the gauge for that label combination is set to `1`.
- When a violation is no longer present, the gauge for that label combination is removed entirely rather than set to `0`. This means the absence of a label set in the output indicates no violation, and stale series do not linger.

**Labels**

| Label | Description |
|-------|-------------|
| `source` | The input that detected the violation |
| `policy` | The name of the violated policy |
| `decision` | The enforcement action or decision associated with the violation (e.g. `warn`, `deny`, `dryrun`) |
| `violation_namespace` | The namespace of the violating resource; empty string for cluster-scoped resources |
| `kind` | The kind of the violating resource (e.g. `Pod`) |
| `name` | The name of the violating resource |

**Example**

A single active violation from the [constraint input](constraint-input.md) might appear as:

```
violation{source="constraint",policy="K8sAllowedRepos",decision="warn",violation_namespace="bar",kind="Pod",name="maximal-security-violation"} 1
```

## Input Interface

Inputs communicate violation state to the metrics handler by sending update messages on a channel. The metrics handler owns the channel and inputs are given a reference to send on.

Each update message carries the full label set that identifies the gauge instance, plus a boolean indicating the operation:

| Field | Type | Description |
|-------|------|-------------|
| `source` | string | The input reporting the violation |
| `policy` | string | The name of the violated policy |
| `decision` | string | The enforcement action (e.g. `warn`, `deny`, `dryrun`) |
| `violation_namespace` | string | Namespace of the violating resource; empty for cluster-scoped |
| `kind` | string | Kind of the violating resource |
| `name` | string | Name of the violating resource |
| `active` | bool | `true` to register/upsert the violation; `false` to remove it |

When `active` is `true`, the metrics handler sets the gauge for that label combination to `1`, creating it if it does not exist. When `active` is `false`, the gauge for that label combination is deleted.

Inputs are responsible for sending a `false` update when they observe that a previously-reported violation is no longer present.
