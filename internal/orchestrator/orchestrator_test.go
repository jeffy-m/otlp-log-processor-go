package orchestrator

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfgpkg "dash0.com/otlp-log-processor-backend/internal/config"
	"dash0.com/otlp-log-processor-backend/internal/sink"
	"dash0.com/otlp-log-processor-backend/internal/sink/mocks"
)

func TestNew_ConstructsAggregator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := cfgpkg.Config{
		ListenAddr:            "",
		MaxReceiveMessageSize: 1024,
		AttributeKey:          "k",
		Window:                10 * time.Millisecond,
		MaxQueue:              2,
		OutputFormat:          "json",
		LogLevel:              "info",
		GracefulTimeout:       time.Second,
	}
	s, err := New(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, s.Aggregator)

	// Start and stop quickly to ensure no panics and proper lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	// Idempotent start
	s.Start(ctx)
	cancel()
	require.NoError(t, s.Close(context.Background()))
	// Idempotent close
	require.NoError(t, s.Close(context.Background()))

	// If Aggregator is nil, Start should no-op.
	s.Aggregator = nil
	s.Start(context.Background())
}

func TestNew_WithSink_PublishesSnapshot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := cfgpkg.Config{
		ListenAddr:            "",
		MaxReceiveMessageSize: 1024,
		AttributeKey:          "k",
		Window:                15 * time.Millisecond,
		MaxQueue:              4,
		OutputFormat:          "json",
		LogLevel:              "info",
		GracefulTimeout:       time.Second,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ms := mocks.NewMockSink(ctrl)

	// Expect at least one publish; capture to validate structure
	var got sink.Snapshot

	ms.EXPECT().Publish(gomock.Any(), gomock.AssignableToTypeOf(sink.Snapshot{})).DoAndReturn(
		func(_ context.Context, s sink.Snapshot) error { got = s; return nil },
	).MinTimes(1)

	s, err := New(cfg, logger, WithSink(ms))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Enqueue a couple of values and wait long enough for a flush tick.
	require.True(t, s.Aggregator.Enqueue("v1"))
	require.True(t, s.Aggregator.Enqueue("v2"))

	time.Sleep(60 * time.Millisecond)
	require.GreaterOrEqual(t, got.WindowEnd, got.WindowStart)
}
