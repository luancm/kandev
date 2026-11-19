---
id: "12-lifecycle-convergence"
title: "Settle cancellation and converge retained state"
status: done
wave: 11
depends_on:
  - "11-notification-dispatch"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-012
  - REQ-TASKS-ERRANDS-013
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-002.2
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.2
  - AC-TASKS-ERRANDS-007.4
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-008.3
  - AC-TASKS-ERRANDS-012.2
  - AC-TASKS-ERRANDS-012.3
  - AC-TASKS-ERRANDS-013.1
  - AC-TASKS-ERRANDS-013.2
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 12: Settle cancellation and converge retained state

## Summary

Ensure turn cancellation, session deletion, recovery, and flag-off operation
leave consistent durable and visible state. Existing errands remain isolated;
neither a stuck outstanding bit nor stale client projections should prevent
the parent from continuing its work.

## In scope

- Settle a cancelled parent-initiated errand turn as an interrupted failure
  and clear its `awaiting_result`. A user-origin turn restores its prior
  settled outcome and produces no implicit result. Guard by the observed
  turn/submission so a stale callback cannot settle a successor.
- Route both parent `CancelTurn` and generic `CancelAgent` through the
  outstanding guard. End awaits without claims; keep children running,
  defer workflow, and do not wake a notification because of the cancel.
- Prevent flag-off deletion from admitting prepared cancellation output.
  Retained session deletion still works and retained roles remain detached.
  With the flag on, preserve the task 10 bounded cancellation snapshot.
- Authorize cancellation of a queued reserved errand ID through its owning
  task when no session row exists. Enforce the same workspace membership and
  cross-task denials as ordinary session actions; never bypass authorization.
- Project `waiting_on_errands` on task-service cancel/recovery events. Route
  existing `session.removed` events to authorized workspace viewers as well as
  transcript subscribers; parent-only browsers otherwise miss the removal.
  Consume that event through `removeTaskSession`. The ordered client's
  synthetic event can contain only `session_id`: resolve its task from store
  state without clearing another task. Test local deletion, a second
  parent-only client, reconnect, and session-only payloads. Fence an older
  in-flight `errand.list` response from resurrecting the deleted row, and
  choose a deterministic surviving session if the removed one was selected.
- Preserve upstream `nativeResumePendingTerminal` and
  `VerifyPendingNativeResumeTerminal` evidence when reconciling interrupted
  errand turns: authenticated peer/history, SQL effect/generation proof and
  persisted terminal markers must settle before classifying abandonment.
- Emit the missing structured reconciliation/delivery logs after durable
  changes and assert all three counter label sets, including delivery routes.

## Out of scope

- A new event family, a phone per-row delete menu, and presentation changes.
- New runtime-flag identities, changed shipped defaults, and generic auth
  redesign. Session isolation stays unconditional when Errands is disabled.

## Acceptance

1. Parent-origin and user-origin child-turn cancellations settle correctly;
   parent-turn cancellation preserves live children and defers workflow, while
   stopping/deleting the parent still cascades. Stale callbacks are harmless.
2. Disabled retained-session deletion creates no result queue entry or prompt;
   enabled deletion preserves its notice. Queued cancellation succeeds for an
   authorized user and fails for a foreign workspace before mutation.
3. DTOs, live clients, and reload converge on waiting/deletion state; logs
   describe committed transitions and metrics contain only closed labels.

## Verification

Run from root. Use TDD for the concrete cases above, including enforced-auth
gateway dispatch rather than invoking only the already-authorized handler.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|CancelTurn|CancelAgent|CancelledTurn|CompletionFollowUp' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service ./internal/task/dto ./internal/task/handlers ./internal/gateway/websocket -run 'Errand|WaitingOnErrands|Scope|Authorize' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/backendapp ./internal/errands -run 'NativeResumePendingTerminal|RuntimeTurnChangeSummaryProcessing|Reconcile|Restart' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/statussummary ./internal/runtimeflags ./internal/profiles -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/errands lib/state/slices/session/remove-task-session.test.ts lib/ws/handlers/agent-session.test.ts lib/ws/handlers/errands.test.ts lib/ws/ordered-session-events.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Run changed-Go/TypeScript lint on the owned files. This work order normalizes
data within existing surfaces; task 09's desktop and phone flows provide the
rendered cancellation, deletion, recovery, and flag-off proof.

