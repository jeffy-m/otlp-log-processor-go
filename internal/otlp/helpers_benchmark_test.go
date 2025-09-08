package otlp

import (
    "testing"

    commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func benchKVStr(k, v string) *commonpb.KeyValue {
    return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

func BenchmarkExtractAttrs_LogHit(b *testing.B) {
    key := "foo"
    logAttrs := []*commonpb.KeyValue{benchKVStr(key, "logv")}
    scopeAttrs := []*commonpb.KeyValue{benchKVStr(key, "scopev")}
    resAttrs := []*commonpb.KeyValue{benchKVStr(key, "resv")}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
    }
}

func BenchmarkExtractAttrs_ScopeFallback(b *testing.B) {
    key := "foo"
    var logAttrs []*commonpb.KeyValue
    scopeAttrs := []*commonpb.KeyValue{benchKVStr(key, "scopev")}
    resAttrs := []*commonpb.KeyValue{benchKVStr(key, "resv")}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
    }
}

func BenchmarkExtractAttrs_ResourceFallback(b *testing.B) {
    key := "foo"
    var logAttrs []*commonpb.KeyValue
    var scopeAttrs []*commonpb.KeyValue
    resAttrs := []*commonpb.KeyValue{benchKVStr(key, "resv")}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
    }
}

func BenchmarkExtractAttrs_Missing(b *testing.B) {
    key := "foo"
    var logAttrs []*commonpb.KeyValue
    var scopeAttrs []*commonpb.KeyValue
    var resAttrs []*commonpb.KeyValue
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
    }
}

