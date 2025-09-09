package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"dash0.com/otlp-log-processor-backend/internal/sink"
)

// Event is a lightweight ingestion item carrying the attribute value.
type Event struct{ Value string }

// Aggregator performs windowed counting by attribute value and publishes snapshots.
type Aggregator struct {
	in           chan Event
	inBatch      chan []string
	window       time.Duration
	sink         sink.Sink
	logger       *slog.Logger
	attributeKey string

	nowFn func() time.Time

	// Single-goroutine owned fields
	counts map[string]uint64
	total  uint64

	// Drops recorded from producers when channel is full
	externalDropped atomic.Uint64

	done chan struct{}

	// Optional metric callbacks provided by the owner (e.g., orchestrator).
	incrFlushes       func(int64)
	incrPublishFailed func(int64)
}

func New(window time.Duration, attributeKey string, s sink.Sink, logger *slog.Logger, maxQueue int) *Aggregator {
	if maxQueue < 0 {
		maxQueue = 0
	}

	a := &Aggregator{
		in:           make(chan Event, maxQueue),
		inBatch:      make(chan []string, maxQueue),
		window:       window,
		sink:         s,
		logger:       logger,
		attributeKey: attributeKey,
		counts:       make(map[string]uint64, 32),
		done:         make(chan struct{}),
	}
	a.nowFn = time.Now

	return a
}

// SetMetricsCallbacks installs optional callbacks for metrics updates.
// If not provided, metrics are not recorded by the aggregator.
func (a *Aggregator) SetMetricsCallbacks(incrFlushes, incrPublishFailed func(int64)) {
	a.incrFlushes = incrFlushes
	a.incrPublishFailed = incrPublishFailed
}

// Enqueue attempts to add an event without blocking. Returns false if queue is full.
func (a *Aggregator) Enqueue(v string) bool {
	select {
	case a.in <- Event{Value: v}:
		return true
	default:
		return false
	}
}

// EnqueueBatch attempts to add a batch of events without blocking. Returns false if queue is full.
func (a *Aggregator) EnqueueBatch(values []string) bool {
	if len(values) == 0 {
		return true
	}

	select {
	case a.inBatch <- values:
		return true
	default:
		return false
	}
}

// RecordDrop adds to the external drop counter, to be included in the next snapshot.
func (a *Aggregator) RecordDrop(n uint64) { a.externalDropped.Add(n) }

// Start begins the aggregation loop.
func (a *Aggregator) Start(ctx context.Context) {
	go func() {
		defer close(a.done)

		ticker := time.NewTicker(a.window)
		defer ticker.Stop()

		windowStart := a.nowFn().UnixMilli()

		for {
			select {
			case <-ctx.Done():
				a.flush(windowStart, a.nowFn().UnixMilli())
				return
			case ev := <-a.in:
				a.total++
				a.counts[ev.Value]++
			case vals := <-a.inBatch:
				a.total += uint64(len(vals))
				for _, v := range vals {
					a.counts[v]++
				}
			case <-ticker.C:
				windowEnd := a.nowFn().UnixMilli()
				a.flush(windowStart, windowEnd)
				windowStart = windowEnd
			}
		}
	}()
}

// Stop requests the loop to stop and waits for completion.
func (a *Aggregator) Stop(ctx context.Context) {
	// Wait for the aggregator loop to finish; caller should cancel the context passed to Start.
	select {
	case <-a.done:
		return
	case <-ctx.Done():
		return
	}
}

func (a *Aggregator) flush(windowStart, windowEnd int64) {
	if len(a.counts) == 0 && a.total == 0 && a.externalDropped.Load() == 0 {
		return
	}
	// Snapshot counts
	snapshotCounts := make(map[string]uint64, len(a.counts))
	for k, v := range a.counts {
		snapshotCounts[k] = v
	}

	dropped := a.externalDropped.Swap(0)
	snap := sink.Snapshot{
		WindowStart:  windowStart,
		WindowEnd:    windowEnd,
		AttributeKey: a.attributeKey,
		Counts:       snapshotCounts,
		Total:        a.total,
		Dropped:      dropped,
	}

	if err := a.sink.Publish(context.Background(), snap); err != nil {
		a.logger.Error(
			"failed to publish snapshot",
			slog.String("err", err.Error()),
			slog.String("attribute_key", a.attributeKey),
			slog.Int64("window_start", windowStart),
			slog.Int64("window_end", windowEnd),
			slog.Any("total", a.total),
			slog.Any("dropped", dropped),
			slog.String("sink", fmt.Sprintf("%T", a.sink)),
		)

		if a.incrPublishFailed != nil {
			a.incrPublishFailed(1)
		}
		// Do not reset counts on failed publish to avoid losing data.
		// We'll attempt to publish combined data on the next flush.
		return
	}

	if a.incrFlushes != nil {
		a.incrFlushes(1)
	}
	// Reset on successful publish
	a.counts = make(map[string]uint64, 32)
	a.total = 0
}

// QueueLen returns the current queue length; can be observed for metrics.
func (a *Aggregator) QueueLen() int { return len(a.in) }
