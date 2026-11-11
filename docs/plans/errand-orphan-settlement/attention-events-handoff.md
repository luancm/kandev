# Errand recovery attention event handoff

Scope: the confirmed AC-TASKS-ERRANDS-002.6 recovery-observer gap from the attention checkpoint review. This packet changes only recovery publication, its focused orchestrator tests, and this handoff.

## Behavior

After a recovery metadata write succeeds and the current persisted session reload succeeds, `publishAgentDeliveryRecoveryState` publishes the session projection and calls the existing Errand session-state observer. The observer publishes the current Errand snapshot and wakes in-flight parent awaits even when the orchestrator session event bus is nil.

Recovery clears use the same publication path only when the repository reports that the exact identity matched and the clear persisted. A stale or already-settled no-op clear does not emit a duplicate Errand update or wake. Persistence failures and post-write session reload failures do not reach the observer; ordinary sessions and rejected stale recovery events do not publish an Errand update or wake a join.

No status classifier, SQL evidence predicate, receipt writer, settlement authority, continuation gate, cleanup behavior, frontend, or runtime state transition changed. The change only projects already-persisted recovery metadata; it does not synthesize terminal outcomes or replay work.

## Verification

The initial RED test failed because a same-session-state `reconnecting` → `uncertain` recovery write produced no Errand attention event. A separate no-op-clear RED check showed an observer projection read despite the repository rejecting the stale identity.

The focused race command passed all six cases: same-state recovery attention and parent-join wake, persistence and reload failures, ordinary-session isolation, stale-event rejection, successful clear notification, and no-op clear suppression.

```sh
flock -w 900 /tmp/kandev-errands-repair-build.lock env GOTMPDIR=/tmp/kandev-errand-attention-events GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run '^(TestDeliveryRecoveryPhaseChangePublishesAttentionAndWakesAwait|TestRecoveryPublicationFailureDoesNotNotifyOrWakeErrandAwait|TestOrdinarySessionRecoveryDoesNotNotifyOrWakeErrandAwait|TestRejectedStaleRecoveryDoesNotNotifyOrWakeErrandAwait|TestClearedRecoveryNoticePublishesAttentionAndWakesAwait|TestNoOpRecoveryClearDoesNotNotifyOrWakeErrandAwait)$' -count=1 -v
```

The initial recovery-event RED run was handle `46713` and failed with no Errand event; its GREEN run was `92613`. The four-case focused run was `36777`. The no-op-clear RED run was `77902` and failed because projection reads increased from one to two. The six-case race run was `21229` and passed.

`gofmt -d` and `git diff --check` passed for the changed Go files. The changed-file lint helper first reported three `staticcheck` QF1008 findings and timed out (handle `15763`); the three redundant `.Repository` selectors were removed. Its retry reported zero issues but again hit the helper's fixed five-minute timeout (handle `37656`). The same package/new-code lint then passed with zero issues using a ten-minute limit:

```sh
flock -w 900 /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS='-trimpath -p=1' golangci-lint run ./internal/orchestrator --new-from-rev=52309e613fc9a76d38a9f891b74b4d8b32bff265 --timeout=10m --concurrency=2
```

That final command completed synchronously with exit code zero and `0 issues`. The race run preceded only the redundant-selector cleanup; the extended final lint typechecked the resulting Go files.

## Write set

```text
apps/backend/internal/orchestrator/agent_delivery_recovery_state.go
apps/backend/internal/orchestrator/agent_delivery_recovery_errand_test.go
docs/plans/errand-orphan-settlement/attention-events-handoff.md
```

The work is uncommitted in the shared fix worktree. No branch operation, commit, push, live runtime mutation, or unrelated file edit was performed by this packet.
