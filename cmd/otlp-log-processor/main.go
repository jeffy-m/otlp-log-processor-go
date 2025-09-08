package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
	otelsetup "dash0.com/otlp-log-processor-backend/internal/otel"
	otlpsrv "dash0.com/otlp-log-processor-backend/internal/otlp"
	"dash0.com/otlp-log-processor-backend/internal/orchestrator"
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

	// Instance-scoped service
	orchestratorSvc, err := orchestrator.New(cfg, logger)
	if err != nil {
		return err
	}
	// Start internal components and ensure clean shutdown
	runCtx, runCancel := context.WithCancel(context.Background())
	orchestratorSvc.Start(runCtx)
	defer func() {
		runCancel()
		orchestratorSvc.Close(context.Background())
	}()

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.MaxRecvMsgSize(cfg.MaxReceiveMessageSize),
		grpc.Creds(insecure.NewCredentials()),
	)
    collogspb.RegisterLogsServiceServer(grpcServer, otlpsrv.NewServer(orchestratorSvc))

	slog.Debug("Starting gRPC server")
	return grpcServer.Serve(listener)
}
