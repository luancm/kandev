package sqlite

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandMCPSnapshotReadsLegacyArraysAndExclusionEnvelopes(t *testing.T) {
	for _, encoded := range []string{`["added"]`, ` {"server_ids":["added"],"excluded_server_ids":["blocked"]}`} {
		var row models.Errand
		if err := decodeErrandMCPSnapshot(encoded, &row); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(row.MCPServerIDs, []string{"added"}) {
			t.Fatalf("decoded additions = %v", row.MCPServerIDs)
		}
		if encoded[0] == ' ' && !reflect.DeepEqual(row.MCPExcludedServerIDs, []string{"blocked"}) {
			t.Fatalf("decoded exclusions = %v", row.MCPExcludedServerIDs)
		}
		stored, err := encodeErrandMCPSnapshot(&row)
		if err != nil {
			t.Fatal(err)
		}
		var restored models.Errand
		if err := decodeErrandMCPSnapshot(stored, &restored); err != nil || !reflect.DeepEqual(row, restored) {
			t.Fatalf("snapshot round trip = %+v, %v", restored, err)
		}
	}
}
