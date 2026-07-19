# 003 — SSE Over Polling for GPU Fleet Data

## Status
Accepted

## Context
The `/v1/gpus` endpoint processes 10,000 GPUs through a worker pool and originally
returned a single JSON response after all results were collected. This caused two
problems:

1. The client received a 6MB JSON payload at once, blocking the main thread during
   parsing and preventing the loading skeleton from ever being painted.
2. The user experienced a long blank period with no feedback before the table appeared.

Three approaches were considered:

1. **Polling** — repeated requests on an interval
2. **HTTP streaming / chunked transfer** — stream raw JSON chunks over a single
   connection without an event protocol
3. **Server-Sent Events (SSE)** — stream discrete named events over a persistent
   HTTP connection

Polling was rejected — it would repeat the same 6MB transfer and main-thread block
on every tick without improving the initial load experience.

HTTP streaming was rejected because parsing an incrementally received JSON array
on the client is non-trivial — the `fetch` streaming API (`ReadableStream`) requires
a custom parser to handle partial JSON chunks, adding complexity with little benefit
over SSE.

SSE was chosen because each GPU result is a discrete, self-contained JSON object.
The `data:` event format maps naturally to individual results, the `EventSource` API
handles reconnection and framing automatically, and named events (`done`) provide a
clean termination signal without custom protocol design.

## Decision
Changed the `/v1/gpus` handler to stream results using Server-Sent Events (SSE).
As each goroutine in the worker pool resolves a GPU's health, the result is
immediately written to the response stream as a `data:` event and flushed. A final
`event: done` signals the client that the stream is complete.

On the client, `EventSource` replaces `fetch`. Incoming GPU events are buffered
locally and flushed to React state every 200ms so the table fills progressively
without causing 10,000 individual re-renders. The final sort runs once on the `done`
event.

## Consequences
- Loading skeleton is visible immediately on page load
- The table fills progressively as the worker pool resolves GPUs, giving real-time
  feedback instead of a single delayed paint
- Main thread is never blocked by a large JSON parse — each event is a small payload
- The Go handler now uses `http.Flusher` and `r.Context().Done()` for client
  disconnect detection
- `chi`'s 60-second timeout middleware is sufficient for the current fleet size but
  would need revisiting for significantly larger fleets or slower simulators
- SSE is unidirectional and HTTP/1.1 compatible; no WebSocket upgrade needed for
  this use case
- Browser SSE connections count against the per-domain connection limit (6 for
  HTTP/1.1), which is acceptable for a single long-lived stream
