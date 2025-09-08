package orchestrator

import (
    "context"
    "log/slog"

    "go.opentelemetry.io/otel"
    otelmetric "go.opentelemetry.io/otel/metric"
    oteltrace "go.opentelemetry.io/otel/trace"
    cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
    "dash0.com/otlp-log-processor-backend/internal/agg"
    "dash0.com/otlp-log-processor-backend/internal/sink"
)

const instrumentationName = "dash0.com/otlp-log-processor-backend"

// Service holds all instance-scoped dependencies and metrics.
type Service struct {
    Cfg    cfgpkg.Config
    Logger *slog.Logger
    Tracer oteltrace.Tracer
    Meter  otelmetric.Meter

    // Metrics
    LogsReceived  otelmetric.Int64Counter
    LogsProcessed otelmetric.Int64Counter
    LogsDropped   otelmetric.Int64Counter
    Flushes       otelmetric.Int64Counter

    Aggregator *agg.Aggregator

    outSink sink.Sink

    aggCancel context.CancelFunc
}

// New constructs a Service with instance-level instruments.
type Option func(*Service) error

// WithSink overrides the default stdout JSON sink with a custom sink (useful for tests).
func WithSink(s sink.Sink) Option { return func(svc *Service) error { svc.outSink = s; return nil } }

func New(cfg cfgpkg.Config, logger *slog.Logger, opts ...Option) (*Service, error) {
    s := &Service{
        Cfg:    cfg,
        Logger: logger,
        Tracer: otel.Tracer(instrumentationName),
        Meter:  otel.Meter(instrumentationName),
    }

    var err error
    if s.LogsReceived, err = s.Meter.Int64Counter(
        "com.dash0.homeexercise.logs.received",
        otelmetric.WithDescription("The number of logs received by otlp-log-processor-backend"),
        otelmetric.WithUnit("{log}"),
    ); err != nil {
        return nil, err
    }
    if s.LogsProcessed, err = s.Meter.Int64Counter(
        "com.dash0.homeexercise.logs.processed",
        otelmetric.WithDescription("The number of logs processed by otlp-log-processor-backend"),
        otelmetric.WithUnit("{log}"),
    ); err != nil {
        return nil, err
    }
    if s.LogsDropped, err = s.Meter.Int64Counter(
        "com.dash0.homeexercise.logs.dropped",
        otelmetric.WithDescription("The number of logs dropped by otlp-log-processor-backend"),
        otelmetric.WithUnit("{log}"),
    ); err != nil {
        return nil, err
    }
    if s.Flushes, err = s.Meter.Int64Counter(
        "com.dash0.homeexercise.flushes",
        otelmetric.WithDescription("Number of aggregation window flushes"),
        otelmetric.WithUnit("{flush}"),
    ); err != nil {
        return nil, err
    }

    // Apply options
    for _, opt := range opts {
        if err := opt(s); err != nil { return nil, err }
    }

    // Default sink to stdout JSON if not set
    if s.outSink == nil { s.outSink = sink.NewStdoutJSON() }

    // Aggregator
    s.Aggregator = agg.New(cfg.Window, cfg.AttributeKey, s.outSink, logger, cfg.MaxQueue)

    return s, nil
}

// Close is a placeholder for future instance-scoped shutdown.
func (s *Service) Close(ctx context.Context) error {
    if s.aggCancel != nil {
        s.aggCancel()
        if s.Aggregator != nil {
            s.Aggregator.Stop(ctx)
        }
        s.aggCancel = nil
    }
    return nil
}

// Start starts the service’s internal components (e.g., the aggregator).
// It is safe to call more than once; subsequent calls are no-ops until Close.
func (s *Service) Start(ctx context.Context) {
    if s.Aggregator == nil || s.aggCancel != nil {
        return
    }
    aggCtx, cancel := context.WithCancel(ctx)
    s.aggCancel = cancel
    s.Aggregator.Start(aggCtx)
}
