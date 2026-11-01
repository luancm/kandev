---
id: "11-recover-lost-cascades"
title: "Reconstruct cancellation before admission"
status: done
wave: 11
depends_on:
  - "10-retry-exact-teardown"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-007.2
  - AC-TASKS-ERRANDS-007.4
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-008.2
  - AC-TASKS-ERRANDS-008.3
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 11: Reconstruct cancellation before admission

## Scope and authority

Repairs review finding(s) 5 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation and verification receipts are recorded in Results below.

## Low-level implementation and regression plan

**Owned:** new `internal/errands/lifecycle_recovery.go` and test; `reconcile.go` startup ordering, `drain.go:drainOne`, `interfaces.go:Store`, `internal/orchestrator/ceiling_sweep.go` bounded lifecycle-recovery invocation via ErrandController extension; `errand_launch.go` for the final parent/task liveness extension. Tests in `errand_service_integration_test.go`. Task/archive code need not be rewritten: its durable `ArchivedAt` is already the source.

**Private interface:** Extend Errands Store with `GetTask(ctx,id) (*models.Task,error)`; SQLite already has it. Use `models.Task.ArchivedAt`, not frontend/scheduler `v1.Task`, which lacks it. Add `ReconcileLifecycle(ctx) error` as a narrow optional controller capability invoked on startup and existing sweep (production must wire it; no permissive fallback). No public API/schema change beyond Task 09.

**Algorithm:**

1. After generic session/native runtime recovery but **before** launched-result reconciliation, notification delivery or Errands admission, load all retained rows and group by parent/task for bounded repeated reads.
2. For each task/parent inspect authoritative durable state: missing/archived task, missing parent, existing `isTerminalParent` states COMPLETED/FAILED/CANCELLED, or mismatched task/role is non-admissible. A parent at WAITING_FOR_INPUT after ordinary turn cancel is **not** terminal; no child cancellation. Do not infer intent from `waiting_on_errands` or a missing execution alone.
3. On positively terminal ownership, call normal cascade cancellation with source `cascade`, queued children included, discard undelivered results and record durable Task 09 teardown obligations for live runtime targets. No user notice. Existing completed delivered snapshots stay historical. Retry existing pending teardown regardless of projection.
4. On task/parent read error, log with IDs and leave durable state unchanged. That row/task is ineligible for this pass; unrelated verified tasks continue. Do not discard results or launch on an error-as-not-found conversion.
5. `drainOne` independently repeats ownership validation under the reservation/admission fence, before every launcher call. If ownership is terminal, cancel instead of launching, then continue FIFO after that removed reservation. If evidence is unreadable or unknown, stop this task's walk (do not overtake it), but drain other tasks. Startup success alone is insufficient because a parent can terminate during normal operation.
6. The final creation/registration/dispatch guard introduced in Task 10, and reused by Task 13, repeats the task/parent/current errand lifecycle check so a stop accepted after the queue scan wins. If a launch already registered before terminal commit, Task 10's durable exact teardown is the compensating path. Never reserve the task's ordinary deferred launch slot.
7. Keep cascadeWorker as low-latency hint, not durable source. A lost memory job is recovered without recreating user-facing events or requiring `Service.Start` in tests. Do not add a polling service; existing 20-second ceiling sweep and startup own retries.