## Files likely touched

- `apps/backend/internal/orchestrator/{cancel_turn.go,task_operations.go,errand_hooks.go,errand_deletion.go}`
  and cancellation/isolation tests.
- `apps/backend/internal/errands/{delivery.go,results.go,reconcile.go}` and
  cancellation/reconciliation/metrics tests.
- `apps/backend/internal/gateway/websocket/{dispatch_scope.go,task_notifications.go}`
  and authorization/routing tests;
  task handler/repository lookup adapters as needed for reserved IDs.
- `apps/backend/internal/task/service/service_events.go` and event tests.
- `apps/web/lib/ws/handlers/agent-session.ts`, `lib/ws/client.ts`,
  `lib/state/slices/errands/`, session actions, and their tests. Reuse the
  typed `session.removed` event rather than adding a deletion event family.

## Dependencies

Task 11. Shared cancellation output uses task 10's envelope and task 11's
dispatch guarantees. Task 07 consumes these corrected bounded projections.

## Risks

- New state checks must not turn a parent-turn cancel into a session stop.
- Task-scoped lookup for reserved IDs must not widen another user's authority.
- Omitted `waiting_on_errands` is not equivalent to an explicit cleared value.
- Publish only after durable writes and ignore late events for removed sessions.

## Parallelism

`sequential`

## Inputs

- System-design cancellation, release-toggle, recovery, and observability sections.
- Historical gaps in work orders 04, 05, and 08; existing session event routing.

## Results

Primary implementation acceptance, 2026-10-28: packets 12A-E satisfy the saved implementation checks for the available SQLite environment. The final exact native-recovery selector passed all four packages, and root whitespace checks passed. All owners released processes/writes. PostgreSQL cases were skipped with no test DSN, so PostgreSQL evidence remains unproved and is not accepted as passing. No Docker, Podman or PostgreSQL binaries are installed on this host. Consistent with the manifest's existing PostgreSQL-pilot/broader-support gate, implementation completion releases frontend work but does not establish daily-use, provider or PostgreSQL readiness. The skipped-case inventory and exact/verbose receipts remain under `.kandev/diagnostics/errands-12e-session-93aa2d1e/`.

Primary accepted 12D observability on 2026-10-28 after exact receipt and stopped-writer handoff inspection. 12A-D checkpoints are accepted. The remaining packet 12E owns only saved native-recovery verification and command-evidence reconciliation; full order12 stays in_progress until primary acceptance. No source or test changes are authorized by 12E.

Primary acceptance checkpoint, 2026-10-28: 12A cancellation, 12B disabled/deletion authorization and 12C waiting/removal convergence are accepted from saved exact receipts and stopped-writer handoffs. Order 12 remains in_progress. Packet 12D owns structured observability and bounded labels; native-recovery verification remains a separate subsequent checkpoint. The 12C typecheck needs an 8 GiB Node heap in this environment. Removed-session tombstones clear with task errand state and can grow with deletion count on a long-lived retained task; this is a recorded operational limitation, not a new persistence contract.

In progress. On 2026-10-27 the primary completed the bounded 12A cancellation implementation after child startup failures. `CancelTurn` now settles the observed errand turn through the same durable-terminal reconciliation guard as `CancelAgent`, before closing the turn; errand cancellation no longer invokes parent settlement. The existing `CancelAgent` test and the new `CancelTurn` test create immutable errand roles at session creation. Additional service tests cover pending-result retention on parent cancellation and completion winning the cancellation race.

