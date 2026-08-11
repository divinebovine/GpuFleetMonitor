me 
# Spike: `node-problem-detector` Xid Detection Against a Custom Log File

## Outcome: **Go.** `custom-plugin-monitor` (not `system-log-monitor`) is the right mechanism.

There's no real "standard NVIDIA Xid rule" for NPD's `system-log-monitor`/log-monitor
plugin (the mechanism originally assumed in the design doc — matching a regex
`pattern` directly against streamed log lines). What real GPU fleets actually use
(confirmed against a genuine production config from
[Azure's AKS GPU node-health setup](https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml))
is NPD's **`custom-plugin-monitor`** mechanism: a periodic script that greps a log
file for an XID-code allowlist, with time-window/dedup logic layered on top. This
spike adapts that real config rather than inventing a new one, run against a plain
file instead of `/var/log/syslog`.

The hardware-failure XID allowlist (`48 56 57 58 62 63 64 65 68 69 73 74 79 80 81
92 119 120`) is reused verbatim from the Azure source — it's a real, if informal,
reference for which XID codes should count as a genuine hardware fault in
`gpu-injector`'s later fault-injection logic (sub-project #4), as opposed to benign
transient codes.

---

## Step 1: Get a real Xid rule config to adapt

Checked the upstream `node-problem-detector` repo's own `config/` directory first —
no Xid-specific rule exists there at all (`gh search code "Xid" --repo
kubernetes/node-problem-detector` returns nothing). Searched further and found a
real, production Azure AKS GPU-health config using NPD's `custom-plugin-monitor`
mechanism instead of the log-monitor/regex mechanism originally assumed. Confirmed
its JSON schema matches NPD's own canonical
[`config/custom-plugin-monitor.json`](https://github.com/kubernetes/node-problem-detector/blob/master/config/custom-plugin-monitor.json)
example field-for-field (`invoke_interval`, `timeout`, `max_output_length`,
`concurrency`, `conditions`, temporary/permanent `rules` pairs) — it's a legitimate
use of the real mechanism, not an Azure-specific invention.

Adapted into `config/custom-plugin-gpu-xid.json`, shrinking `invoke_interval`/rule
`timeout` from the original `60s`/`20s` to `10s`/`5s` for faster spike iteration.

## Step 2: Point the script at a plain file

The Azure reference script (`check_gpu_xid.sh`) hardcodes `kernel_log="var/log/syslog"`
(a relative path — presumably resolves against wherever NPD's plugin scripts run
from in their setup). Copied the script and changed the constant to
`/config/fake-kmsg.log`, the plain file this spike controls.

## Step 3: Run NPD locally against it

Image: `registry.k8s.io/node-problem-detector/node-problem-detector:v1.36.0`.

First attempt failed immediately:
```
F0801 06:19:21.785239 1 log_monitor.go:68] Failed to read configuration file "/config/kernel-monitor.json": open /config/kernel-monitor.json: no such file or directory
```

Same gotcha as the DCGM spike's `dcgm-exporter` image: this image's own
`Dockerfile` bakes flags into `ENTRYPOINT`:
```
ENTRYPOINT ["/node-problem-detector", "--config.system-log-monitor=/config/kernel-monitor.json,/config/readonly-monitor.json"]
```
`docker run`'s trailing args get *appended* to that, not substituted for it — so
NPD was still trying (and failing) to load the baked-in default log-monitor
configs we never provided, on top of the custom-plugin config we did. Fixed by
overriding the entrypoint entirely:

```bash
docker run -d --name npd-spike \
  -v "$(pwd)/hack/spikes/npd-custom-log/config:/config" \
  --entrypoint /node-problem-detector \
  registry.k8s.io/node-problem-detector/node-problem-detector:v1.36.0 \
  --config.custom-plugin-monitor=/config/custom-plugin-gpu-xid.json \
  --enable-k8s-exporter=false \
  --logtostderr
```

`--enable-k8s-exporter=false` matters here — it lets NPD run and evaluate rules
fully standalone, with no real k8s API server needed. (Actually patching a real
Node's `.status.conditions` in-cluster is deferred to Task 15, same division of
labor as originally planned — this spike only needs to confirm the detection
mechanics.)

This started cleanly and set the initial condition:
```
Initialized conditions for /config/custom-plugin-gpu-xid.json: [{Type:GpuXid Status:False ... Reason:GpuXidGood Message:GPU XID is ok}]
```

## Step 4: Append a matching line and confirm NPD reacts — three real bugs found along the way

**Bug 1 — allowlist loop exits on the first miss.** The script's
`for XID in $XID_EC; do ... done` loop's `else` branch (hit when a given code
*isn't* found) originally did `exit $OK` immediately. Since the allowlist is
checked in order (`48 56 57 ...`) and our test line used XID 79, the loop exited
"clean" on its very first iteration (checking for 48) and never reached 79 at all,
regardless of what else was in the file. Confirmed by running the script directly:
```
No GPU Xid 48 error found in kernel log
EXIT CODE: 0
```
This is a real bug in the original Azure script itself (inherited, not introduced
by adapting it) — as originally written it can only ever detect whichever
allowlisted code happens to be checked first. Fixed by making that branch a no-op
(just don't exit) instead of exiting, so the loop actually walks the whole list.

**Bug 2 — missing log file treated as a failure.** The script's earlier
`if [[ ! -f $kernel_log ]]; then ... exit $NONOK; fi` guard exits *NONOK* when the
file doesn't exist yet. In our real DaemonSet, `gpu-injector` won't create the
shared log file until the first XID event actually happens — so this would report
every node as GPU-faulted from pod startup until the first real event, a
false-positive baked into the design. Fixed to `exit $OK` instead.

**Bug 3 (the significant one) — dedup logic applied to the permanent rule causes
the Node condition to self-heal after one poll, independent of Bugs 1/2.** The
JSON wires the *same* script to both the `temporary` rule (fire an Event once per
new XID — this is what the dedup/logfile logic is actually for) and the
`permanent` rule (set the sticky Node condition). Confirmed via NPD's own logs:
the condition flipped `False → True` on the first poll after the XID line
appeared, then flipped straight back to `False` on the *next* poll 10s later —
even though the fault line was still sitting in the file, still within the 2-hour
window. Root cause: on the second poll, the dedup file already had the XID
recorded as "seen," so the *same* script (now serving the `permanent` rule too)
returned `OK` instead of `NONOK`, and NPD dutifully un-set the condition it had
just set.

For our use case (sub-project #2 will eventually cordon/drain nodes off this
condition), a condition that silently clears itself while the underlying problem
is still present is a correctness bug, not a cosmetic one. Fixed by splitting into
two scripts:
- `config/plugin/temp_rule_check_gpu_xid.sh` — unchanged dedup'd script, wired
  only to the `temporary` rule.
- `config/plugin/perm_rule_check_gpu_xid.sh` — same allowlist/time-threshold walk,
  **no dedup at all**: answers "is a matching XID present within the last
  `time_threshold` hours, right now?" on every single poll, `exit $NONOK` if yes,
  `$OK` if no. (This file also had its own two bugs during authoring — a dropped
  `logXid=...` assignment that broke the date-math entirely, and an inverted
  `-ge`/`-le` comparison with no `exit $NONOK` anywhere in the script, meaning it
  could never report a fault at all. Both fixed; verified by running the script
  directly against a live fault line and confirming `EXIT CODE: 1`.)

Confirmed end-to-end after the fix — three poll cycles (~35s) with the XID line
present the whole time, exactly one `False → True` transition logged and no
flip back:
```
Node condition GpuXid is now: True, reason: GpuXidBad, message: "GPU Xid errors detected: ..."
```
(and no further "Node condition GpuXid is now" lines afterward, confirming it held).

## Gotchas worth remembering

- **This image also bakes flags into `ENTRYPOINT`.** Same pattern as
  `nv-hostengine`/`dcgm-exporter` from the DCGM spike — always check an image's own
  `Dockerfile` before assuming `docker run <image> <flags>` replaces its defaults
  rather than appending to them.
- **Timestamps must be in the container's timezone (UTC), not the host's local
  time**, when hand-constructing log lines for scripts that do their own date
  parsing/diffing — a line stamped in local time can silently compute as "hours
  old" and get skipped by a time-threshold check, with no error, just silence.
- **A script shared between a `temporary` and a `permanent` rule runs once per
  rule, per poll** — any state it mutates (like a dedup logfile) is shared across
  both invocations, which can produce surprising cross-talk if the two rules need
  different semantics (fire-once vs. reflect-current-state).

## Cleanup

```bash
docker rm -f npd-spike 2>/dev/null
```
