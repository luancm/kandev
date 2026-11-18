package lifecycle

import (
	"context"
	"errors"
	"net"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	spritesutil "github.com/kandev/kandev/internal/sprites"
	"github.com/kandev/kandev/internal/task/models"
	sprites "github.com/superfly/sprites-go"
)

const spriteStateRunning = "running"

func (r *SpritesExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	name := getMetadataString(row.Metadata, MetadataKeySpriteName)
	createdAt := recordedErrandSpriteBirth(row.Metadata[MetadataKeySpriteCreatedAt])
	if name == "" || createdAt.IsZero() {
		return ErrErrandRecoveryRequiresReconnect
	}
	token, err := r.errandSpriteCredential(ctx, row)
	if err != nil {
		return err
	}
	if token == "" {
		return ErrErrandRecoveryRequiresReconnect
	}

	ctx, cancel := context.WithTimeout(ctx, spriteHealthTimeout)
	defer cancel()
	sprite, err := r.recoveryClient(token).GetSprite(ctx, name)
	if err != nil {
		return err
	}
	if !matchesRecordedErrandSprite(sprite, name, createdAt) {
		return ErrErrandRecoveryRequiresReconnect
	}
	if err := r.stopErrandInSprite(ctx, row.AgentExecutionID, sprite, owner); err != nil {
		return err
	}
	r.mu.Lock()
	proxy := r.proxies[row.AgentExecutionID]
	delete(r.proxies, row.AgentExecutionID)
	delete(r.tokens, row.AgentExecutionID)
	r.mu.Unlock()
	r.closeProxySession(proxy)
	return nil
}

func matchesRecordedErrandSprite(sprite *sprites.Sprite, name string, createdAt time.Time) bool {
	return sprite != nil && !createdAt.IsZero() && sprite.Name() == name && sprite.CreatedAt.Equal(createdAt) &&
		spritesutil.NormalizeSpriteStatus(sprite.Status) == spriteStateRunning
}

func (r *SpritesExecutor) errandSpriteCredential(ctx context.Context, row *models.ExecutorRunning) (string, error) {
	if tokenID := getMetadataString(row.Metadata, spritesAPITokenSecretMetadataKey); tokenID != "" {
		return r.revealErrandSpriteCredential(ctx, tokenID)
	}
	r.mu.RLock()
	token := r.tokens[row.AgentExecutionID]
	r.mu.RUnlock()
	if token != "" {
		return token, nil
	}
	profile, err := readOwnedSpriteProfile(ctx, r.executorProfileReader, getMetadataString(row.Metadata, MetadataKeyExecutorProfileID), row.ExecutorID)
	if err != nil {
		return "", err
	}
	var definitions []runtimeenv.Definition
	for _, definition := range executorProfileDefinitions(profile) {
		if definition.Key == spritesAPITokenEnvKey {
			definitions = append(definitions, definition)
		}
	}
	values, _, err := runtimeenv.Resolve(ctx, definitions, func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		return r.revealErrandSpriteCredential(ctx, definition.SecretID)
	})
	return values[spritesAPITokenEnvKey], err
}

func (r *SpritesExecutor) revealErrandSpriteCredential(ctx context.Context, id string) (string, error) {
	if r.secretStore == nil {
		return "", ErrErrandRecoveryRequiresReconnect
	}
	return revealGlobalSecret(ctx, r.secretStore, id)
}

func (r *SpritesExecutor) stopErrandInSprite(ctx context.Context, executionID string, sprite *sprites.Sprite, owner agentctltypes.ErrandMCPOwner) error {
	proxy, err := sprite.ProxyPort(ctx, 0, r.agentctlPort)
	if err != nil {
		return err
	}
	defer func() { _ = proxy.Close() }()
	control, err := r.spriteErrandControlClient(proxy.LocalAddr())
	if err != nil {
		return err
	}
	defer control.Close()
	return control.StopErrandInstance(ctx, executionID, owner)
}

func (r *SpritesExecutor) spriteErrandControlClient(proxyAddr net.Addr) (*agentctl.ControlClient, error) {
	addr, ok := proxyAddr.(*net.TCPAddr)
	if !ok || addr == nil || !addr.IP.IsLoopback() || addr.Port < 1 {
		return nil, errors.New("errand Sprite control proxy has no loopback address")
	}
	host := addr.IP.String()
	if addr.IP.To4() == nil {
		host = "[" + host + "]"
	}
	return agentctl.NewControlClient(host, addr.Port, r.logger), nil
}

func (r *SpritesExecutor) requireSpriteErrandPolicy(ctx context.Context, proxyAddr net.Addr, owner *agentctltypes.ErrandMCPOwner) error {
	if owner == nil {
		return nil
	}
	control, err := r.spriteErrandControlClient(proxyAddr)
	if err != nil {
		return err
	}
	defer control.Close()
	return requireErrandQuestionPolicy(ctx, control, owner)
}

func recordedErrandSpriteBirth(value any) time.Time {
	switch stamp := value.(type) {
	case time.Time:
		return stamp
	case string:
		birth, _ := time.Parse(time.RFC3339Nano, stamp)
		return birth
	default:
		return time.Time{}
	}
}