The valid-fixture `CancelTurn` regression failed with its hook absent, then the full saved orchestrator selector `go test -trimpath ./internal/orchestrator -run 'Errand|CancelTurn|CancelAgent|CancelledTurn|CompletionFollowUp' -count=1` passed (1.701s). Initial builds failed with compiler/cache corruption; the passing run used `TMPDIR=/dev/shm GOCACHE=/dev/shm/errands-12a-gocache GODEBUG=goindex=0 GOFLAGS=-p=2 GOGC=50`. The subsequent Errands race run was interrupted by server problems. As of recovery on 2026-10-28, no test/lint process remains and shared-memory logs/cache are gone. Do not infer a race pass. Changed-file lint remains pending. A fresh 12A validation checkpoint must record surviving file-backed evidence before acceptance.

Recovery validation completed on 2026-10-28. The exact orchestrator selector passed with exit code 0; `go test -race -trimpath ./internal/errands/... -count=1` passed with exit code 0; changed-Go lint reported `0 issues` with exit code 0 for the four cancellation-owned orchestrator files and `internal/errands/cancellation_settlement_test.go`; and focused `git diff --check` checks passed for those tracked files and the untracked cancellation settlement test. This run used the existing Go cache without recurring cache corruption. Command receipts, raw logs, and exit codes are retained in the ignored workspace directory `.kandev/diagnostics/errands-12a-recovery-session-f3578a9e/`. The validation changed no source files. Order 12 remains in progress; this checkpoint validates only 12A, while the remaining cancellation, authorization, convergence, observability, and full-order acceptance work is pending.

Remaining order 12 scope includes disabled/queued authorization, client waiting/removal convergence, observability and all their assigned verification. The full order is not complete; no frontend or browser successor is released.

Primary accepted packet 12A on 2026-10-28 after inspecting its command/exit receipts and the explicit stopped-writer handoff. The next packet, 12B, owns Acceptance 2 only: disabled/enabled deletion notices and enforced authorization for queued reserved-ID cancellation. Client convergence, observability and remaining recovery checks stay pending; this acceptance does not complete order 12. Upstream/main has advanced remotely to `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`; the integrated and lint-comparison base remains `f150e6accd66c9cecdadb56d481180ebc9f75c7f`. No additional merge is part of this packet.

Packet 12B completed on 2026-10-28 for Acceptance 2 only. The backend gateway auth policy now compiles and resolves a reserved errand ID through its persisted errand row to the owning task. An authenticated WebSocket regression uses PAT-derived identities and real workspace owners: queued cancellation succeeds for the owner, foreign-workspace and same-owner cross-task requests fail before the errand row changes, and a retained errand still follows ordinary session deletion while the feature is off. An orchestrator deletion integration uses the real errands service to prove flag-off deletion admits neither prepared output nor a parent notification, while enabled deletion admits exactly one UTF-8 cancellation notice within 16 KiB with a notification snapshot and bounded preview before removing the session and errand row.

The exact saved task/gateway, backendapp authorization, runtimeflag/status, and orchestrator selectors passed; changed-Go lint finished with `0 issues`; tracked and untracked focused whitespace checks reported no whitespace warnings. The 12A errands race receipt was reused because this packet changed no errands package source or tests. Raw logs and exact command/exit receipts for this packet are retained under `.kandev/diagnostics/errands-12b-session-1973b8a0/`. No test or lint process remains. Order 12 stays `in_progress`; client convergence, observability, remaining recovery checks, and full-order acceptance remain pending. No commit, push, PR, default promotion, live-instance change, or workflow-step completion occurred.

Primary accepted packet 12B on 2026-10-28 after inspecting its final command/exit receipts and explicit release of all writes/processes. Packet 12C now owns only waiting/removal client convergence and its focused backend/frontend checks. Observability, native-recovery evidence and final acceptance remain pending; no indicator, Threads, inline-card or browser packet is released.

Packet 12C completed on 2026-10-28 for the client portion of Acceptance 3 only.
Task-service cancel and recovery events now emit an explicit
`waiting_on_errands: false` when no errand rows remain. The existing
`session.removed` event reaches authorized workspace viewers and transcript
subscribers; the frontend consumes it through `removeTaskSession`, resolves
session-only synthetic events from local ownership, and chooses the newest
surviving session when the active session is removed. Removed session IDs are
tombstoned so delayed `errand.list` responses and `errand.updated` events cannot
restore deleted rows. Existing reconnect list refresh was revalidated.

