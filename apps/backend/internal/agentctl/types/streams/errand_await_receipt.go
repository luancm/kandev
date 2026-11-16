package streams

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

const awaitReceiptText = "text"

const ErrandAwaitToolName = "kandev/await_errands_kandev"

// ErrandAwaitReceipt is issued only by supported native result adapters. It is
// correlation evidence; authenticated inbox admission supplies its owner.
type ErrandAwaitReceipt struct {
	Version int                      `json:"version"`
	Token   string                   `json:"token"`
	Items   []ErrandAwaitReceiptItem `json:"items"`
}

type ErrandAwaitReceiptItem struct {
	ErrandID       string `json:"errand_id"`
	ResultRevision string `json:"result_revision"`
}

func receiptIdentity(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func (r *ErrandAwaitReceipt) Valid() bool {
	if r == nil || r.Version != 1 || !receiptIdentity(r.Token) || r.Items == nil || len(r.Items) > 5 {
		return false
	}
	seen := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if !receiptIdentity(item.ErrandID) || !receiptIdentity(item.ResultRevision) || seen[item.ErrandID] {
			return false
		}
		seen[item.ErrandID] = true
	}
	return true
}

func (r *ErrandAwaitReceipt) Snapshot() *ErrandAwaitReceipt {
	if !r.Valid() {
		return nil
	}
	copy := *r
	copy.Items = slices.Clone(r.Items)
	return &copy
}

// ParseErrandAwaitReceipt inspects one complete MCP text body, never previews
// or arbitrary nested output. Provider error envelopes must be checked first.
func ParseErrandAwaitReceipt(result any) *ErrandAwaitReceipt {
	m, ok := result.(map[string]any)
	if !ok || m["error"] != nil {
		return nil
	}
	if value, exists := m["isError"]; exists && value != false {
		return nil
	}
	content, ok := m["content"].([]any)
	if !ok || len(content) != 1 {
		return nil
	}
	block, ok := content[0].(map[string]any)
	if !ok || block["type"] != awaitReceiptText {
		return nil
	}
	body, ok := block[awaitReceiptText].(string)
	if !ok {
		return nil
	}
	object, err := uniqueReceiptObject([]byte(body))
	if err != nil {
		return nil
	}
	receipt := decodeAwaitReceipt(object["await_delivery"])
	if receipt == nil {
		return nil
	}
	if !matchingStructuredAwaitReceipt(m["structuredContent"], receipt) {
		return nil
	}
	return receipt
}

func matchingStructuredAwaitReceipt(structured any, receipt *ErrandAwaitReceipt) bool {
	if structured == nil {
		return true
	}
	data, err := json.Marshal(structured)
	if err != nil {
		return false
	}
	other, err := uniqueReceiptObject(data)
	if err != nil {
		return false
	}
	parsed := decodeAwaitReceipt(other["await_delivery"])
	return parsed != nil && parsed.Token == receipt.Token && slices.Equal(parsed.Items, receipt.Items)
}

func decodeAwaitReceipt(data []byte) *ErrandAwaitReceipt {
	object, err := uniqueReceiptObject(data)
	if err != nil || len(object) != 3 {
		return nil
	}
	var receipt ErrandAwaitReceipt
	if json.Unmarshal(object["version"], &receipt.Version) != nil || json.Unmarshal(object["token"], &receipt.Token) != nil {
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(object["items"], &items) != nil || items == nil || len(items) > 5 {
		return nil
	}
	receipt.Items = make([]ErrandAwaitReceiptItem, 0, len(items))
	for _, raw := range items {
		fields, err := uniqueReceiptObject(raw)
		if err != nil || len(fields) != 2 {
			return nil
		}
		var item ErrandAwaitReceiptItem
		if json.Unmarshal(fields["errand_id"], &item.ErrandID) != nil || json.Unmarshal(fields["result_revision"], &item.ResultRevision) != nil {
			return nil
		}
		receipt.Items = append(receipt.Items, item)
	}
	if !receipt.Valid() {
		return nil
	}
	return &receipt
}

func uniqueReceiptObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("expected receipt object")
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok || values[name] != nil {
			return nil, errors.New("duplicate receipt field")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing receipt data")
	}
	return values, nil
}