**Regression:** `TestErrandLifecycleRecoveryCancelsLostCascadeBeforeDrain` table persisted FAILED/CANCELLED/COMPLETED/missing parent and archived task with **both** queued child without session and launched child, plus settled pending result. Stop workers, commit durable parent/task state directly to model crash before enqueue, construct fresh service, reconcile then release real ceiling and tick. Assert queued child session never created, launched captured execution stopped, results discarded, no failure/cancellation notice to dead parent. Mix a live parent's queued child and a failed parent's queued child in one inventory; the valid child must still launch. `TestErrandAdmissionRechecksParentAfterQueueScan` uses a channel barrier to change parent after listing; no start. `TestErrandLifecycleReadErrorBlocksOnlyAffectedTask` and a parent-turn-cancel negative prove fail-closed and correct quantifier. Enabled/off table for cancellation recovery; preserve existing flag semantics for retained queued launches rather than inventing a separate promotion rule here.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands -run 'Lifecycle|Cascade|DrainQueued|Reconcile|Teardown' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(ErrandLifecycle|ErrandAdmissionRechecks|ErrandServiceQueues|StoppingTheParent|CeilingSweepTickDrains)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/errands/lifecycle_recovery.go apps/backend/internal/errands/reconcile.go apps/backend/internal/errands/drain.go apps/backend/internal/errands/interfaces.go apps/backend/internal/orchestrator/ceiling_sweep.go apps/backend/internal/orchestrator/errand_launch.go
git diff --check
```

**ACs:** `007.2`, `007.4`, `007.5`, `008.2`, `008.3`, `012.3`. Dependency Task 10. The narrow `errand_launch.go` extension adds owner-liveness validation to its existing final reservation gate. The pre-admission guard must remain when Task 13 replaces blind launch replay.

## Dependencies and ownership

[Task 10](task-10-retry-exact-teardown.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

**Task11 complete and verified.** The final driver `/tmp/errands-task11-final.sh` exited 0 after the last source/test edit. All command handles are consumed and the shared build lock is released.

Durable task archive/missing state and parent task/role/lifecycle now reconstruct lost cascades through `ReconcileLifecycle`, before launched-result reconciliation, claim release or notification delivery. Reads are grouped by task and parent; a read error remains an error, excludes that task from reconciliation/delivery and leaves its rows unchanged. Known terminal ownership uses normal cascade cancellation, including queued reservations without sessions, while pending results are discarded and completed delivered history remains unchanged. Row failures are joined without hiding independently verified siblings. Existing startup ordering already follows authenticated session/runtime recovery; the existing ceiling tick now requires the lifecycle capability before errand drain or delivery. A controller missing that capability cannot authorize either path.

`drainOne` repeats durable ownership checks while holding the existing launch reservation. A cancelled reservation lets FIFO continue; unreadable evidence stops that task without overtaking it, and other tasks drain. Task10's creation, registration, harness-start, dispatch and acceptance fences now repeat current task/parent liveness. The existing exact cancellation owner compensates an allocated runtime. Pending or cancelled teardown intents remain with Task10's bounded retry owner, including flag-off operation; lifecycle reconstruction does not add a second retry loop, poller, cancellation coordinator or ordinary deferred-launch claim.

Two narrow caller integrations were documented in `/tmp/errands-task11-progress.md` before editing: `start.go:launchQueued` classifies final owner refusal before failure bookkeeping can manufacture a result/notification, and `delivery.go:SweepDelivery` uses the grouped, ownership-validated pending-parent walk. Store gains the existing `GetTask(ctx,id) (*models.Task,error)` contract; production SQLite already implements it. The memory fixture receives only the required task read. Task07 successor fences, Task08 role isolation and Task10 exact teardown/admission ownership remain intact. No repository method, schema, public API, shared manifest, public documentation or live instance was changed.

**Behavioral RED and GREEN.** `/tmp/errands-task11-lifecycle-red-final.log` reproduced all 14 persisted ownership-loss/flag variants launching queued children, task-read errors returning success, parent-read errors allowing row changes, missed queue-scan cancellation and missing sweep reconstruction. The WAITING_FOR_INPUT control passed. `/tmp/errands-task11-final-gates-red.log` reproduced creation, registration and dispatch after parent termination. `/tmp/errands-task11-retry-budget-red.log` reproduced lifecycle recovery bypassing Task10's bounded retry owner. Earlier fixture constant/field compile errors and duplicate-workspace setup failure are retained separately and are not behavioral RED or acceptance. The focused GREEN receipts are `/tmp/errands-task11-lifecycle-green.log` and `/tmp/errands-task11-final-gates-green.log`.

The final fixture strengthening passed 10 top-level tests / 26 nodes in `/tmp/errands-task11-acceptance-fixtures.log`: real SQLite errand and message-queue storage, production service/adapter/executor/parent deliverer, an actually filled/released ceiling, and cancellation asserted immediately after fresh Reconcile before the ceiling tick. The notification observer forwards to the production deliverer; read recovery must persist a positive-control notification, while dead/unreadable owners leave the durable queue empty. Channel barriers cover post-scan and pre-creation/registration ownership changes; the production dispatch callback refuses a newly terminal parent. Direct recovery needs no Service.Start or cascade worker. Physical provider behavior is controlled; the inherited fixed-profile fixture is setup, not profile-selection acceptance. Tests preserve completed delivery snapshots, retry an exact failed stop with the flag off, retain intent identity, continue independent tasks, and leave a WAITING_FOR_INPUT parent and its running child intact without inferring intent from absent execution/waiting metadata. A service-boundary unit test covers typed missing-task evidence; enforced SQLite task deletion ordinarily cascades its child rows.

**Final race and caller-build receipts after the last source/test edit, 2026-11-01.** These ran serially under `/tmp/kandev-errands-repair-build.lock` with shared Go/module caches, `GOMAXPROCS=2`, `GOFLAGS=-p=1`, explicit `-p 1` / `-parallel 1` and the known `GOMEMLIMIT=8GiB` compiler allowance. Both race runs have zero skips, failures and race reports.

- `cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'Lifecycle|Cascade|DrainQueued|Reconcile|Teardown|Cancel' -count=1 -v`: **51 top-level tests / 59 passed nodes**, `/tmp/errands-task11-race-errands.log`.
- `cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run '^Test(ErrandLifecycle|ErrandAdmissionRechecks|ErrandServiceQueues|StoppingTheParent|CeilingSweepTickDrains|BulkCancellation|ErrandTeardown|StaleErrandTeardown|QueuedErrandCancellation|LaunchErrandSession|ErrandAdapter|StopSession_ExecutionLessSettlement|StopSession_ExecutionLess(SettledSessionClosesOpenTurn|LaunchingSessionKeepsTurn)|ErrandServiceStartsAndCancels|GenericStopOfAnErrand|StopSessionCascades|StopSessionDoesNotCascade|TerminalTransitionsCascade|DeleteSessionRecords|DeleteErrandSessionAdmits|CancellingAParentTurn|CancelTurnMismatch)' -count=1 -v`: **71 top-level tests / 156 passed nodes**, `/tmp/errands-task11-race-orchestrator.log`. This includes the Task11 integration cases and Task10 exact teardown, reserved-launch, successor, generic-stop and deletion regressions.
- `cd apps/backend && go build -trimpath -p 1 ./internal/errands ./internal/orchestrator ./internal/backendapp`: **passed**, `/tmp/errands-task11-build.log`.

Scoped lint passed with **0 issues**: `cd apps/backend && GOMAXPROCS=1 golangci-lint run ./internal/errands ./internal/orchestrator --new-from-rev=74a91cfc161916f0d5264b86762adae14e43119e --timeout=15m --concurrency=1`, `/tmp/errands-task11-lint.log`. This is the repository scoped helper's same package/base selection with Task10's proven 15-minute loading allowance, rather than repeating its earlier five-minute timeout. Shared caches, `GOFLAGS=-p=1` and the known compiler allowance remained in effect under the exclusive lock. The first driver also passed before fixture strengthening; its `*-before-strengthening.log` receipts do not replace these final runs.

`gofmt -l` over all 11 owned Go files and global `git diff --check` passed. Explicit `git diff --no-index --check -- /dev/null <path>` comparisons for both new Go files and this work order produced no whitespace diagnostics; exit 1 is the expected added-content difference. An initial inventory utility incorrectly treated that normal comparison status as a failure; the corrected utility passed without source changes. `/tmp/errands-task11-verification.json` records exact commands, final hashes, race counts and zero active handles/processes. All **10,549 non-owned tracked backend Go files** match the protected inventory; HEAD remains `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. Nonblocking acquisition verified release of `/tmp/kandev-errands-repair-build.lock`. The initial inventory and pre-edit diff remain `/tmp/errands-task11-before-inventory.json` and `/tmp/errands-task11-before.patch`; the full stage/handle ledger is `/tmp/errands-task11-progress.md`.

**Limits and Task13 handoff.** Storage is unchanged, so Task09's actual PostgreSQL/SQLite upgrade/conformance evidence remains authoritative and no PostgreSQL rerun was required. This does not certify a real-provider crash or teardown guarantee. Tasks12/13 initial-launch evidence and adoption/recovery without replay remain out of scope. Task13 must preserve the owner-liveness guard at entry, creation, registration, harness start, dispatch and acceptance when replacing blind launch replay. Successful-errand automatic parking remains a separate proposal. No commit, push or deployment was performed.
