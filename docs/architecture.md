# Architecture Diagrams — OTLP Log Processor (Go)

## Component Overview

```mermaid
flowchart TD
  Client[Client(s)] -->|OTLP Logs Export (gRPC)| GRPC[grpc.Server]
  GRPC --> LogsSvc[internal/otlp\nLogsServiceServer]

  subgraph App Process
    Cmd[cmd/otlp-log-processor\nmain]
    Cfg[internal/config\nConfig]
    OTel[internal/otel\nSetup]
    Orch[internal/orchestrator\nService (instance-scoped)]
    Extract[internal/otlp\nExtractAttrs(key,log,scope,res)]
    Queue[(Ingestion Queue\nchan Event)]
    Agg[internal/aggregator\nAggregator (goroutine\n+ticker windowing)]
    Sink[internal/sink\nJSONSink (stdout)]
  end

  Cmd -->|flag.Parse| Cfg
  Cmd -->|Setup(ctx)| OTel
  Cmd -->|New(cfg, logger)| Orch
  Cmd -->|start| GRPC
  Orch -->|owns| Agg
  Agg -->|Publish(Snapshot)| Sink

  LogsSvc -->|for each record| Extract
  Extract -->|value or "unknown"| Queue
  Agg <-->|non-blocking Enqueue/EnqueueBatch| Queue

  %% Telemetry (observability) paths
  GRPC -. traces/metrics .-> OTel
  LogsSvc -. counters .-> OTel
  Agg -. counters/gauges .-> OTel
```

Key points:
- Single-queue → single-writer aggregator for simplicity and throughput.
- Attribute extraction precedence: Log > Scope > Resource; fallback "unknown".
- Backpressure via bounded channel; drops accounted in PartialSuccess and metrics.
- Sink is pluggable; JSON stdout is the default implementation.

## Export Path — Sequence

```mermaid
sequenceDiagram
  participant Client
  participant GRPC as grpc.Server
  participant Logs as LogsServiceServer
  participant EA as ExtractAttrs
  participant Agg as Aggregator
  participant Out as Sink

  Client->>GRPC: ExportLogsServiceRequest
  GRPC->>Logs: request
  activate Logs
  loop each Resource/Scope/LogRecord
    Logs->>EA: ExtractAttrs(key, log, scope, resource)
    EA-->>Logs: value | "unknown"
    Logs->>Agg: EnqueueBatch(values) (non-blocking)
    alt queue full
      Logs->>Agg: RecordDrop(1)
      Logs-->>Logs: rejected++ (PartialSuccess)
    else ok
      Logs-->>Logs: metrics.logsProcessed++
    end
  end
  deactivate Logs
  Agg-->>Out: Publish(Snapshot) on window tick
  GRPC-->>Client: ExportLogsServiceResponse (PartialSuccess.rejected)
```

## Packages and Dependencies

```mermaid
flowchart LR
  subgraph internal
    G[internal/agg]
    O[internal/otel]
    C[internal/config]
    R[internal/orchestrator]
    L[internal/otlp]
    P[internal/sink]
  end
  M[cmd/otlp-log-processor]

  M --> C
  M --> O
  M --> R
  M --> L
  R --> G
  G --> P
  L --> R
```

Legend:
- internal/*: core business logic and app wiring (not importable by external modules).
- cmd/*: binary entrypoint.
