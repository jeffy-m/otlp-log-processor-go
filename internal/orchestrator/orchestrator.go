package orchestrator

//go:generate mockgen -source=orchestrator.go -destination=./mocks/mock_orchestrator.go -package=mocks

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"dash0.com/otlp-log-processor-backend/internal/aggregator"
	cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
	"dash0.com/otlp-log-processor-backend/internal/sink"
)

const instrumentationName = "dash0.com/otlp-log-processor-backend"

type Orchestrator interface {
	AttributeKey() string
	EnqueueBatch(values []string) bool
	RecordDrop(n uint64)
	IncrMetric(ctx context.Context, mt MetricType, n int64)
}

// orchestratorSvc holds all instance-scoped dependencies and metrics.
type orchestratorSvc struct {
	Cfg    cfgpkg.Config
	Logger *slog.Logger
	Tracer oteltrace.Tracer
	Meter  otelmetric.Meter

	// Metrics
	LogsReceived  otelmetric.Int64Counter
	LogsProcessed otelmetric.Int64Counter
	LogsDropped   otelmetric.Int64Counter
	Flushes       otelmetric.Int64Counter
	PublishFailed otelmetric.Int64Counter

	Aggregator *aggregator.Aggregator

	outSink sink.Sink

	aggCancel context.CancelFunc
}

// New constructs a Service with instance-level instruments.
type Option func(*orchestratorSvc) error

// WithSink overrides the default stdout JSON sink with a custom sink (useful for tests).
func WithSink(s sink.Sink) Option {
	return func(svc *orchestratorSvc) error { svc.outSink = s; return nil }
}

func New(cfg cfgpkg.Config, logger *slog.Logger, opts ...Option) (*orchestratorSvc, error) {
	s := &orchestratorSvc{
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

	if s.PublishFailed, err = s.Meter.Int64Counter(
		"com.dash0.homeexercise.publish.failed",
		otelmetric.WithDescription("Number of failed snapshot publishes"),
		otelmetric.WithUnit("{failure}"),
	); err != nil {
		return nil, err
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Default sink to stdout JSON if not set
	if s.outSink == nil {
		s.outSink = sink.NewStdoutJSON()
	}

	// Aggregator
	s.Aggregator = aggregator.New(cfg.Window, cfg.AttributeKey, s.outSink, logger, cfg.MaxQueue)
	// Wire aggregator metric callbacks
	s.Aggregator.SetMetricsCallbacks(
		func(n int64) { s.IncrMetric(context.Background(), MetricFlushes, n) },
		func(n int64) { s.IncrMetric(context.Background(), MetricPublishFailed, n) },
	)

	return s, nil
}

// Close is a placeholder for future instance-scoped shutdown.
func (s *orchestratorSvc) Close(ctx context.Context) error {
	ctx, span := s.Tracer.Start(ctx, "orchestrator.Close")
	defer span.End()

	s.Logger.DebugContext(ctx, "orchestrator.Close: begin")

	if s.aggCancel != nil {
		s.aggCancel()

		if s.Aggregator != nil {
			s.Aggregator.Stop(ctx)
		}

		s.aggCancel = nil
	}

	s.Logger.DebugContext(ctx, "orchestrator.Close: end")

	return nil
}

// Start starts the service’s internal components (e.g., the aggregator).
// It is safe to call more than once; subsequent calls are no-ops until Close.
func (s *orchestratorSvc) Start(ctx context.Context) {
	if s.Aggregator == nil || s.aggCancel != nil {
		return
	}

	ctx, span := s.Tracer.Start(ctx, "orchestrator.Start")
	defer span.End()

	s.Logger.DebugContext(ctx, "orchestrator.Start: begin")
	aggCtx, cancel := context.WithCancel(ctx)
	s.aggCancel = cancel
	s.Aggregator.Start(aggCtx)
	s.Logger.DebugContext(ctx, "orchestrator.Start: started aggregator", slog.Int("queue_len", s.Aggregator.QueueLen()))
}

// AttributeKey returns the configured attribute key used for aggregation.
func (s *orchestratorSvc) AttributeKey() string { return s.Cfg.AttributeKey }

// EnqueueBatch forwards a batch of values to the aggregator if present.
func (s *orchestratorSvc) EnqueueBatch(values []string) bool {
	if s.Aggregator == nil {
		return false
	}
	// Use a background context for logging/tracing as this method has no ctx param
	ctx, span := s.Tracer.Start(context.Background(), "orchestrator.EnqueueBatch")
	defer span.End()

	span.SetAttributes(attribute.Int("batch.size", len(values)))
	s.Logger.DebugContext(ctx, "orchestrator.EnqueueBatch: begin", slog.Int("batch_size", len(values)))
	ok := s.Aggregator.EnqueueBatch(values)
	s.Logger.DebugContext(ctx, "orchestrator.EnqueueBatch: end", slog.Bool("enqueued", ok), slog.Int("queue_len", s.Aggregator.QueueLen()))

	return ok
}

// RecordDrop forwards a drop count to the aggregator if present.
func (s *orchestratorSvc) RecordDrop(n uint64) {
	// Use a background context for logging/tracing as this method has no ctx param
	ctx, span := s.Tracer.Start(context.Background(), "orchestrator.RecordDrop")
	defer span.End()

	span.SetAttributes(attribute.Int64("dropped", int64(n)))
	s.Logger.DebugContext(ctx, "orchestrator.RecordDrop: begin", slog.Uint64("n", n))

	if s.Aggregator != nil {
		s.Aggregator.RecordDrop(n)
	}

	s.Logger.DebugContext(ctx, "orchestrator.RecordDrop: end")
}

// MetricType enumerates orchestrator metric counters.
type MetricType int

const (
	MetricLogsReceived MetricType = iota
	MetricLogsProcessed
	MetricLogsDropped
	MetricFlushes
	MetricPublishFailed
)

// IncrMetric increments the selected metric by n (if n > 0).
func (s *orchestratorSvc) IncrMetric(ctx context.Context, mt MetricType, n int64) {
	if n <= 0 {
		return
	}

	switch mt {
	case MetricLogsReceived:
		s.LogsReceived.Add(ctx, n)
	case MetricLogsProcessed:
		s.LogsProcessed.Add(ctx, n)
	case MetricLogsDropped:
		s.LogsDropped.Add(ctx, n)
	case MetricFlushes:
		s.Flushes.Add(ctx, n)
	case MetricPublishFailed:
		s.PublishFailed.Add(ctx, n)
	}
}
