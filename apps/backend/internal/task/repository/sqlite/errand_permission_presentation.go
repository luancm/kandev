package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// UpdateErrandAutomaticPermissionPresentation changes only an unclaimed
// automatic proposal on the current owned execution.
func (r *Repository) UpdateErrandAutomaticPermissionPresentation(ctx context.Context, source models.ErrandPermissionSource, optionID, optionKind string, presentation models.PermissionPresentation) (bool, error) {
	if presentation != models.PermissionPresentationInteractive && presentation != models.PermissionPresentationUnavailable {
		return false, fmt.Errorf("invalid errand permission presentation %q", presentation)
	}
	tx, message, err := r.beginErrandPermissionResolution(ctx, source, true)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	_, eligible, err := errandPermissionClaimEligibility(message, true)
	if err != nil || !eligible || message.Metadata[models.PermissionPresentationMetadataKey] != string(models.PermissionPresentationAutomatic) {
		return false, err
	}
	if !matchesErrandAutomaticPermissionProposal(message, models.PermissionResolutionAudit{
		Source: models.PermissionSourceAutoApprove, OptionID: optionID, OptionKind: optionKind,
	}) {
		return false, nil
	}
	message.Metadata[models.PermissionPresentationMetadataKey] = string(presentation)
	message.RequestsInput = presentation == models.PermissionPresentationInteractive
	if !message.RequestsInput {
		message.Metadata["status"] = string(models.PermissionStatusExpired)
	}
	if err = r.writeErrandPermissionTx(ctx, tx, source.Owner.ErrandID, message); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
