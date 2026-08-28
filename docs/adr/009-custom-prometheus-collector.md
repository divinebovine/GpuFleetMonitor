# 009 - Custom `prometheus.Collector` Instead of `GaugeVec`/`CounterVec`

## Status
Accepted

## Context
`gpu-injector`'s `/metrics` endpoint needs to expose `DCGM_FI_DEV_*` fields per entity, refreshed every tick. The first implementation used `client_golang`'s standard `prometheus.GaugeVec`/`CounterVec` types directly, calling `.Set(value)` for gauges and `.Add(delta)` for counters.

This broke down for the counter fields (`DCGM_FI_DEV_ECC_SBE_VOL_TOTAL`, `DCGM_FI_DEV_ECC_DBE_VOL_TOTAL`). DCGM reports these as an absolute cumulative total each tick, not an incremental delta — but `prometheus.Counter` only exposes `Inc()`/`Add(positiveDelta)`, with no `Set()`. Reproducing "the current absolute value DCGM reports" as a `Counter` therefore required the caller to track the last-reported value per entity per field and compute `Add(value - last)` — and `Counter.Add()` panics on a negative argument, which a real simulated hardware replacement (resetting a GPU's error counts) would trigger. The fix for that (`DeleteLabelValues` + recreate on a decrease) worked but added real, easy-to-get-wrong bookkeeping: a missed write-back to the last-value map caused values to double-count on every subsequent tick during development.

## Decision
`MetricsRecorderImpl` implements `prometheus.Collector` directly (`Describe`/`Collect`) instead of wrapping `GaugeVec`/`CounterVec`. `Record(entityID, fieldID, value)` just stores whatever value it's given — `values map[FieldID]map[uint]float64` — with no accumulation logic at all. `Collect` reports whatever's currently stored, tagged with the field's declared `prometheus.ValueType` (`GaugeValue` or `CounterValue`) via `prometheus.MustNewConstMetric`.

This means the caller (`tick.go`) is always responsible for computing the true current absolute value of a field, the same way for every field type — no delta tracking, no reset detection, no negative-delta panics to guard against. A simulated hardware replacement that drops an ECC count is just a lower number reported on the next scrape, which is exactly the signal Prometheus's own `rate()`/`increase()` reset-detection is built to interpret correctly, for free.

## Consequences
- `Record`'s implementation is trivial and can't get the delta/reset bookkeeping wrong, because there isn't any
- Field metadata (`Name`, `Help`, `prometheus.ValueType`) lives in a separate declarative list (`config.go`'s `simMetrics`), which both `initDescs` and `initValueTypes` are built from — adding a field is one new entry, not new logic in `Record`/`Collect`
- `MetricsRecorderImpl` must be registered with a `prometheus.Registry` and served via `promhttp.HandlerFor(registry, ...)` rather than exposing `ServeHTTP` itself — it's a `Collector`, not an `http.Handler`
- Any field this project ever adds must be reportable as "the current absolute value," matching how DCGM itself works — this approach would not fit a metric that's genuinely only expressible as a delta-since-last-scrape
