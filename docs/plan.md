# Implementation Plan — OTLP Log Processor (Go)

## Objectives

- Receive OTLP Logs over gRPC (`LogsService/Export`).
- For a configurable attribute key and window duration, count unique log records per distinct attribute value within each window and emit the delta counts to stdout.
- Attribute may be present at Log, Scope, or Resource level; if missing, count as `"unknown"`.
- Sustain high throughput (many messages and many records per message), with backpressure and observability.
- Product-ready: clear configuration, structured logs, metrics, graceful shutdown, and comprehensive tests.
- Testability: all state is instance-level; dependencies are injected via interfaces.

## High-Level Design

- Components:
  - gRPC server exposing OTLP Logs `Export`.
  - `Service` (instance-scoped) that owns: config, logger, tracer/meter, metrics instruments, aggregator, sink, and lifecycle control.
  - Attribute extraction helper that searches in order: Log > Scope > Resource.
  - Ingestion queue (buffered channel) for log events to decouple gRPC from aggregation.
  - Aggregator goroutine that increments counts for the current window and flushes on a ticker.
  - Sink interface with a stdout implementation that emits structured JSON snapshots per window.
- Data flow:
  1) `Export` iterates received `ResourceLogs -> ScopeLogs -> LogRecords`.
  2) Extract attribute value (or `"unknown"`).
  3) Enqueue lightweight `Event` into aggregator queue (non-blocking; drop with accounting if full).
  4) Aggregator increments in-memory counts; on each tick (window), it publishes a snapshot and resets counts.
  5) Metrics: processed, dropped, queue depth, flushes, durations. Structured logs for lifecycle and warnings.

## Configuration (instance-level)

- Flags/env (parsed once, stored in `Config`):
  - `-listenAddr` (string, default `localhost:4317`).
  - `-maxReceiveMessageSize` (int, default `16MiB`).
  - `-attributeKey` (string, required). Key to count per value.
  - `-window` (duration, default `10s`).
  - `-maxQueue` (int, default `100_000`).
  - `-outputFormat` (string, default `json`). Present but currently only JSON is implemented; non-`json` values are ignored.
  - `-outputFile` (string, default empty). If set, snapshots are written to this JSONL file; otherwise to stdout.
  - `-logLevel` (string, default `info`). Present but not currently wired to change the logger level.
  - `-gracefulTimeout` (duration, default `10s`).
  - Optional TLS hardening (not implemented yet): `-tls`, `-certFile`, `-keyFile`.
- Validation: ensure `attributeKey != ""`, `window > 0`, `maxQueue >= 0`; fail fast with clear errors.

## Public Types and Interfaces

```go
type Config struct {
    ListenAddr            string
    MaxReceiveMessageSize int
    AttributeKey          string
    Window                time.Duration
    MaxQueue              int
    OutputFormat          string // json only, currently ignored
    OutputFile            string // optional JSONL file path
    LogLevel              string // currently ignored; bridged via otelslog
    GracefulTimeout       time.Duration
}

type Event struct { Value string }

// Matches internal/sink.Snapshot (Unix millis for window times).
type Snapshot struct {
    WindowStart  int64
    WindowEnd    int64
    AttributeKey string
    Counts       map[string]uint64
    Total        uint64
    Dropped      uint64
}

type Sink interface { Publish(ctx context.Context, s Snapshot) error }

// Service (orchestrator) holds instance-scoped deps and instruments.
// See internal/orchestrator for the concrete type.

// Aggregator supports single events and batched enqueues.
type Aggregator struct {
    in      chan Event
    inBatch chan []string
    // ... windowing fields, counters, nowFn, done, etc.
}
```

## Attribute Extraction

- Precedence: LogRecord attributes > Scope attributes > Resource attributes.
- Supported value types: string, bool, integers, doubles; convert to canonical string representation. For others (arrays/maps), fallback to JSON-encoding or type-tagged string; keep it deterministic.
- If attribute missing: return `"unknown"`.
- Provide a pure function: `ExtractAttribute(resourceAttrs, scopeAttrs, logAttrs, key) (string, bool)` to keep it unit-testable.

## Export Handler Behavior

- For each `ExportLogsServiceRequest`:
  - Iterate all records and extract the attribute.
  - Increment `logsReceived` for each record seen.
  - Enqueue `Event{Value: val}` into aggregator:
    - Non-blocking send. If queue is full, increment `logsDropped` and an `rejected` counter for this request.
  - Return `ExportLogsServiceResponse` with `PartialSuccess.RejectedLogRecords = rejected` when any drops occur.
- Add a span around processing and attribute extraction for visibility at high volume (sampling can be tuned).

## Aggregation & Windowing

- Single goroutine owns the mutable aggregation state (no locks on the hot path):
  - `counts` map from value -> count, `total`, and `dropped` counters per window.
  - `ticker := time.NewTicker(cfg.Window)`; on tick, build `Snapshot`, call `sink.Publish`, reset `counts/total/dropped`.
  - On shutdown, flush a final partial window.
- Backpressure & drops:
  - `in` is a bounded buffered channel; when full, drops occur and are accounted for (metrics + response `PartialSuccess`).
  - Consider emitting a warning log when drops happen the first time per window to avoid log spam.

## Sink (stdout)

- Interface allows easy testability; production impl prints one JSON line per window with fields matching `internal/sink.Snapshot`:
  {"window_start":1710000000000,"window_end":1710000005000,"attribute_key":"foo","counts":{"alpha":25,"beta":10},"total":35,"dropped":0}
- `outputFormat` flag exists but only JSON is implemented at the moment.

## Metrics & Logging

