# CLI Flag Combination Requirements

## Introduction

This document defines which flag combinations are valid, invalid, or redundant. It does not repeat what individual flags do — see `requirements-core.md` for that. It only defines the rules governing how flags interact.

## Notation

| Symbol | Flag |
|---|---|
| `P` | `<project-path>` (positional argument) |
| `A` | `--agents` |
| `T` | `--port` |
| `K` | `--ssh-key` |
| `R` | `--rebuild` |
| `N` | `--no-update-known-hosts` |
| `C` | `--no-update-ssh-config` |
| `V` | `--verbose` |
| `D` | `--docker-restart-policy` |
| `H` | `--host-network-off` |
| `S` | `--stop-and-remove` |
| `U` | `--purge` |

`∅` = flag absent (using its default)  
`✓` = flag present  
`⊕` = exactly one of  
`∧` = and  
`¬` = not

---

## Modes

Every valid invocation belongs to exactly one of three **modes**. The mode is determined by which action flag is present.

| Mode | Condition | Description |
|---|---|---|
| **START** | `¬S ∧ ¬U` | Start (or reconnect to) a container for a project |
| **STOP** | `S ∧ ¬U` | Stop and remove a container |
| **PURGE** | `U ∧ ¬S` | Remove all tool data, containers, and images |

`S ∧ U` is invalid — see Req CLI-1.

---

## Requirements

### Requirement CLI-1: Mutually exclusive action flags

`S` and `U` are mutually exclusive. They represent distinct destructive operations and cannot be combined.

**Formal:** `¬(S ∧ U)`

IF `S ∧ U` THEN THE CLI SHALL print a descriptive error to stderr and exit with a non-zero exit code.

---

### Requirement CLI-2: project-path is required in START and STOP modes

`P` is required whenever the operation is project-scoped (START and STOP). It is not used in PURGE mode.

**Formal:**
- `(¬S ∧ ¬U) → P` — START mode requires `P`
- `(S ∧ ¬U) → P` — STOP mode requires `P`
- `(U ∧ ¬S) → ¬P` — PURGE mode does not accept `P`

IF PURGE mode AND `P` is provided THEN THE CLI SHALL print a descriptive error to stderr and exit with a non-zero exit code.

IF START or STOP mode AND `P` is absent THEN THE CLI SHALL print a usage message to stderr and exit with a non-zero exit code.

---

### Requirement CLI-3: START-only flags are invalid in STOP and PURGE modes

`A`, `T`, `K`, `R`, `N`, `C`, `V`, `D`, and `H` are only meaningful in START mode. They have no effect on STOP or PURGE operations and must not be silently ignored.

**Formal:** `(S ∨ U) → ¬(A ∨ T ∨ K ∨ R ∨ N ∨ C ∨ V ∨ D ∨ H)`

IF STOP or PURGE mode AND any of `A`, `T`, `K`, `R`, `N`, `C`, `V`, `D`, `H` is provided THEN THE CLI SHALL print a descriptive error to stderr identifying the incompatible flag(s) and exit with a non-zero exit code.

---

### Requirement CLI-4: --rebuild is only meaningful with --agents or an existing image

`R` without `A` (and with no existing image) is valid but redundant — it forces a rebuild of the default agent set. This is permitted. However, `R` in STOP or PURGE mode is covered by CLI-3.

No additional constraint beyond CLI-3.

---

### Requirement CLI-5: --port range

`T` must be a valid unprivileged TCP port.

**Formal:** `T → (1024 ≤ T ≤ 65535)`

IF `T` is provided AND `T < 1024` OR `T > 65535` THEN THE CLI SHALL print a descriptive error to stderr and exit with a non-zero exit code.

Note: ports 1–1023 are privileged and require root, which is forbidden by Req 11.

---

### Requirement CLI-6: --agents must contain at least one valid ID

`A` must resolve to a non-empty list of known agent IDs after parsing.

**Formal:** `A → (|parsed(A)| ≥ 1) ∧ (∀ id ∈ parsed(A): id ∈ AgentRegistry)`

IF `A` is provided AND the parsed list is empty THEN THE CLI SHALL print a descriptive error to stderr and exit with a non-zero exit code.

IF `A` is provided AND any ID is not in the AgentRegistry THEN THE CLI SHALL print a descriptive error listing the unknown ID(s) and the available IDs, and exit with a non-zero exit code.

---

### Requirement CLI-7: --docker-restart-policy must be a valid Docker restart policy

`D` must be one of the recognised Docker restart policy names.

**Formal:** `D → D ∈ {"no", "always", "unless-stopped", "on-failure"}`

IF `D` is provided AND its value is not one of `no`, `always`, `unless-stopped`, `on-failure` THEN THE CLI SHALL print a descriptive error to stderr listing the valid values and exit with a non-zero exit code.

---

## Valid Combination Summary

The table below lists all meaningful flag combinations. `✓` = present, `∅` = absent/default, `✗` = forbidden.

| Mode | P | A | T | K | R | N | C | V | D | H | S | U | Valid? |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ minimal start |
| START | ✓ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ custom agents |
| START | ✓ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ custom port |
| START | ✓ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ custom SSH key |
| START | ✓ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ force rebuild |
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ skip known_hosts update |
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ skip SSH config update |
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ verbose build output |
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ✓ custom restart policy |
| START | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ✓ disable host network |
| START | ✓ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ force rebuild + verbose |
| START | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ∅ | ∅ | ✓ all start flags |
| STOP  | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✓ |
| PURGE | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ✓ |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✗ no mode, no path |
| —     | ✓ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: A with S |
| —     | ✓ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: T with S |
| —     | ✓ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: R with S |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: N with S |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: C with S |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-3: V with S |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✓ | ∅ | ✗ CLI-3: D with S |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ✓ | ∅ | ✗ CLI-3: H with S |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ✗ CLI-3: N with U |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ∅ | ✓ | ✗ CLI-3: C with U |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ∅ | ✓ | ✗ CLI-3: V with U |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ∅ | ✓ | ✗ CLI-3: D with U |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✓ | ✗ CLI-3: H with U |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ✓ | ✗ CLI-1: S ∧ U |
| —     | ✓ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ✗ CLI-2: P with U |
| —     | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ∅ | ✓ | ∅ | ✗ CLI-2: S without P |
