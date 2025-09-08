package otlp

import (
    "context"
    "io"
    "log/slog"
    "sync"
    "testing"
    "time"

    collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
    commonpb "go.opentelemetry.io/proto/otlp/common/v1"
    otellogs "go.opentelemetry.io/proto/otlp/logs/v1"
    resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

    cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
    "dash0.com/otlp-log-processor-backend/internal/orchestrator"
    "dash0.com/otlp-log-processor-backend/internal/mocks"
    "dash0.com/otlp-log-processor-backend/internal/sink"
    "github.com/stretchr/testify/require"
    "go.uber.org/mock/gomock"
)

func makeSvc(t *testing.T, maxQueue int) *orchestrator.Service {
    t.Helper()
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    cfg := cfgpkg.Config{
        ListenAddr:            "",
        MaxReceiveMessageSize: 16 * 1024 * 1024,
        AttributeKey:          "foo",
        Window:                20 * time.Millisecond,
        MaxQueue:              maxQueue,
        OutputFormat:          "json",
        LogLevel:              "info",
        GracefulTimeout:       1 * time.Second,
    }
    svc, err := orchestrator.New(cfg, logger)
    if err != nil { t.Fatalf("service.New error: %v", err) }
    return svc
}

func reqWithN(n int) *collogspb.ExportLogsServiceRequest {
    recs := make([]*otellogs.LogRecord, 0, n)
    for i := 0; i < n; i++ {
        recs = append(recs, &otellogs.LogRecord{})
    }
    return &collogspb.ExportLogsServiceRequest{
        ResourceLogs: []*otellogs.ResourceLogs{
            {
                ScopeLogs: []*otellogs.ScopeLogs{{ LogRecords: recs }},
            },
        },
    }
}

func TestExport_NoDrops_NoPartialSuccess(t *testing.T) {
    svc := makeSvc(t, 10)
    srv := NewServer(svc)

    out, err := srv.Export(context.Background(), reqWithN(3))
    require.NoError(t, err)
    require.Zero(t, out.GetPartialSuccess().GetRejectedLogRecords())
    require.Empty(t, out.GetPartialSuccess().GetErrorMessage())
}

func TestExport_AllDrops_ReportsPartialSuccess(t *testing.T) {
    svc := makeSvc(t, 0) // queue size 0 -> Enqueue fails
    srv := NewServer(svc)

    out, err := srv.Export(context.Background(), reqWithN(5))
    require.NoError(t, err)
    require.EqualValues(t, 5, out.GetPartialSuccess().GetRejectedLogRecords())
}

// Build a request exercising attribute precedence: Log > Scope > Resource > unknown.
func buildPrecedenceRequest(t *testing.T, key string) *collogspb.ExportLogsServiceRequest {
    t.Helper()
    // Helpers
    kvStr := func(k, v string) *commonpb.KeyValue {
        return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
    }
    // Resource with foo=resv
    res := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{kvStr(key, "resv")}}
    // Scope1 with foo=scopev
    scope1 := &otellogs.ScopeLogs{Scope: &commonpb.InstrumentationScope{Attributes: []*commonpb.KeyValue{kvStr(key, "scopev")}}}
    // Scope2 with no foo
    scope2 := &otellogs.ScopeLogs{Scope: &commonpb.InstrumentationScope{}}

    // Records
    recLog := &otellogs.LogRecord{Attributes: []*commonpb.KeyValue{kvStr(key, "logv")}}
    recScope := &otellogs.LogRecord{}                       // picks scopev
    recScope2 := &otellogs.LogRecord{}                      // picks scopev
    recRes := &otellogs.LogRecord{}                         // in scope2 -> resv
    recUnknown := &otellogs.LogRecord{Attributes: []*commonpb.KeyValue{kvStr("other", "x")}} // unknown

    scope1.LogRecords = []*otellogs.LogRecord{recLog, recScope, recScope2}
    scope2.LogRecords = []*otellogs.LogRecord{recRes, recUnknown}

    return &collogspb.ExportLogsServiceRequest{
        ResourceLogs: []*otellogs.ResourceLogs{{
            Resource:  res,
            ScopeLogs: []*otellogs.ScopeLogs{scope1, scope2},
        }},
    }
}

func TestExport_AttributePrecedence_AggregatesCounts(t *testing.T) {
    // gomock sink with WaitGroup to signal completion.
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    ms := mocks.NewMockSink(ctrl)
    var captured sink.Snapshot
    var wg sync.WaitGroup
    wg.Add(1)
    first := ms.EXPECT().Publish(gomock.Any(), gomock.AssignableToTypeOf(sink.Snapshot{})).DoAndReturn(
        func(_ context.Context, s sink.Snapshot) error {
            captured = s
            wg.Done()
            return nil
        },
    ).Times(1)
    // If there were multiple publishes, we could chain .After(first) on subsequent expectations.
    _ = first

    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    cfg := cfgpkg.Config{ListenAddr: "", MaxReceiveMessageSize: 1024, AttributeKey: "foo", Window: 30 * time.Millisecond, MaxQueue: 10, OutputFormat: "json", LogLevel: "info", GracefulTimeout: time.Second}
    svc, err := orchestrator.New(cfg, logger, orchestrator.WithSink(ms))
    require.NoError(t, err)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    svc.Start(ctx)

    srv := NewServer(svc)

    req := buildPrecedenceRequest(t, "foo")
    _, err = srv.Export(context.Background(), req)
    require.NoError(t, err)

    done := make(chan struct{})
    go func(){ wg.Wait(); close(done) }()
    require.Eventually(t, func() bool {
        select {
        case <-done:
            return true
        default:
            return false
        }
    }, 1*time.Second, 10*time.Millisecond)

    counts := captured.Counts
    require.EqualValues(t, 1, counts["logv"])
    require.EqualValues(t, 2, counts["scopev"])
    // Resource-level attribute covers records in scope without key; both records in scope2 fall back to resource.
    require.EqualValues(t, 2, counts["resv"])
    require.NotContains(t, counts, "unknown")
}
