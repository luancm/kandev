# Successful errand cleanup regression handoff

## Packet

Added focused orchestration regressions against the existing SQLite settlement fixture and mocked `StopAgentWithReason` boundary. The tests cover both `agent.ready` and process-exit completion, capture the durable errand row inside the stop callback before it signals entry, and require the retained session to become `COMPLETED` only after stop acknowledgement. Additional cases preserve an admitted successor and preserve an already committed cancellation.

I changed only the new test file and this handoff. I did not change production code, existing tests, fixtures, specifications, manifests, or work orders. The synchronization uses Go 1.26's `testing/synctest`; no production-only test seam is required.

## Review findings and disposition

- HIGH 1, result read after stop entry: fixed. The stop callback reads the exact durable errand row before sending its entry observation, and the assertion uses that captured row rather than a later database read.
- HIGH 2, one pre-release session snapshot: fixed. With the stop callback still blocked, `synctest.Wait` drains every runnable goroutine in the test bubble before the test reads session state. The stop gate cannot be released until after that observation. A second `synctest.Wait` follows stop return before the completed-state assertion.
- HIGH 3, successor/cancellation checks could miss detached cleanup: fixed. The control cases run in a `synctest` bubble and call `synctest.Wait` after successor admission or process-exit handling, respectively, before asserting that no stop occurred or cancellation was preserved. This gives all runnable cleanup owners a deterministic scheduling boundary; the tests contain no sleeps.

All goroutines started by these test paths are created inside the `synctest` bubble. `synctest.Wait` returns after runnable goroutines finish or become durably blocked; it does not advance timers while waiting, and `synctest.Test` joins every bubble goroutine before each test returns. No production synchronization seam was needed.

## Focused RED evidence

Command, run from `apps/backend` while holding the shared build lock:

```bash
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run 'TestErrandCompletionCleanup' -count=1 -v
```

The finalized file compiled and the race-enabled selector exited 1 on exactly the two intended behavioral assertions. The ready case settled `settlement-A` and retained `canonical result A`, but observed no stop of `settlement-execution`; its session remained `WAITING_FOR_INPUT`. The process-exit case captured the exact successful result before stop entry and observed the stop return with a non-cancellation reason, but the retained session remained `WAITING_FOR_INPUT` instead of becoming `COMPLETED`. `TestErrandCompletionCleanupDoesNotStopAnAdmittedSuccessor` and `TestErrandCompletionCleanupDoesNotReplaceCancellationOnProcessExit` both passed after their synchronization barriers. No race report occurred.

The same selector passed `TestErrandCompletionCleanupDoesNotStopAnAdmittedSuccessor` and `TestErrandCompletionCleanupDoesNotReplaceCancellationOnProcessExit`. The successor case admits the queued turn before replaying the older completion event and confirms that the old execution is not stopped. The cancellation case confirms that a late process-exit success event leaves the durable cancellation row unchanged.

## Existing seams

- The ready path enters `settleDetachedAgentReadyTurn` in [event_handlers_agent.go](../../../apps/backend/internal/orchestrator/event_handlers_agent.go), commits the exact result through `settleErrandTurn` in [errand_settlement.go](../../../apps/backend/internal/orchestrator/errand_settlement.go), then unconditionally publishes `WAITING_FOR_INPUT`. It does not create a durable success-close obligation or stop the runtime.
- The process-exit path settles through `evaluateCompletedAgentWorkflow` and then reaches `finishAgentCompletedTurn` in [event_handlers_agent.go](../../../apps/backend/internal/orchestrator/event_handlers_agent.go). That path sets an errand to `WAITING_FOR_INPUT` and schedules generic `cleanupAgentExecution`; it has no success-specific durable receipt or `COMPLETED` projection after the stop.
- Existing [ErrandTeardownIntent](../../../apps/backend/internal/task/models/errand_teardown.go) validation requires a cancelled errand with a cancellation source. [errand_teardown.go](../../../apps/backend/internal/orchestrator/errand_teardown.go) applies cancellation state, cancellation reason, and failed-execution effects. Reuse its exact target and admission arbitration, but give successful completion distinct durable purpose and effects; preserve missing-purpose compatibility for stored cancellation intents.
- SQLite currently rejects replacing an already completed teardown intent. Repeated `complete → continue → complete` therefore needs conditional replacement without losing a still-pending physical-stop obligation, and cancellation must overtake pending successful cleanup safely.
- A nil `OnErrandTurnSettled` error is not sufficient cleanup authority: the service can commit a historical receipt without changing the current errand row. `LastOutcome` is also insufficient because a user-origin success can preserve an earlier parent-origin outcome. Authorize close from the exact current successful terminal decision and unchanged session/turn ownership; test historical success beside a newer unresolved turn and opposite parent/user outcomes.
- Successful cleanup must not qualify for the existing idle-focus passive-resume path. Only explicit, validated user or authorized parent continuation should resume the retained conversation.
- The real SQLite result capture is exercised by `newQueueSettlementFixture` and `settlementTranscript` in [errand_queue_settlement_test.go](../../../apps/backend/internal/orchestrator/errand_queue_settlement_test.go). The test stop gate reads the durable errand row before publishing callback entry, then blocks at the existing executor boundary. `testing/synctest` provides a deterministic quiescence boundary while that stop remains blocked and after detached control-path work is triggered.

## Safety coverage and remaining work

Existing coverage remains useful and was not duplicated: `TestErrandReadyCapturesResultBeforeQueuedUserTurn`, `TestErrandReadyQueuedParentTurnSupersedesCapturedResult`, and `TestErrandReadyStaleTerminalPreservesSuccessorIdentity` protect result and successor identity; `TestErrandReadyCancelledChildKeepsNoticeAndPausedQueue` protects ready-after-cancellation behavior; and `TestErrandAgentCompletedCapturesResultBeforeQueuedSuccessor` covers process-exit result capture.

Production cleanup remains sequenced after recovery integration. Keep this packet bounded to the ready/process-exit close contract; the next RED matrix should prioritize repeated complete/continue/complete, cancellation overtaking pending success, historical success beside a newer unresolved turn, and opposite user/parent outcomes. The broader packet must also cover durable stop or receipt failure and retry, restart and flag-off recovery, same-execution successor and replacement-incarnation barriers, legacy absence evidence, user-origin no-result settlement, explicit user/parent continuation, ordinary-session controls, and desktop/phone history and continuation.

## Changed paths

- [errand_completion_cleanup_test.go](../../../apps/backend/internal/orchestrator/errand_completion_cleanup_test.go)
- [regression-handoff.md](regression-handoff.md)

The race-test process exited and released the shared build lock. Go removed its test build scratch at process exit; this packet created no separate scratch files. No runtime, database, deployment, staging, commit, or push operation was performed.
