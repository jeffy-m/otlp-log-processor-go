package aggregator

import (
	// Custom benchmark flags must be declared in a _test file before use.
	// Use -benchprocs to override GOMAXPROCS, or prefer the built-in -cpu flag.
	// Example: go test -bench=Aggregator -benchmem -benchprocs=4 ./internal/aggregator
	"context"
	"flag"
	"io"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"dash0.com/otlp-log-processor-backend/internal/sink"
)

type benchSink struct{}

func (benchSink) Publish(_ context.Context, _ sink.Snapshot) error { return nil }

var benchProcs = flag.Int("benchprocs", 0, "override GOMAXPROCS for aggregator benchmarks (0 = use testing -cpu)")

func newBenchAgg(maxQueue int) (*Aggregator, context.CancelFunc) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	a := New(1*time.Hour, "foo", benchSink{}, logger, maxQueue)
	ctx, cancel := context.WithCancel(context.Background())
	a.Start(ctx)

	return a, cancel
}

// Ensure the aggregator can keep up with a stream of single-value enqueues.
func BenchmarkAggregator_Enqueue(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 20) // large buffer to minimize drops

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for !a.Enqueue("v") {
			// spin until accepted to avoid drop influencing the benchmark
		}
	}
}

// Parallel producers enqueueing single values.
func BenchmarkAggregator_Enqueue_Parallel(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 20)

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for !a.Enqueue("v") {
				runtime.Gosched()
			}
		}
	})
}

// Benchmark batch enqueues with a moderate batch size.
func BenchmarkAggregator_EnqueueBatch_64(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 16)

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	batch := make([]string, 64)
	for i := range batch {
		batch[i] = "v"
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for !a.EnqueueBatch(batch) {
			// spin until accepted
		}
	}
}

// Parallel batch producers (64 per batch).
func BenchmarkAggregator_EnqueueBatch_64_Parallel(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 16)

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		batch := make([]string, 64)
		for i := range batch {
			batch[i] = "v"
		}

		for pb.Next() {
			for !a.EnqueueBatch(batch) {
				runtime.Gosched()
			}
		}
	})
}

// Benchmark batch enqueues with a larger batch size.
func BenchmarkAggregator_EnqueueBatch_1024(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 16)

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	batch := make([]string, 1024)
	for i := range batch {
		batch[i] = "v"
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for !a.EnqueueBatch(batch) {
			// spin until accepted
		}
	}
}

// Parallel batch producers (1024 per batch).
func BenchmarkAggregator_EnqueueBatch_1024_Parallel(b *testing.B) {
	if *benchProcs > 0 {
		runtime.GOMAXPROCS(*benchProcs)
	}

	a, cancel := newBenchAgg(1 << 16)

	b.Cleanup(func() { cancel(); a.Stop(context.Background()) })

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		batch := make([]string, 1024)
		for i := range batch {
			batch[i] = "v"
		}

		for pb.Next() {
			for !a.EnqueueBatch(batch) {
				runtime.Gosched()
			}
		}
	})
}
