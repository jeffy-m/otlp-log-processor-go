package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
	"dash0.com/otlp-log-processor-backend/internal/orchestrator"
	otelsetup "dash0.com/otlp-log-processor-backend/internal/otel"
	otlpsrv "dash0.com/otlp-log-processor-backend/internal/otlp"
	"dash0.com/otlp-log-processor-backend/internal/sink"
)

const name = "dash0.com/otlp-log-processor-backend"

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() (err error) {
	// Instance logger bridged to OTel.
	logger := otelslog.NewLogger(name)
	slog.SetDefault(logger)
	logger.Info("Starting application")

	// Set up OpenTelemetry.
	otelShutdown, err := otelsetup.Setup(context.Background())
	if err != nil {
		return
	}

	defer func() { err = errors.Join(err, otelShutdown(context.Background())) }()

	// Config
	readFlags := cfgpkg.RegisterFlags()

	flag.Parse()

	cfg := readFlags()

	slog.Debug("Starting listener", slog.String("listenAddr", cfg.ListenAddr))

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}

	// Optional output file for JSON sink
	var outFile *os.File
	if cfg.OutputFile != "" {
		f, openErr := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if openErr != nil {
			return openErr
		}

		outFile = f
	}

	var opts []orchestrator.Option
	if outFile != nil {
		opts = append(opts, orchestrator.WithSink(sink.NewJSONSink(outFile)))
	}

	orchestratorSvc, err := orchestrator.New(cfg, logger, opts...)
	if err != nil {
		return err
	}
	// Derive a context canceled on SIGINT/SIGTERM for graceful shutdown
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start internal components; they will stop when sigCtx is canceled
	orchestratorSvc.Start(sigCtx)

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.MaxRecvMsgSize(cfg.MaxReceiveMessageSize),
		grpc.Creds(insecure.NewCredentials()),
	)
	collogspb.RegisterLogsServiceServer(grpcServer, otlpsrv.NewServer(orchestratorSvc))

	slog.Debug("Starting gRPC server")

	// Serve in a goroutine so we can handle signals
	serveErr := make(chan error, 1)

	go func() { serveErr <- grpcServer.Serve(listener) }()

	select {
	case err := <-serveErr:
		return err
	case <-sigCtx.Done():
		// Begin graceful shutdown
		slog.Info("Shutdown signal received; beginning graceful shutdown")
		// Stop accepting new connections and allow in-flight RPCs to complete
		done := make(chan struct{})

		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		// Bound the shutdown with configured timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulTimeout)
		defer cancel()

		select {
		case <-done:
			// graceful stop completed
		case <-shutdownCtx.Done():
			slog.Warn("Graceful stop timed out; forcing stop")
			grpcServer.Stop()
		}

		// Cancel internal components and wait for close
		if err := orchestratorSvc.Close(shutdownCtx); err != nil {
			return err
		}
		// Close the output file if used
		if outFile != nil {
			if cerr := outFile.Close(); cerr != nil {
				return cerr
			}
		}

		return nil
	}
}
