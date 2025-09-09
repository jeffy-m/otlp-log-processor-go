package aggregator

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dash0.com/otlp-log-processor-backend/internal/sink"
)

type fakeSink struct {
	got atomic.Pointer[sink.Snapshot]
	ch  chan struct{}
}

func (f *fakeSink) Publish(_ context.Context, s sink.Snapshot) error {
	f.got.Store(&s)

	select {
	case f.ch <- struct{}{}:
	default:
	}

	return nil
}

func TestAggregator_FlushesCounts(t *testing.T) {
	fs := &fakeSink{ch: make(chan struct{}, 1)}
	a := New(30*time.Millisecond, "foo", fs, slog.Default(), 10)
	ctx, cancel := context.WithCancel(context.Background())
	a.Start(ctx)
	// ensure cancel happens before waiting in Stop
	defer a.Stop(context.Background())
	defer cancel()

	require.True(t, a.Enqueue("bar"))
	require.True(t, a.Enqueue("baz"))
	require.True(t, a.Enqueue("bar"))

	require.Eventually(t, func() bool { return fs.got.Load() != nil }, 300*time.Millisecond, 5*time.Millisecond)

	snap := fs.got.Load()
	require.NotNil(t, snap)
	require.Equal(t, "foo", snap.AttributeKey)
	require.EqualValues(t, 3, snap.Total)
	assert.EqualValues(t, 2, snap.Counts["bar"]) // independent count checks
	assert.EqualValues(t, 1, snap.Counts["baz"])
}

func TestAggregator_RecordsDrops(t *testing.T) {
	fs := &fakeSink{ch: make(chan struct{}, 1)}
	a := New(20*time.Millisecond, "foo", fs, slog.Default(), 0)
	ctx, cancel := context.WithCancel(context.Background())
	a.Start(ctx)
	// ensure cancel happens before waiting in Stop
	defer a.Stop(context.Background())
	defer cancel()

	// With maxQueue=0, Enqueue always drops.
	require.False(t, a.Enqueue("v"))
	a.RecordDrop(1)

	require.Eventually(t, func() bool {
		return fs.got.Load() != nil
	}, 250*time.Millisecond, 5*time.Millisecond)

	snap := fs.got.Load()
	require.NotNil(t, snap)
	require.EqualValues(t, 1, snap.Dropped)
}
