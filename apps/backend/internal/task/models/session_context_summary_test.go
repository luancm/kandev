package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProjectSessionContextMetadata(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "missing stays absent",
			in:   map[string]interface{}{"unrelated": "value"},
			want: map[string]interface{}{},
		},
		{
			name: "explicit null stays authoritative",
			in:   map[string]interface{}{SessionMetaKeyContextWindow: nil},
			want: map[string]interface{}{SessionMetaKeyContextWindow: nil},
		},
		{
			name: "measured full context keeps only fixed scalars",
			in: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{
					"size": 100, "used": 100, "remaining": 0, "efficiency": 100,
					"source": "acp", "timestamp": "2026-10-30T10:00:00Z",
					"provider_catalogue": map[string]interface{}{"secret": "excluded"},
				},
				SessionMetaKeyContextCompactionCount: json.Number("2"),
				"unrelated":                          "excluded",
			},
			want: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{
					"size": 100, "used": 100, "remaining": 0, "efficiency": 100,
					"source": "acp", "timestamp": "2026-10-30T10:00:00Z",
				},
				SessionMetaKeyContextCompactionCount: json.Number("2"),
			},
		},
		{
			name: "unmeasured context preserves zero",
			in: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{"size": int64(100), "used": float64(0)},
			},
			want: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{"size": int64(100), "used": float64(0)},
			},
		},
		{
			name: "invalid shapes become unavailable",
			in: map[string]interface{}{
				SessionMetaKeyContextWindow:          []interface{}{"invalid"},
				SessionMetaKeyContextCompactionCount: "not a number",
			},
			want: map[string]interface{}{SessionMetaKeyContextWindow: nil},
		},
		{
			name: "invalid scalar fields are omitted",
			in: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{
					"size": "100", "used": 10, "source": []interface{}{"invalid"},
				},
			},
			want: map[string]interface{}{
				SessionMetaKeyContextWindow: map[string]interface{}{"used": 10},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ProjectSessionContextMetadata(test.in)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ProjectSessionContextMetadata() = %#v, want %#v", got, test.want)
			}
		})
	}
}