The saved backend selector passed for task service, DTOs, handlers, and gateway
WebSockets. The saved frontend selector passed 120 tests across 6 files; the
reconnect hook test passed 5 tests. `pnpm run typecheck` passed with an 8 GiB
Node heap after default and 4 GiB attempts exhausted the heap in
`rich-output:check`. Changed-file Go and TypeScript lint passed with 0 issues,
and `git diff --check` passed. Exact commands, exit codes, and logs are retained
in `.kandev/diagnostics/errands-12c-session-8ef8a553/`. Order 12 remains
`in_progress`; observability, native-recovery evidence, and full-order
acceptance remain pending. No commit, push, PR, merge, default promotion,
live-instance mutation, or workflow-step completion occurred.

Packet 12D completed on 2026-10-28 for AC-TASKS-ERRANDS-013.1 and .2 only.
Errand start outcomes now emit structured entries, successful starts log both
`errand.started` and `errand.admitted` after the launch state commits, and the
three expvar counters reject labels outside their specified sets. Notification
delivery logs only after prepared admission succeeds. Reconciliation logs only
after a durable state update, including completed/interrupted turns, failed
launch rows, terminal-parent result discards, and released orphaned claims.
Entries carry task, session, errand, and parent IDs as fields. Refused
notification admission and failed reconciliation writes emit no successful
delivery/reconciliation event or delivery count.

The focused observability selector first failed on the missing label guard and
start/reconciliation/delivery logs (exit 1), then passed after implementation
(exit 0). `go test -race -trimpath ./internal/errands/... -count=1` passed
(exit 0), changed-Go lint reported `0 issues`, and tracked/untracked whitespace
checks had no warnings. Exact commands, exit receipts, and raw logs are retained
in `.kandev/diagnostics/errands-12d-session-f38e1a99/`. Native-recovery checks
and remaining order 12 acceptance remain pending; order 12 stays `in_progress`.
No commit, push, PR, merge, default promotion, live-instance mutation, or
workflow-step completion occurred.

Packet 12E completed the saved native-recovery validation on 2026-10-28. The
exact saved selector from the Verification section passed with exit 0; all four
package summaries passed. Source
inventory and verbose output confirmed the lifecycle native-terminal reopen,
mismatch-preserves-fence, and settlement cases; SQLite terminal proof; runtime
turn-summary processing; and interrupted errand reconciliation. The recovery
boundary remains distinct: authenticated peer/history matching, SQL effect and
generation proof, projection, and persisted terminal markers precede errand
abandonment classification.

`KANDEV_TEST_POSTGRES_DSN` was unset. No PostgreSQL process was started or
contacted. The verbose companion run records skips for all 16
`TestPostgresNativeResumePendingTerminalProof` subcases, all 9
`TestRestartBlockBindingRepairPostgres` subcases, the PostgreSQL delivery
retention and restart-notice cases, and both PostgreSQL unpublished-prompt-turn
cases. These are pending database evidence, not passes. An initial verbose
run had an invalid log destination; its captured rerun and the exact saved
command both passed. Commands, exit codes, source inventory, full raw output,
and skip list are in `.kandev/diagnostics/errands-12e-session-93aa2d1e/`.

The saved order-12 command evidence maps to accepted 12A-D receipts plus this
12E selector: errands race and observability in 12D; orchestrator, authorization,
runtime-flag and status-summary selectors across 12A-B; task/gateway and
frontend selectors, typecheck (8 GiB Node heap), and changed Go/TypeScript lint
in 12B-C; changed-Go lint in 12D; and root `git diff --check` in 12E. The root
whitespace check passed with exit 0. No source or test files changed in 12E;
all validation processes are stopped. Order 12 remains `in_progress` pending
primary acceptance, including the unrun PostgreSQL cases. No commit, push, PR,
merge, default promotion, live-instance mutation, or workflow-step completion
occurred.
