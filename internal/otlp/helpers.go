package otlp

import (
	"encoding/base64"
	"fmt"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// ExtractAttrs finds the attribute value by precedence: logAttrs > scopeAttrs > resourceAttrs.
// Returns the canonical string representation and true if the key was found.
func ExtractAttrs(key string, logAttrs, scopeAttrs, resourceAttrs []*commonpb.KeyValue) (string, bool) {
	if v, ok := findInKVs(key, logAttrs); ok {
		return v, true
	}

	if v, ok := findInKVs(key, scopeAttrs); ok {
		return v, true
	}

	if v, ok := findInKVs(key, resourceAttrs); ok {
		return v, true
	}

	return "", false
}

func findInKVs(key string, kvs []*commonpb.KeyValue) (string, bool) {
	for _, kv := range kvs {
		if kv.GetKey() == key {
			if kv.GetValue() == nil {
				return "", false
			}

			return anyToString(kv.GetValue()), true
		}
	}

	return "", false
}

func anyToString(v *commonpb.AnyValue) string {
	switch x := v.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case *commonpb.AnyValue_BoolValue:
		if x.BoolValue {
			return "true"
		}

		return "false"
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", x.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", x.DoubleValue)
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(x.BytesValue)
	default:
		return "<unknown>"
	}
}