- Metrics (OTel):
  - `com.dash0.homeexercise.logs.received` (counter): per LogRecord seen.
  - `com.dash0.homeexercise.logs.processed` (counter): successfully enqueued/aggregated.
  - `com.dash0.homeexercise.logs.dropped` (counter): dropped due to backpressure.
  - `com.dash0.homeexercise.flushes` (counter): number of window flushes.
  - `com.dash0.homeexercise.publish.failed` (counter): failed snapshot publishes.
  - Note: queue depth gauge is not implemented currently.
- Logging (slog via otelslog bridge):
  - Startup/shutdown, aggregator activity, and Export summaries.
  - All variables and state are instance-level (no package-level mutable globals).

## Extensibility

- Pluggable sink: `Sink` interface enables additional outputs (e.g., OTLP, file) without touching ingestion/aggregation.
- Optional processor: Insert a processor interface between `Export` and aggregator later for enrichment/filters (no-op default now).
- Windowing policy: Encapsulate in aggregator; support alternate window strategies via a strategy type.
- Admin endpoints: Add an HTTP server (health, pprof, Prometheus) behind flags without changing core service APIs.
- Telemetry wiring: Start with global providers for simplicity; later, switch to instance-scoped providers by passing them into gRPC handlers and storing instruments on `Service`.
- Configuration growth: Extend `Config` with new flags/env without refactoring call sites; validation remains centralized.

## Lifecycle & Shutdown

- Main sets up OTel, logger, parses config, constructs `Service` and `Aggregator` (instance-level), starts aggregator goroutine, then gRPC server.
- Trap `SIGINT/SIGTERM` (or use context cancellation):
  - Stop accepting new gRPC calls gracefully.
  - Signal aggregator to stop and flush final snapshot.
  - Shutdown OTel providers.
- `gracefulTimeout` bounds total shutdown time.

## Performance Considerations

- Keep `Event` lightweight (`string` only), avoid per-record allocations by reusing buffers only if profiling shows pressure (can introduce `sync.Pool` later).
- Avoid locks on hot path by goroutine ownership of aggregation state.
- Prefer pre-sized maps when the number of distinct values is known/predictable (start small and grow automatically; avoid premature optimization).
- Use `bufconn` for fast integration tests; use `-race` locally.

## Package Layout (current)

- `cmd/otlp-log-processor/main.go`: Entrypoint and server wiring.
- `internal/config`: Flags and config.
- `internal/otel`: OpenTelemetry setup.
- `internal/orchestrator`: Instance-scoped service, metrics, lifecycle.
- `internal/otlp`: OTLP Logs service (Export) and helpers.
- `internal/aggregator`: Windowed aggregator.
- `internal/sink`: JSON sink (stdout or file).

## Testing Strategy

- Unit tests:
  - Attribute extraction precedence (Log > Scope > Resource) and type conversions.
  - Aggregator: enqueue events, tick, verify snapshot counts and reset behavior; drops when channel full.
  - Sink: fake sink capturing last `Snapshot` for assertions.
- Integration test (bufconn):
  - Start instance-scoped `Service` with fake sink and small window; send an `ExportLogsServiceRequest` with sample records; assert published snapshot matches expected counts (including `"unknown"`).
  - Verify `PartialSuccess.RejectedLogRecords` when `maxQueue=0` or artificially saturated.
- Concurrency checks: run with `-race`.
- Benchmarks (optional): aggregator ingest and flush.

## Step-by-Step Implementation Tasks

1) Config and wiring (instance-level)
   - Introduce `Config`, parse flags, validate.
   - Build `Service` with instance-scoped logger, tracer, meter, and metrics instruments; remove/avoid package-level mutable globals.
   - Keep OTel setup functions but return providers/instruments to be stored on the `Service` instance.

2) Attribute extraction helper
   - Implement `ExtractAttribute` with precedence and canonical string conversion.
   - Add focused unit tests.

3) Sink interface and stdout implementation
   - Implement `Sink` with `StdoutSink` (JSON line output) and optional `log` format.
   - Add fake sink for tests.

4) Aggregator
   - Implement `Aggregator` with input channel, ticker, flush, and reset.
   - Expose `Start(ctx)` and `Stop(ctx)`; ensure final flush on stop.
   - Metrics: increment `flushes_total`, measure `queue_depth` via callback.
   - Unit tests for counts, drops, and flush behavior.

5) Export implementation
   - Refactor `dash0LogsServiceServer` to hold a `*Service` instance.
   - Iterate logs, extract attribute, enqueue events; track `rejected` drops.
   - Update metrics: `logs.received_total`, `logs.processed_total`, `logs.dropped_total`.
   - Return `PartialSuccess` when `rejected > 0`.
   - Add trace spans around processing loop.

6) Server and lifecycle
   - Start aggregator before serving; ensure graceful shutdown signals aggregator and gRPC.
   - Log startup config and bind address.

7) Tests
   - Update/add unit tests for extractor, aggregator, and Export with bufconn and fake sink.
   - Keep tests deterministic (inject `nowFn` into aggregator for fixed window times).

8) Documentation
   - Update README: flags, behavior, example output, testing instructions.

## Acceptance Criteria

- Instance-level state only (no package-level mutable variables for service logic or meters/instruments).
- For a sample input (as in README), per-window JSONL output matches expected counts including `"unknown"`; `window_start/window_end` are Unix millis.
- Under queue saturation, `PartialSuccess.RejectedLogRecords` reflects drops and `logs.dropped_total` increments.
- `go test ./...` passes; integration and unit tests verify extractor, aggregator, and export behavior.
- Graceful shutdown flushes final snapshot.
- Logs and metrics provide enough detail to operate the service.

## Open Questions / Assumptions

- Precedence chosen as Log > Scope > Resource (common-sense default); confirm if different precedence is desired.
- Non-string attributes are stringified (no normalization like lowercasing unless requested).
- TLS is optional; default insecure for local use; can be added if required.
