package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	otellogs "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/stretchr/testify/require"

	cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
	"dash0.com/otlp-log-processor-backend/internal/orchestrator"
	otlpsrv "dash0.com/otlp-log-processor-backend/internal/otlp"
)

func TestLogsServiceServer_Export_Basic(t *testing.T) {
	ctx := context.Background()

	client, closer := startTestServer(t)
	defer closer()

	in := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*otellogs.ResourceLogs{
			{ScopeLogs: []*otellogs.ScopeLogs{}},
		},
	}

	out, err := client.Export(ctx, in)
	require.NoError(t, err)
	require.Zero(t, out.GetPartialSuccess().GetRejectedLogRecords())
	require.Empty(t, out.GetPartialSuccess().GetErrorMessage())
}

func startTestServer(t *testing.T) (collogspb.LogsServiceClient, func()) {
	t.Helper()

	addr := "localhost:4317"
	lis := bufconn.Listen(1024 * 1024)

	baseServer := grpc.NewServer()

	// Minimal instance service (no OTel setup needed for this test)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := cfgpkg.Config{
		ListenAddr:            addr,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		AttributeKey:          "foo",
		Window:                100 * time.Millisecond,
		MaxQueue:              10,
		OutputFormat:          "json",
		LogLevel:              "info",
		GracefulTimeout:       1 * time.Second,
	}
	svc, err := orchestrator.New(cfg, logger)
	require.NoError(t, err)

	collogspb.RegisterLogsServiceServer(baseServer, otlpsrv.NewServer(svc))

	go func() {
		if err := baseServer.Serve(lis); err != nil {
			log.Printf("error serving test server: %v", err)
		}
	}()

	conn, err := grpc.NewClient(addr,
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	closer := func() {
		_ = lis.Close()

		baseServer.Stop()
	}

	client := collogspb.NewLogsServiceClient(conn)

	return client, closer
}
