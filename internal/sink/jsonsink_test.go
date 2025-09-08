package sink

import (
    "bytes"
    "context"
    "encoding/json"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestJSONSink_Publish_EncodesSnapshot(t *testing.T) {
    buf := new(bytes.Buffer)
    s := NewJSONSink(buf)

    snap := Snapshot{
        WindowStart:  1000,
        WindowEnd:    2000,
        AttributeKey: "foo",
        Counts: map[string]uint64{
            "bar": 1,
            "baz": 2,
            "unknown": 3,
        },
        Total:   6,
        Dropped: 1,
    }

    require.NoError(t, s.Publish(context.Background(), snap))

    // Ensure we got exactly one JSON line (Encoder.Encode adds a newline)
    out := buf.String()
    require.True(t, strings.HasSuffix(out, "\n"), "expected trailing newline, got: %q", out)

    // Decode back and compare fields to avoid JSON key order issues.
    var got Snapshot
    require.NoErrorf(t, json.Unmarshal(buf.Bytes(), &got), "data=%q", out)

    require.EqualValues(t, snap.WindowStart, got.WindowStart)
    require.EqualValues(t, snap.WindowEnd, got.WindowEnd)
    require.Equal(t, snap.AttributeKey, got.AttributeKey)
    require.EqualValues(t, snap.Total, got.Total)
    require.EqualValues(t, snap.Dropped, got.Dropped)
    require.Len(t, got.Counts, len(snap.Counts))
    require.EqualValues(t, 1, got.Counts["bar"]) 
    require.EqualValues(t, 2, got.Counts["baz"]) 
    require.EqualValues(t, 3, got.Counts["unknown"]) 
}

