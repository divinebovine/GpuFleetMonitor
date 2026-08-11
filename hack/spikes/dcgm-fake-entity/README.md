# Spike: DCGM Fake-Entity Injection vs. Real `dcgm-exporter`

## Outcome: **No-go on stock `dcgm-exporter`. Falling back to Approach C (custom exporter).**

`nv-hostengine` and DCGM's fake-entity/field-injection API both work correctly with
no GPU/driver present. But stock `dcgm-exporter` cannot serve metrics for those fake
entities: it establishes a live field *watch* (a polling subscription), and DCGM
refuses to set up a watch on any `GPU`-type entity — real or fake — when NVML isn't
loaded. Manually injected values are visible via `dcgmi test --introspect`, but
nothing that goes through the normal watch/poll path (i.e. `dcgm-exporter`) can see
them.

Since `gpu-injector` will already be computing every value it injects, the fix is to
skip the inject-then-scrape round trip and have `gpu-injector` serve its own
`/metrics` endpoint directly, using the `DCGM_FI_DEV_*` naming convention so it's a
drop-in look-alike for real `dcgm-exporter` output. `nv-hostengine` stays in the pod
(so `dcgmi`/DCGM API tooling still works against it for debugging/parity), it's just
no longer the metrics source of truth.

---

## Step 1: `nv-hostengine` with no GPU present

First attempt used `nvidia/dcgm:4.5.2-1-ubi9` with `nv-hostengine -n` as the run
command, which failed:

```
PARSE ERROR: Argument: nv-hostengine
             Couldn't find match for argument
```

