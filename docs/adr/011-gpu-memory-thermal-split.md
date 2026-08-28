# 011 - Separate GPU-Core and Memory Thermal Failure Types and Ranges

## Status
Accepted

## Context
The original port carried forward the old simulator's single `FailureTypeThermal`, which drove both `DCGM_FI_DEV_GPU_TEMP` and `DCGM_FI_DEV_MEMORY_TEMP_CELSIUS` from one shared temperature range — memory temperature was derived as a fixed offset below GPU-core temperature rather than modeled independently.

This doesn't match real hardware or real DCGM output. `dcgm-exporter`'s own default field set treats GPU-core and memory temperature as two independently-observable fields precisely because they're separate physical components (the die vs. the HBM/GDDR stacks) with independently-specified thermal limits — a memory stack can run hot while the core is fine, or vice versa. General HBM2e/HBM3 guidance also puts memory's safe ceiling meaningfully below the GPU core's (roughly 85°C vs. the die's low-90s throttle point in commonly-cited figures), so sharing one range/threshold set for both was never going to be representative of either one accurately.

## Decision
Split the single `FailureTypeThermal` into `FailureTypeGPUThermal` and `FailureTypeMemoryThermal`, each with its own independent range map in `gpu.GPUSpec` (`GpuTemperatureRanges` and `MemoryTemperatureRanges`). `internal/injector/simvalue.go`'s `advanceThermal` selects both the failure type to check against and the range map to sample from based on which DCGM field is being advanced (140 vs. 150), so the two temperatures vary independently rather than one being derived from the other.

Both new failure types remain in the same probabilistic category (`gpu.TransientFailures`) as the original `Thermal` did — they're both self-healing conditions with identical transition-risk behavior; only which physical component (and which range/field) is implicated differs.

## Consequences
- A GPU can now be simulated as thermally failing at the memory level without its core temperature also reading abnormal, matching what a real dashboard built against genuine `dcgm-exporter` output would expect to be independently possible
- `handleHealthy`'s uniform random pick across `TransientFailures` now has three members instead of two, which shifts the per-tick probability of *any specific* transient failure type from 50% to ~33% each — an intentional consequence of adding a category member, not a bug, but worth remembering if failure-type distribution ever needs re-tuning
- Memory temperature range values (35/70/80/90 °C boundaries) are a reasonable approximation for demo purposes, not verified against a manufacturer datasheet — precise per-model, per-generation HBM thermal specs weren't readily available from public sources at the time this was written; revisit if higher fidelity is ever needed
