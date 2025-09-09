# OTLP Log Processor (Go)

**Overview**
- **Purpose:** Receives OTLP Logs over gRPC and aggregates counts per distinct value of a configurable attribute key in fixed windows, then emits a JSON Lines (JSONL) snapshot per window.
- **Signals:** Uses OpenTelemetry for traces/metrics/logs. A gRPC server interceptor creates a span per RPC. Entry/exit debug logs exist in the logs service and orchestrator. Aggregation window flushes are counted via metrics.
- **Output:** One JSON object per line containing `window_start`, `window_end`, `attribute_key`, `counts`, `total`, and `dropped`. By default it writes to stdout; you can redirect snapshots to a file via `-outputFile`.

**Prerequisites**
- **Go:** `1.23+` to build and run locally.
- **Docker:** Docker Engine and the Compose plugin for the quick demo.
- **golangci-lint (optional):** Install via `make lint-tools`.

**Build And Run (Binary)**
- **Build:** `go build -o bin/otlp-log-processor ./cmd/otlp-log-processor`
- **Run:** `./bin/otlp-log-processor -listenAddr :4317 -attributeKey foo -window 5s -maxQueue 100000 -outputFile ./snapshots.jsonl`
- The server listens for OTLP Logs over gRPC on `-listenAddr`.

**Quick Demo (Docker Compose)**
- **Start:** `make compose-up`
  - Starts the app and two telemetry generators producing logs with `foo=alpha` and `foo=beta`.
  - The app is configured to write snapshots to a mounted file at `./data/snapshots.jsonl`.
- **Watch snapshots:** `tail -f data/snapshots.jsonl`
- **Stop:** `make compose-down`


**Command Line Arguments**
- `-listenAddr`: gRPC listen address (default `localhost:4317`).
- `-maxReceiveMessageSize`: Max gRPC message size in bytes (default `16777216`).
- `-attributeKey`: Attribute key to aggregate on (default `foo`).
- `-window`: Aggregation window duration (default `10s`).
- `-maxQueue`: Max ingestion queue size for the aggregator (default `100000`).
- `-outputFormat`: Output format `json|log` (default `json`, only supports json for now).
- `-outputFile`: Path to a JSONL file to write snapshots to; if empty, writes to stdout (default empty).
- `-logLevel`: `debug|info|warn|error` (default `info`).
- `-gracefulTimeout`: Timeout for graceful shutdown (default `10s`).

**Make Targets**
- `unit`: Runs unit tests.
- `test`: Alias for `unit`.
- `lint-tools`: Installs `golangci-lint` v2.4.0.
- `lint`: Runs linters (`errcheck`, `govet`, `staticcheck`, `wsl_v5`, `gofmt`, `goimports`, etc.).
- `lint-fix`: Attempts auto-fixes where supported.
- `docker-build`: Builds the container image locally.
- `compose-up`: Builds and starts the demo stack (app + telemetry generators).
- `compose-down`: Stops the demo stack and removes volumes.
- `compose-logs`: Tails app logs from Compose (useful when not writing snapshots to a file).

**Development**
- Initial setup: `make setup`
  - Installs `golangci-lint` and `lefthook`, and installs Git hooks.
  - Pre-commit hook runs `make lint` to keep quality consistent.
- Run linters manually: `make lint` (or `make lint-fix` to auto-fix where possible).
- Update or reinstall hooks: re-run `make setup` (or `lefthook install`).
- Remove hooks: `lefthook uninstall` (you can reinstall later with `make setup`).

**Snapshot Format (JSONL)**
- Each line is a JSON object with fields:
  - `window_start`: Unix millis for window start
  - `window_end`: Unix millis for window end
  - `attribute_key`: Key used for aggregation
  - `counts`: Map of attribute value -> count within the window
  - `total`: Number of records processed in the window
  - `dropped`: Number of dropped records (e.g., due to backpressure)

Example line:
`{"window_start":1710000000000,"window_end":1710000005000,"attribute_key":"foo","counts":{"alpha":25,"beta":10},"total":35,"dropped":0}`

**Graceful Shutdown**
- Receives `SIGINT`/`SIGTERM`, stops accepting new RPCs via gRPC `GracefulStop`, then cancels the aggregator and waits for the final flush within `-gracefulTimeout`. If `--outputFile` is used, it is closed after shutdown completes.

**Repository Structure**
- `cmd/otlp-log-processor`: Main entrypoint and server wiring.
- `internal/otlp`: gRPC Logs service (`Export`), attribute helpers, and tests.
- `internal/orchestrator`: Service lifecycle, metrics, wiring to the aggregator and sink.
- `internal/aggregator`: Windowed aggregator with non-blocking ingestion and periodic flush.
- `internal/sink`: JSON sink writing to an `io.Writer` (stdout or a file when configured).
