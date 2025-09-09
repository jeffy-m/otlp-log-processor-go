package otlp

import (
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func kvStr(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

func kvInt(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}

func TestExtractAttrs_Precedence(t *testing.T) {
	key := "foo"
	logAttrs := []*commonpb.KeyValue{kvStr("foo", "log")}
	scopeAttrs := []*commonpb.KeyValue{kvStr("foo", "scope")}
	resAttrs := []*commonpb.KeyValue{kvStr("foo", "res")}

	got, ok := ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
	require.True(t, ok)
	require.Equal(t, "log", got)
}

func TestExtractAttrs_ScopeFallback(t *testing.T) {
	key := "foo"
	logAttrs := []*commonpb.KeyValue{}
	scopeAttrs := []*commonpb.KeyValue{kvStr("foo", "scope")}
	resAttrs := []*commonpb.KeyValue{kvStr("foo", "res")}

	got, ok := ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
	require.True(t, ok)
	require.Equal(t, "scope", got)
}

func TestExtractAttrs_ResourceFallback(t *testing.T) {
	key := "foo"
	logAttrs := []*commonpb.KeyValue{}
	scopeAttrs := []*commonpb.KeyValue{}
	resAttrs := []*commonpb.KeyValue{kvStr("foo", "res")}

	got, ok := ExtractAttrs(key, logAttrs, scopeAttrs, resAttrs)
	require.True(t, ok)
	require.Equal(t, "res", got)
}

func TestExtractAttrs_Missing(t *testing.T) {
	key := "foo"
	got, ok := ExtractAttrs(key, nil, nil, nil)
	require.False(t, ok)
	require.Equal(t, "", got)
}

func TestExtractAttrs_IntAndStringify(t *testing.T) {
	key := "foo"
	logAttrs := []*commonpb.KeyValue{kvInt("foo", 42)}
	got, ok := ExtractAttrs(key, logAttrs, nil, nil)
	require.True(t, ok)
	require.Equal(t, "42", got)
}

func TestAnyToString_Types(t *testing.T) {
	tests := []struct {
		name string
		val  *commonpb.AnyValue
		want string
	}{
		{
			name: "string",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "abc"}},
			want: "abc",
		},
		{
			name: "bool_true",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}},
			want: "true",
		},
		{
			name: "bool_false",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: false}},
			want: "false",
		},
		{
			name: "int",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 123}},
			want: "123",
		},
		{
			name: "double",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 3.14}},
			want: "3.14",
		},
		{
			name: "bytes",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: []byte{0xDE, 0xAD}}},
			want: "3q0=",
		},
		{
			name: "array_fallback",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{}}},
			want: "<unknown>",
		},
		{
			name: "object_fallback",
			val:  &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{kvStr("a", "b")}}}},
			want: "<unknown>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyToString(tt.val)
			require.Equal(t, tt.want, got)
		})
	}
}
