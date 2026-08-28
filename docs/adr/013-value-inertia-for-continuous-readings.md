# 013 - Bounded-Step Inertia for Continuous Readings, Instantaneous Resampling for Others

## Status
Accepted

## Context
The most direct way to generate a simulated field value each tick is to pick a fresh random number from whatever range the GPU's current status/failure type implies. This is what the original port did, and it produces unrealistic telemetry for temperature specifically: two consecutive 10-second samples could legitimately read, say, 84°C then 61°C, purely by chance, with nothing about the GPU's actual condition having changed. Real temperature has physical thermal mass behind it and cannot jump like that between short sampling intervals.

Power draw does not have the same physical constraint. It's determined by how many transistors are switching right now, which tracks workload almost instantly — DCGM itself reflects this by exposing both a standard and an explicitly-named "instantaneous" power reading (`DCGM_FI_DEV_BOARD_POWER_WATTS` vs. `DCGM_FI_DEV_BOARD_POWER_RAW_WATTS`), with no equivalent "raw vs. smoothed" distinction offered anywhere for temperature. That asymmetry in DCGM's own field set is evidence the two should be modeled differently, not treated uniformly.

A related design question came up: should the simulator instead invert its causality entirely, generating raw values first via an independent random walk and *deriving* health status from thresholding them (closer to how real hardware/DCGM alerting actually works)? This was considered and rejected — no consumer of this data (Prometheus, Grafana, Alertmanager) can observe which direction the internal causality runs; they only ever see the resulting reported values. The fidelity gain from inverting causality is invisible externally, while the cost (reworking the already-built, tested probability-driven state machine to mean something entirely different) is real and large. The bounded-step approach below achieves the actual visible goal — no more unrealistic jumps — without that rework.

## Decision
Temperature fields (`DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_MEMORY_TEMP_CELSIUS`) are advanced by `nextBoundedStep` — a bounded random step (a caller-supplied step size ± a caller-supplied fuzz range) toward whatever range the GPU's current status/failure type implies, rather than resampled fresh each tick — including once the value has already reached its target range, where it continues to wander by a bounded amount rather than resampling across the full range. `nextBoundedStep` itself is generic (step size and fuzz are parameters, not hardcoded constants); `advanceThermal` supplies the thermal-specific tuning (`ThermalStepSize`, `ThermalFuzz`) at the call site. Power (`DCGM_FI_DEV_BOARD_POWER_WATTS`) continues to resample fresh from its target range every tick, with no inertia, matching its real instantaneous behavior.

`GPUState.Status`/`FailureType` remain the probabilistically-driven source of truth for what a value should be trending toward; the value-generation layer (`AdvanceValues`/`advanceThermal`/`advancePowerUsage`) only decides how quickly the *reported number* catches up to that target, never the reverse.

## Consequences
- Adding inertia to a field that shouldn't have it (or vice versa) only requires deciding whether that field's advance function calls `nextBoundedStep` (with its own tuning) or resamples directly — the two behaviors are cleanly separated by field, not entangled in one generic function
- Because `nextBoundedStep` takes its step size and fuzz range as parameters rather than reading fixed constants, a future field needing the same bounded-random-walk behavior with different tuning (e.g. a power model that accounts for supply-side capacitance) can reuse it directly, supplying its own values at the call site, without copying the function
- ECC error counts and the last-XID field are neither target-range values nor need smoothing at all — they're accumulators/events driven directly by `FailureType`/`Advance`'s output, handled by separate logic entirely outside this inertia model
