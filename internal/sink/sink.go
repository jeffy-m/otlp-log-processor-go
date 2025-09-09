package sink

import "context"

//go:generate mockgen -source=sink.go -destination=./mocks/mock_sink.go -package=mocks

// Snapshot describes the data emitted at the end of a window.
type Snapshot struct {
	WindowStart  int64             `json:"window_start"`
	WindowEnd    int64             `json:"window_end"`
	AttributeKey string            `json:"attribute_key"`
	Counts       map[string]uint64 `json:"counts"`
	Total        uint64            `json:"total"`
	Dropped      uint64            `json:"dropped"`
}

// Sink publishes per-window snapshots. A JSON stdout implementation can be added later.
type Sink interface {
	Publish(ctx context.Context, s Snapshot) error
}
