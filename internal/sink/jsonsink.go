package sink

import (
    "context"
    "encoding/json"
    "io"
    "os"
)

// JSONSink writes snapshots as single-line JSON to an io.Writer.
type JSONSink struct {
    w io.Writer
}

// NewJSONSink creates a JSON sink writing to the provided writer.
func NewJSONSink(w io.Writer) *JSONSink { return &JSONSink{w: w} }

// NewStdoutJSON returns a JSON sink that writes to os.Stdout.
func NewStdoutJSON() *JSONSink { return &JSONSink{w: os.Stdout} }

// Publish marshals the snapshot as JSON and writes it with a trailing newline.
func (s *JSONSink) Publish(_ context.Context, snap Snapshot) error {
    enc := json.NewEncoder(s.w)
    // Keep compact output; callers can wrap the writer if they want pretty printing.
    return enc.Encode(snap)
}

