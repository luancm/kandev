package lifecycle

import (
	"context"
	"errors"
	"net"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	spritesutil "github.com/kandev/kandev/internal/sprites"
	"github.com/kandev/kandev/internal/task/models"
	sprites "github.com/superfly/sprites-go"
)

const spriteStateRunning = "running"

func (r *SpritesExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	name := getMetadataString(row.Metadata, MetadataKeySpriteName)
	createdAt := recordedErrandSpriteBirth(row.Metadata[MetadataKeySpriteCreatedAt])
	tokenID := getMetadataString(row.Metadata, "env_secret_id_SPRITES_API_TOKEN")
	if name == "" || createdAt.IsZero() || tokenID == "" || r.secretStore == nil {
		return ErrErrandRecoveryRequiresReconnect
	}
	token, err := revealGlobalSecret(ctx, r.secretStore, tokenID)
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("errand stop requires the Sprite credential")
	}

	ctx, cancel := context.WithTimeout(ctx, spriteHealthTimeout)
	defer cancel()
	sprite, err := r.recoveryClient(token).GetSprite(ctx, name)
	if err != nil {
		return err
	}
	if sprite == nil || sprite.Name() != name || !sprite.CreatedAt.Equal(createdAt) ||
		spritesutil.NormalizeSpriteStatus(sprite.Status) != spriteStateRunning {
		return ErrErrandRecoveryRequiresReconnect
	}
	return r.stopErrandInSprite(ctx, row.AgentExecutionID, sprite, owner)
}

func (r *SpritesExecutor) stopErrandInSprite(ctx context.Context, executionID string, sprite *sprites.Sprite, owner agentctltypes.ErrandMCPOwner) error {
	proxy, err := sprite.ProxyPort(ctx, 0, r.agentctlPort)
	if err != nil {
		return err
	}
	defer func() { _ = proxy.Close() }()
	addr, ok := proxy.LocalAddr().(*net.TCPAddr)
	if !ok || addr == nil || !addr.IP.IsLoopback() || addr.Port < 1 {
		return errors.New("errand Sprite control proxy has no loopback address")
	}
	host := addr.IP.String()
	if addr.IP.To4() == nil {
		host = "[" + host + "]"
	}
	control := agentctl.NewControlClient(host, addr.Port, r.logger)
	defer control.Close()
	return control.StopErrandInstance(ctx, executionID, owner)
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
