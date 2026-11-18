package lifecycle

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	spritesAPITokenEnvKey            = "SPRITES_API_TOKEN"
	spritesAPITokenSecretMetadataKey = "env_secret_id_" + spritesAPITokenEnvKey
)

func (m *Manager) prepareOwnedSpriteEnvironment(ctx context.Context, req *LaunchRequest) error {
	if req.Owner.Kind != ExecutionOwnerErrand || req.ExecutorType != string(models.ExecutorTypeSprites) {
		return nil
	}
	profileID := getMetadataString(req.Metadata, MetadataKeyExecutorProfileID)
	executorID := getMetadataString(req.Metadata, "executor_id")
	if m.executorProfileReader == nil || profileID == "" || executorID == "" {
		return fmt.Errorf("%w: sprite errand has no saved executor profile", models.ErrWorkspaceReuseUnsafe)
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("read sprite errand executor profile: %w", err)
	}
	if profile == nil || profile.ID != profileID || profile.ExecutorID != executorID {
		return fmt.Errorf("%w: sprite errand executor profile identity changed", models.ErrWorkspaceReuseUnsafe)
	}
	definitions := executorProfileDefinitions(profile)
	if !hasEnvironmentDefinition(definitions, spritesAPITokenEnvKey) {
		return fmt.Errorf("%w: sprite errand executor profile has no API credential", models.ErrWorkspaceReuseUnsafe)
	}
	req.EnvironmentDefinitions = append(slices.Clone(req.EnvironmentDefinitions), definitions...)
	req.ApprovedSecretEnvKeys = approvedSecretEnvironmentKeys(req.EnvironmentDefinitions)
	req.EnvironmentResolutionRequired = true
	req.Metadata = maps.Clone(req.Metadata)
	delete(req.Metadata, spritesAPITokenSecretMetadataKey)
	for _, definition := range definitions {
		if definition.Key == spritesAPITokenEnvKey && definition.SecretID != "" {
			req.Metadata[spritesAPITokenSecretMetadataKey] = definition.SecretID
		}
	}
	return nil
}
