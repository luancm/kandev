package move

import "strings"

// NormalizeEntryOptions trims all string fields, folds the legacy top-level
// prompt into instructions, and returns nil for an empty options object.
//
// The legacy prompt is accepted when the nested instructions field is absent,
// and is also accepted when both values are equal after trimming. Different
// non-blank values are rejected so a caller cannot silently lose its intent.
func NormalizeEntryOptions(options *EntryOptions, legacyPrompt string) (*EntryOptions, error) {
	legacyPrompt = strings.TrimSpace(legacyPrompt)
	normalized := normalizedCopy(options)
	if normalized == nil {
		normalized = &EntryOptions{}
	}

	if normalized.Instructions != "" && legacyPrompt != "" && normalized.Instructions != legacyPrompt {
		return nil, &ConflictingInstructionsError{}
	}
	if normalized.Instructions == "" {
		normalized.Instructions = legacyPrompt
	}
	if *normalized == (EntryOptions{}) {
		return nil, nil
	}
	return normalized, nil
}

// ValidateEntryOptions rejects agent-facing options for a move that does not
// actually change workflow step. Empty options are always a no-op and remain
// valid for position-only or no-step moves.
func ValidateEntryOptions(options *EntryOptions, change MoveChange) error {
	if normalizedCopy(options) == nil {
		return nil
	}
	if change != MoveChangeStep {
		return &EntryOptionsNotAllowedError{Change: change}
	}
	return nil
}