Cause: the image's `ENTRYPOINT` is already `["/usr/bin/nv-hostengine"]` (confirmed
against the [image's Dockerfile](https://github.com/NVIDIA/DCGM/blob/master/docker/Dockerfile.ubuntu22.04)),
so passing `nv-hostengine -n` as the command appends the literal word `nv-hostengine`
as an extra argument to itself.

Switched to `nvidia/dcgm:4.5.2-1-ubuntu22.04` and passed only flags, bumping the log
level since the image's default `CMD` uses `--log-level NONE`:

```bash
docker run -d --name dcgm-spike nvidia/dcgm:4.5.2-1-ubuntu22.04 \
  -n -b 0.0.0.0 --log-level INFO -f -
docker logs dcgm-spike
```

Result: started cleanly —

```
Initializing NVML with NVML_INIT_FLAG_NO_GPUS and NVML_INIT_FLAG_NO_ATTACH flags.
Failed to initialize NVML: 12
Cannot load NVML; DCGM will proceed without managing GPUs. ret: 12
...
Started host engine version 4.5.2 using port number: 5555
```

The NvSwitch/NSCQ errors alongside this are unrelated (it's also probing for
NVSwitch hardware) and not a blocker.

## Step 2: `dcgmi` cannot create fake entities

`dcgmi discovery -l` confirmed 0 real GPUs, as expected. But `dcgmi`'s full
subsystem list (`dcgmi --help`) has no fake-entity subsystem, and `dcgmi test`'s
only relevant flags are `--inject`/`--introspect` against an *existing* `gpuid`:

```
$ dcgmi test --inject --gpuid 0 -f 150 -v 65
Error: Unable to inject info. Return: Bad parameter passed to function
```

Checked the image's Python bindings
(`/usr/share/datacenter-gpu-manager-4/bindings/python3/`) for a fake-entity
API too — no `dcgm_agent_internal.py` present, and `dcgm_agent.py` has no
`Fake`/`Inject`/`Create`-entity functions at all (DCGM historically splits this
into an internal/test-only binding set that isn't shipped in this package).

**Conclusion:** fake-entity creation isn't reachable from `dcgmi` or the shipped
Python bindings in this image — it's only reachable through the DCGM C API
(`dcgmCreateFakeEntities`/`dcgmInjectFieldValue`, declared `DCGM_PUBLIC_API` in
`libdcgm.so.4`), which `go-dcgm` calls directly via cgo. Pivoted to a throwaway Go
program instead of fighting the CLI.

## Step 3: Go spike confirms fake-entity creation + injection works

`hack/spikes/dcgm-fake-entity/main.go` (kept in this directory as the spike
artifact) does the following against `go-dcgm`:

```go
cleanup, err := dcgm.Init(dcgm.Standalone, "127.0.0.1:5555", "0")
defer cleanup()

ids, err := dcgm.CreateFakeEntities([]dcgm.MigHierarchyInfo{
    {Entity: dcgm.GroupEntityPair{EntityGroupId: dcgm.FE_GPU}},
})

err = dcgm.InjectFieldValue(ids[0], dcgm.DCGM_FI_DEV_GPU_TEMP_CELSIUS, dcgm.DCGM_FT_INT64, 0, 0, int64(65))
```

Built and run inside the still-running `dcgm-spike` container (needs
`libdcgm.so.4` present at runtime via `dlopen`, which the host doesn't have):

```bash
CGO_ENABLED=1 go build -o ~/dcgm-spike-test ./hack/spikes/dcgm-fake-entity
docker cp ~/dcgm-spike-test dcgm-spike:/dcgm-spike-test
docker exec -it dcgm-spike /dcgm-spike-test
```

Output:
```
dcgm initialized successfully
created fake enttities: [2]
injected temp=65 on entity 2
```

Confirmed the injected value is really there via direct introspection (not
`discovery`, see below):

```
$ dcgmi test --introspect --gpuid 2 -f 150
 Field ID                  : gpu_temp
 Number of Samples         : 1
 Newest Timestamp          : Sat Aug  1 04:20:08 2026
 Number of Watchers        : 0
```

Note `dcgmi discovery -l -a` still reports 0 GPUs even after this — fake entities
created via `CreateFakeEntities` are logical entries in DCGM's entity/field-value
system, not something `discovery`'s NVML-based device enumeration surfaces. This is
expected, not a bug, and foreshadowed the real problem in Step 4.

Also note "Number of Watchers: 0" above — nobody had subscribed a watch to this
field yet at the point we introspected it. That distinction (direct cache write vs.
a polling subscription) turned out to be exactly what breaks `dcgm-exporter`.

## Step 4: `dcgm-exporter` fails to watch fields on a driver-less hostengine

Image: `nvcr.io/nvidia/k8s/dcgm-exporter:4.8.3`. This image is effectively
distroless (has `bash` but no `ls`/`cat`/coreutils), and its `ENTRYPOINT` is a
wrapper that ignores whatever `CMD` you pass and always runs the same startup
sequence — use `--entrypoint dcgm-exporter` to talk to the real binary directly.

First attempt failed immediately with no useful detail:

```bash
docker run --rm -it --network container:dcgm-spike \
  -e DCGM_REMOTE_HOSTENGINE_INFO=127.0.0.1:5555 \
  nvcr.io/nvidia/k8s/dcgm-exporter:4.8.3
```
```
time=... level=INFO msg="Starting dcgm-exporter" Version=4.6.0-4.8.3
exit status 1
```

Traced this to
[`internal/pkg/prerequisites/dcgmlib_rule.go`](https://github.com/NVIDIA/dcgm-exporter/blob/main/internal/pkg/prerequisites/dcgmlib_rule.go)
in the `dcgm-exporter` source: a startup prerequisite check shells out to
`ldconfig -p` to confirm `libdcgm.so.4` is registered, and surfaces that
subprocess's raw error (`exit status 1`) with no further context if it fails.
Bypassed with a flag built for exactly this:

```bash
docker run --rm -it --network container:dcgm-spike \
  --entrypoint dcgm-exporter nvcr.io/nvidia/k8s/dcgm-exporter:4.8.3 \
  --remote-hostengine-info 127.0.0.1:5555 \
  --fake-gpus \
  --disable-startup-validate \
  --debug
```

This got further — it connected, found the fake GPU entity, and tried to watch the
default counters file's fields — but every field group watch failed the same way:

```
level=ERROR msg="Failed to watch DCGM fields" ... error="error watching fields: Cannot perform the requested operation because NVML doesn't exist on this system."
level=ERROR msg="DCGM collector for entity type 'GPU' cannot be initialized; ..."
level=INFO msg="Registry built successfully" collector_count=0
```

To rule out "maybe it's just the default file's NVML-only fields" rather than a
blanket rule, retried with a custom one-field counters file containing *only*
`DCGM_FI_DEV_GPU_TEMP` (the exact field successfully injected in Step 3):

```bash
docker run -d --name dcgm-exporter-test --network container:dcgm-spike \
  -v ~/minimal-counters.csv:/etc/dcgm-exporter/custom-counters.csv:ro \
  --entrypoint dcgm-exporter nvcr.io/nvidia/k8s/dcgm-exporter:4.8.3 \
  --remote-hostengine-info 127.0.0.1:5555 --fake-gpus --disable-startup-validate \
  --collectors /etc/dcgm-exporter/custom-counters.csv --debug
```

Identical failure, field-count of one:

```
level=ERROR msg="Failed to watch DCGM fields" ... field_ids=[150] field_names=[gpu_temp] ... error="error watching fields: Cannot perform the requested operation because NVML doesn't exist on this system."
level=INFO msg="Registry built successfully" collector_count=0
```

`curl http://localhost:9400/metrics | grep DCGM_FI_DEV_GPU_TEMP` returned nothing in
both cases, as expected given `collector_count=0`.

**This confirms it's not a fixable field-selection issue** — DCGM's `Watch`
mechanism has a hard, unconditional NVML dependency for `GPU`-type entities,
independent of the fake/real distinction. `InjectFieldValue`'s direct cache write
is a fundamentally different (and, for a *consumer*, dead-end) path from the
watch/poll mechanism every real metrics client uses.

---

## Gotchas worth remembering

- **Snap-packaged Docker (`/snap/bin/docker`) cannot see `/tmp`.** It's strictly
  confined to `$HOME` and a few whitelisted paths. `docker cp`/bind-mounts from
  `/tmp` fail with a misleading `lstat: no such file or directory` even when the
  file demonstrably exists. Build/stage spike artifacts under `$HOME` instead.
- **`go build -o /tmp/x ./cmd/pkg` requires the *directory*, not a `.go` file
  path**, to build a package that may grow more files later.
- **A bare `go build ./some/path` with no `-o`** drops the binary in your *current*
  directory named after the package — easy to accidentally leave stray binaries at
  the repo root.
- **`go get` run from inside a subdirectory** will `go mod init` a nested module
  there if one doesn't already exist, silently splitting it out of the main
  module's dependency graph (`go build` from root then fails with "main module
  does not contain package...").

## Cleanup

```bash
docker rm -f dcgm-spike dcgm-exporter-test 2>/dev/null
```
