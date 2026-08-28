# 010 - `FailureType` Is an Internal-Only Classification, Not a DCGM-Mirrored Value

## Status
Accepted

## Context
`GPUState.FailureType` (`Thermal`/`Power`/`ECCSingle`/`ECCDouble`/etc.) drives both the injector's transition-risk logic and which DCGM fields get pushed into an abnormal range. Partway through implementation it seemed natural to give it a `uint` representation matching some DCGM-native numeric identity, on the theory that would make mapping it onto DCGM field IDs, XID codes, or health-check categories more direct.

Checking `go-dcgm`'s actual API surface ruled this out. Three distinct DCGM numeric namespaces exist, and none has the right shape:
- **Field IDs** (`DCGM_FI_DEV_GPU_TEMP`=150, etc.) are too fine-grained — one `FailureType` legitimately drives several fields at once (a thermal failure affects temperature *and* utilization).
- **XID codes** (48, 79, ...) are too sparse — only the two hardware-failure types (`ECCDouble`, `GpuFellOffTheBus`) have a real XID; `Thermal`/`Power`/`ECCSingle` don't correspond to a discrete fault event at all.
- **`HealthSystem` bitmask flags** (`DCGM_HEALTH_WATCH_THERMAL`, `DCGM_HEALTH_WATCH_MEM`, etc.) are too coarse — `DCGM_HEALTH_WATCH_MEM` covers both `ECCSingle` and `ECCDouble` under one value, which would erase exactly the distinction the state machine depends on (one self-heals via Warning↔Critical fluctuation and steps back freely; the other never steps back and is unconditionally unrecoverable).

More fundamentally: DCGM's data model has no "root cause" concept anywhere. It reports raw field values, separately logs discrete XID events, and separately exposes coarse per-subsystem health-check results — nothing in DCGM ever represents "why" a reading is bad, only "what" it currently is. `FailureType` answers a question DCGM itself doesn't model, so there was never a DCGM-native value for it to become.

## Decision
`FailureType` stays a plain Go string-backed enum, entirely internal to the simulator. Its only two responsibilities are (1) driving `Advance`'s transition-risk logic, and (2) — via an explicit, hand-written `FailureType → affected fields` mapping in the tick loop — deciding which DCGM fields to sample from an abnormal range each tick. Neither responsibility needs or benefits from numeric parity with any DCGM value.

## Consequences
- No conversion/lookup table is needed between `FailureType` and a DCGM identity, because none should exist — the mapping in `tick.go` from failure type to affected fields is itself the intentional, only place that translation happens
- Error/log output stays human-readable (`%s` on a `FailureType` prints `"thermal"`, not a bare number), which the test suite already leans on throughout
- `FailureType` should not be renamed or reshaped to "look like" a DCGM concept later — if a future field needs a real XID or field-ID mapping, that mapping is a small, explicit, separate function (as already done for the two hardware-failure types' XID codes), not a property of the type itself
