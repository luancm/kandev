package models

import (
	"encoding/json"
	"math"
)

var sessionContextWindowScalarFields = [...]string{
	"size", "used", "remaining", "efficiency", "source", "timestamp",
}

// ProjectSessionContextMetadata returns only the persisted context fields used
// by compact task-session summaries. A present null or invalid window remains
// present so it can invalidate an older client value; an absent key stays absent.
func ProjectSessionContextMetadata(metadata map[string]interface{}) map[string]interface{} {
	projected := make(map[string]interface{})
	if contextWindow, exists := metadata[SessionMetaKeyContextWindow]; exists {
		projected[SessionMetaKeyContextWindow] = projectSessionContextWindow(contextWindow)
	}
	if count, exists := metadata[SessionMetaKeyContextCompactionCount]; exists && isJSONNumber(count) {
		projected[SessionMetaKeyContextCompactionCount] = count
	}
	return projected
}

func projectSessionContextWindow(contextWindow interface{}) interface{} {
	if contextWindow == nil {
		return nil
	}
	fields, ok := contextWindow.(map[string]interface{})
	if !ok {
		return nil
	}
	projected := make(map[string]interface{}, len(sessionContextWindowScalarFields))
	for _, key := range sessionContextWindowScalarFields {
		value, exists := fields[key]
		if !exists {
			continue
		}
		if key == "source" || key == "timestamp" {
			if _, ok := value.(string); ok {
				projected[key] = value
			}
			continue
		}
		if isJSONNumber(value) {
			projected[key] = value
		}
	}
	return projected
}

func isJSONNumber(value interface{}) bool {
	switch number := value.(type) {
	case json.Number:
		_, err := number.Float64()
		return err == nil
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0)
	case float32:
		converted := float64(number)
		return !math.IsNaN(converted) && !math.IsInf(converted, 0)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}
