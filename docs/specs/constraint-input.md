# Constraint Input

## Overview

The constraint input watches Gatekeeper constraint objects for policy violations and makes them available for reporting. It is enabled by default and controlled by `--disable-constraint-input` / `DISABLE_CONSTRAINT_INPUT`.

## Background

Gatekeeper uses a two-level object model:

- A **ConstraintTemplate** (`templates.gatekeeper.sh/v1`, kind `ConstraintTemplate`) defines a policy type. When applied to the cluster, Gatekeeper generates a corresponding CRD under the `constraints.gatekeeper.sh` group.
- A **Constraint** is an instance of one of those generated CRDs. Its `status.violations` field is populated by Gatekeeper's audit controller with the list of currently violating resources.

Because constraint kinds are generated dynamically and may be added or removed over time, the set of CRDs to watch is not known at startup and must be derived at runtime from the live set of ConstraintTemplates.

## Watch Lifecycle

### 1. Watch ConstraintTemplates

On startup, establish an informer watch on ConstraintTemplates:

- **Group/Version/Kind**: `templates.gatekeeper.sh/v1`, kind `ConstraintTemplate`

For each ConstraintTemplate, derive the constraint kind from:

```yaml
spec:
  crd:
    spec:
      names:
        kind: K8sAllowedRepos   # ← the constraint kind
```

### 2. Set Up a Constraint Watch Per Kind

For each constraint kind discovered, set up an informer watch on:

- **Group**: `constraints.gatekeeper.sh`
- **Version**: `v1beta1`
- **Kind**: the value from `spec.crd.spec.names.kind`

The constraint watch should be backed by an informer cache to minimize API server load.

### 3. CRD Availability Delay

There is an arbitrary delay between when a ConstraintTemplate is added or modified and when Gatekeeper generates the corresponding CRD. Attempts to set up a watch before the CRD exists will fail.

When watch setup fails because the CRD is not yet available, retry with exponential backoff until the watch is successfully established. The retry loop for a given kind runs independently and does not block processing of other ConstraintTemplates.

### 4. ConstraintTemplate Removal

When a ConstraintTemplate is deleted, tear down the informer watch for its corresponding constraint kind and discard any cached state for that kind.

### 5. ConstraintTemplate Update

When a ConstraintTemplate is updated, check whether `spec.crd.spec.names.kind` has changed. If it has, tear down the watch for the old kind and establish one for the new kind (subject to the same CRD availability retry described above).

## Extracting Violations

Each constraint object carries its current violations under `status.violations`. A constraint with no violations has an empty or absent list.

```yaml
status:
  totalViolations: 1
  violations:
    - enforcementAction: warn
      group: ""
      version: v1
      kind: Pod
      namespace: bar
      name: maximal-security-violation
      message: "container <violation-container> has an invalid image repo ..."
```

Each entry in `status.violations` represents a single violating resource. The fields of interest per violation are:

| Field | Description |
|-------|-------------|
| `kind` | Kind of the violating resource |
| `group` | API group of the violating resource |
| `version` | API version of the violating resource |
| `namespace` | Namespace of the violating resource (empty for cluster-scoped) |
| `name` | Name of the violating resource |
| `message` | Human-readable description of the violation |
| `enforcementAction` | The action taken (`warn`, `deny`, `dryrun`) |

The constraint's own `metadata.name` and kind identify which policy is being violated. Downstream reporting to the [metrics handler](metrics.md) is handled separately.
