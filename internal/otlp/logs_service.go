package otlp

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"

	"dash0.com/otlp-log-processor-backend/internal/orchestrator"
)

type logsServiceServer struct {
	orchestratorSvc orchestrator.Orchestrator
	collogspb.UnimplementedLogsServiceServer
}

// NewServer returns a LogsServiceServer backed by the provided Orchestrator.
func NewServer(svc orchestrator.Orchestrator) collogspb.LogsServiceServer {
	return &logsServiceServer{orchestratorSvc: svc}
}

func (l *logsServiceServer) Export(ctx context.Context, request *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	// Use the span started by the gRPC OTel interceptor.
	span := oteltrace.SpanFromContext(ctx)

	slog.DebugContext(ctx, "Received ExportLogsServiceRequest")

	var rejected uint64

	var receivedCount int64

	var processedCount int64

	var droppedCount int64

	// Collect attribute values for this request and enqueue as a single batch.
	var batch []string

	for _, rl := range request.GetResourceLogs() {
		// Safe even if Resource is nil; GetAttributes() returns nil in that case.
		resAttrs := rl.GetResource().GetAttributes()

		for _, sl := range rl.GetScopeLogs() {
			scopeAttrs := sl.GetScope().GetAttributes()

			for _, rec := range sl.GetLogRecords() {
				receivedCount++

				val, ok := ExtractAttrs(l.orchestratorSvc.AttributeKey(), rec.GetAttributes(), scopeAttrs, resAttrs)
				if !ok {
					val = "unknown"
				}

				batch = append(batch, val)
			}
		}
	}

	// Enqueue non-blocking as a single batch; on failure, record drop and rejected for all.
	if l.orchestratorSvc.EnqueueBatch(batch) {
		processedCount += int64(len(batch))
	} else {
		rejected += uint64(len(batch))
		droppedCount += int64(len(batch))
		l.orchestratorSvc.RecordDrop(uint64(len(batch)))
	}

	// Update metrics once per request.
	l.orchestratorSvc.IncrMetric(ctx, orchestrator.MetricLogsReceived, receivedCount)
	l.orchestratorSvc.IncrMetric(ctx, orchestrator.MetricLogsProcessed, processedCount)
	l.orchestratorSvc.IncrMetric(ctx, orchestrator.MetricLogsDropped, droppedCount)

	resp := &collogspb.ExportLogsServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collogspb.ExportLogsPartialSuccess{RejectedLogRecords: int64(rejected)}
	}
	// Add summary attributes to the RPC span and exit debug log
	span.SetAttributes(
		attribute.Int64("logs.received", receivedCount),
		attribute.Int64("logs.processed", processedCount),
		attribute.Int64("logs.dropped", droppedCount),
		attribute.Int64("logs.rejected", int64(rejected)),
		attribute.Int("batch.size", len(batch)),
	)
	slog.DebugContext(
		ctx,
		"Completed ExportLogsServiceRequest",
		slog.Int64("received", receivedCount),
		slog.Int64("processed", processedCount),
		slog.Int64("dropped", droppedCount),
		slog.Uint64("rejected", rejected),
		slog.Int("batch_size", len(batch)),
	)

	return resp, nil
}
