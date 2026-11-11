package backendapp

import "context"

func (w *orchestratorWrapper) ValidateErrandContinuation(ctx context.Context, sessionID string) error {
	return w.svc.ValidateErrandContinuation(ctx, sessionID)
}
