package otlp

import (
    "context"
    "log/slog"

    collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
    "dash0.com/otlp-log-processor-backend/internal/orchestrator"
)

type logsServiceServer struct {
    orchestratorSvc *orchestrator.Service
    collogspb.UnimplementedLogsServiceServer
}

// NewServer returns a LogsServiceServer backed by the provided Service.
func NewServer(svc *orchestrator.Service) collogspb.LogsServiceServer {
    return &logsServiceServer{orchestratorSvc: svc}
}

func (l *logsServiceServer) Export(ctx context.Context, request *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
    slog.DebugContext(ctx, "Received ExportLogsServiceRequest")

    var rejected uint64

    for _, rl := range request.GetResourceLogs() {
        // Safe even if Resource is nil; GetAttributes() returns nil in that case.
        resAttrs := rl.GetResource().GetAttributes()
        for _, sl := range rl.GetScopeLogs() {
            scopeAttrs := sl.GetScope().GetAttributes()
            for _, rec := range sl.GetLogRecords() {
                // Count received per record.
                l.orchestratorSvc.LogsReceived.Add(ctx, 1)
                val, ok := ExtractAttrs(l.orchestratorSvc.Cfg.AttributeKey, rec.GetAttributes(), scopeAttrs, resAttrs)
                if !ok || val == "" {
                    val = "unknown"
                }

                // Enqueue non-blocking; on failure, record drop and rejected.
                if l.orchestratorSvc.Aggregator != nil {
                    if ok := l.orchestratorSvc.Aggregator.Enqueue(val); ok {
                        l.orchestratorSvc.LogsProcessed.Add(ctx, 1)
                    } else {
                        rejected++
                        l.orchestratorSvc.LogsDropped.Add(ctx, 1)
                        l.orchestratorSvc.Aggregator.RecordDrop(1)
                    }
                } else {
                    // If no aggregator wired (e.g., in tests), consider it processed.
                    l.orchestratorSvc.LogsProcessed.Add(ctx, 1)
                }
            }
        }
    }

    resp := &collogspb.ExportLogsServiceResponse{}
    if rejected > 0 {
        resp.PartialSuccess = &collogspb.ExportLogsPartialSuccess{RejectedLogRecords: int64(rejected)}
    }
    return resp, nil
}
