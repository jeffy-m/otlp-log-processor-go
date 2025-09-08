package otelsetup

import (
    "context"
    "errors"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
    "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
    "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
    "go.opentelemetry.io/otel/log/global"
    "go.opentelemetry.io/otel/propagation"
    sdklog "go.opentelemetry.io/otel/sdk/log"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

var res = resource.NewWithAttributes(
    semconv.SchemaURL,
    semconv.ServiceNameKey.String("otlp-log-processor-backend"),
    semconv.ServiceNamespaceKey.String("dash0-exercise"),
    semconv.ServiceVersionKey.String("1.0.0"),
)

// Setup bootstraps the OpenTelemetry pipeline and returns a shutdown func.
func Setup(ctx context.Context) (shutdown func(context.Context) error, err error) {
    var shutdownFuncs []func(context.Context) error

    shutdown = func(ctx context.Context) error {
        var err error
        for _, fn := range shutdownFuncs {
            err = errors.Join(err, fn(ctx))
        }
        shutdownFuncs = nil
        return err
    }

    handleErr := func(inErr error) {
        err = errors.Join(inErr, shutdown(ctx))
    }

    // Propagator
    prop := newPropagator()
    otel.SetTextMapPropagator(prop)

    // Traces
    tracerProvider, err := newTraceProvider()
    if err != nil { handleErr(err); return }
    shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
    otel.SetTracerProvider(tracerProvider)

    // Metrics
    meterProvider, err := newMeterProvider()
    if err != nil { handleErr(err); return }
    shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
    otel.SetMeterProvider(meterProvider)

    // Logs
    loggerProvider, err := newLoggerProvider()
    if err != nil { handleErr(err); return }
    shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
    global.SetLoggerProvider(loggerProvider)

    return
}

func newPropagator() propagation.TextMapPropagator {
    return propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    )
}

func newTraceProvider() (*sdktrace.TracerProvider, error) {
    traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
    if err != nil { return nil, err }
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
        sdktrace.WithBatcher(traceExporter, sdktrace.WithBatchTimeout(time.Second)),
    )
    return tp, nil
}

func newMeterProvider() (*sdkmetric.MeterProvider, error) {
    metricExporter, err := stdoutmetric.New(stdoutmetric.WithPrettyPrint())
    if err != nil { return nil, err }
    mp := sdkmetric.NewMeterProvider(
        sdkmetric.WithResource(res),
        sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
    )
    return mp, nil
}

func newLoggerProvider() (*sdklog.LoggerProvider, error) {
    logExporter, err := stdoutlog.New(stdoutlog.WithPrettyPrint())
    if err != nil { return nil, err }
    lp := sdklog.NewLoggerProvider(
        sdklog.WithResource(res),
        sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
    )
    return lp, nil
}

