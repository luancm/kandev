---
id: "07-settle-before-successor"
title: "Capture results before successor admission"
status: done
wave: 7
depends_on:
  - "02-cancellation-settlement"
  - "03-atomic-result-claims"
  - "05-parent-settlement-guard"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.3
  - AC-TASKS-ERRANDS-003.4
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.5
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 07: Capture results before successor admission

## Scope and authority

Repairs review finding(s) 8 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation and final acceptance receipts are recorded below.

## Low-level implementation and regression plan

This packet owns the exact terminal settlement seam specified in the shared design: use existing durable terminal effects/turn records, atomically commit the result or no-result decision, then fence successor reservation under the shared task guard. Include the necessary narrow repository transaction and all actual successor entry-point callers under this packet's ownership.

**Owned:** `internal/orchestrator/event_handlers_agent.go:{handleAgentReady,drainQueuedBeforeWorkflowTransition,settleDetachedAgentReadyTurn}`, `errand_settlement.go:{settleErrandTurn,evaluateCompletedAgentWorkflow}`; new `internal/orchestrator/errand_queue_settlement_test.go`. Any shared queue-admission helper edit must be limited to the settlement precondition described below and integrated with Task 03's guard and result-revision contract.

**Algorithm and ordering:**

1. Keep existing event execution/turn/generation fences and cancellation guard. Complete the observed durable turn, preserving its immutable `data.TurnID`/`DeliverySubmissionID` or ready-captured turn identity. Do not read a successor as the event's turn.
2. For an errand, write `not_required` for workflow as today and capture/settle this exact child turn **before** attempting an automatic queue drain. Return errors from `settleErrandTurn` instead of logging-and-continuing. Result settlement owns parent/user-origin classification and CAS; do not duplicate that logic here.
3. Publish session idle through `setSessionWaitingForInputWithoutTaskState` after successful capture. Wake parent awaiters after the durable row/session commits as today. The service may run its normal parent result delivery check; it does not mutate the child's queue.
4. Only then reserve/launch the next child FIFO entry. A user successor keeps the earlier pending result and settled outcome. A parent successor goes through existing `OnParentPromptAdmitted`, intentionally supersedes the pending result and sets awaiting for its own identity. A duplicate or stale old ready event does not capture the successor or clear its outstanding state.
5. For a parent, call Task 05 settlement before drain. If known outstanding or unreadable, the drain's promptability transition uses the no-task-state setter; otherwise preserve ordinary review-state behavior. Pass that private decision into the drain explicitly instead of a second fallible inventory read. Errand role unconditionally selects no-task-state, including flag off.
6. Failure to capture must not allow a successor to make the previous turn undiscoverable. Leave that FIFO row unclaimed, settle the session idle without task writes, and retain original terminal evidence. Use this packet's durable exact-turn settlement transaction at queue admission before any later FIFO/Send Now retry can supersede it. The required precondition is "every terminal parent-origin turn preceding the prospective successor has its result/discard decision committed", not merely "latest turn is completed". This precondition must cover Auto-run toggle/readiness and Send Now, not only one ready handler. Do not create an in-memory-only pending bit. This packet must implement and wire that transaction before accepting successor admission.
7. Completed-process path already captures detached turns before its final queue drain; keep that property and apply the same failure/precondition semantics. Do not auto-run a queue after user cancellation, which retains its separate paused-queue policy.

**Tests:** `TestErrandReadyCapturesResultBeforeQueuedUserTurn` uses real SQLite and real Errands with TurnReader. Seed turn A plus final assistant sentinel, Auto-run on, and queued user turn B. Gate provider admission with channels; once B is observable assert A's pending result/text/turn and last outcome, `LatestSettledTurnID=A`, no task-state writes and no workflow move. Do not insert sleeps. `TestErrandReadyQueuedParentTurnSupersedesCapturedResult` proves intentional supersession only for verified parent source. `TestParentOutstandingQueueDrainDoesNotWriteReview` observes task state through a real drain; include one queued ordinary entry, not just empty queue. `TestErrandSettlementFailureParksSuccessorUntilDurableRetry` faults the row CAS, then retries the real readiness path; B must not dispatch until A's capture succeeds. Add duplicate/stale A event after B admission and inspect B's turn, submission, active state and pending input unchanged.

```bash
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(ErrandReady|ErrandSettlementFailure|ParentOutstandingQueueDrain|ErrandAgentReady|ErrandAgentCompleted|ErrandTurnComplete|ErrandParentSettlement|DrainQueued)' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/errands -run 'Settlement|Cancelled|ParentPrompt|Supersed|Capture' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/errand_settlement.go
git diff --check
```

**ACs:** `003.1`, `003.3`, `003.4`, `005.1`, `006.1`, `006.2`, `006.5`, `012.3`. Dependencies: Task 05, exact-turn result-settlement packet. No public schema/API change from this packet.

## Dependencies and ownership

[Task 02](task-02-cancellation-settlement.md), [Task 05](task-05-parent-settlement-guard.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

Parallel-safe with Task04C frontend/localization: Task07 owns the backend exact-turn settlement and successor-admission boundary; frontend owns only web files. The user authorizes two disjoint workers. All heavy validation uses `/tmp/kandev-errands-repair-build.lock` and `/tmp/errands-verification-resource-policy.md`. Record the necessary backend inventory before edits; no shared schema or repository writer may overlap this packet.

## Results

Done on 2026-11-01. The observed exact turn settles before successor drain. Admission-origin intent, terminal evidence and a result/no-result decision use the existing `agent_delivery_effects` mechanism; decision and result CAS commit together under the task guard. Turn creation (including stamped/fallback paths), direct session claims, parent reservations, FIFO/take and Send Now enforce the same preceding-decision precondition before reserving a successor, with no provider I/O under a database lock. Failed or unknown capture preserves the source and leaves the session promptable without task-state writes. Recovery reads exact canonical output and matching resolved terminal or committed projection-cursor evidence; foreign, incomplete, missing and contradictory evidence stays blocked.

Real queue regressions prove user B preserves A's result, verified parent B intentionally supersedes it through Task03, known/unreadable parent inventory never writes REVIEW during drain, capture/intent faults retain the source, and readiness/FIFO/Send Now recover after service recreation. Late A ready/completed events preserve B's turn, submission, execution marker, active state and queued input. User/parent/cascade cancellation preserves its notice or discard and paused queue. Exact receipt replay returns the current committed delivery projection; only a newly committed parent result increments the existing settlement metric.

### Final validation

Commands ran sequentially under `/tmp/kandev-errands-repair-build.lock`. Shared caches were `GOCACHE=/home/luancm/.cache/go-build` and `GOMODCACHE=/home/luancm/go/pkg/mod`, with `GOMAXPROCS=2` and `GOFLAGS=-p=1`. Tests, compiler and export preparation used `GOMEMLIMIT=8GiB`; lint analysis and the final light SQL guard used `3GiB`. The disposable PostgreSQL DSN was `postgres://postgres@127.0.0.1:55487/postgres?sslmode=disable`; each test owned an isolated schema, the pre-existing unattributed schema was preserved, and the owned server stopped successfully. All handles are consumed.

From `apps/backend`:

- `go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run 'Test(ErrandReady|ErrandSettlementFailure|ParentOutstandingQueueDrain|ErrandAgentReady|ErrandAgentCompleted|ErrandTurnComplete|ErrandParentSettlement|DrainQueued|CancelledTurn|SettleErrandTurn|HandleAgentCompleted_MarksRotatedExecutionCompleted)' -count=1 -v`: 19 tests and 28 subtests passed, no skips. `/tmp/errands-task07-orchestrator-race.log`.
- `go test -race -trimpath -p 1 -parallel 1 ./internal/task/repository/sqlite -run '^Test(Postgres)?ErrandExactTerminalDecision' -count=1 -v`: 16 tests and 22 subtests passed, including actual PostgreSQL, no skips. Covers atomic row/receipt rollback, independent-connection guarded reservation ordering, missing/conflicting receipts, pre-existing successors, committed replay snapshots and exact projected-terminal recovery. `/tmp/errands-task07-repository-race.log`.
- `go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'Settlement|Cancelled|ParentPrompt|Supersed|Capture' -count=1 -v`: 15 tests and 5 subtests passed, no skips. `/tmp/errands-task07-service-race.log`.
- `go test -race -trimpath -p 1 -parallel 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3|pgx)/^task$' -count=1 -v`: both dialects and all 16 subcases passed, no skips. `/tmp/errands-task07-conformance.log`.
- `go run -trimpath -p 1 ./cmd/sqlguard ./internal`: passed, including the final `GOMEMLIMIT=3GiB` run. `/tmp/errands-task07-sqlguard-light.log`.
- `go build -trimpath -p 1 ./internal/backendapp`: passed, compiling the changed packages and production callers. `/tmp/errands-task07-build.log`.
- `golangci-lint run ./internal/db ./internal/errands ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/task/repository/sqlite --new-from-rev=74a91cfc161916f0d5264b86762adae14e43119e --timeout=5m --concurrency=1`: passed with 0 issues. Package/base selection matches `scripts/lint-go-changed`; explicit concurrency one obeys the shared resource policy. Exact export preparation used `go list -p 1 -buildvcs=false -pgo=off -tags=fts5,e2e -deps -test -export -compiled` for the same packages. `/tmp/errands-task07-lint.log`, `/tmp/errands-task07-lint-exports.log` and `/tmp/errands-task07-lint-accepted-wrapper.log`.

The complete final runtime command receipt is `/tmp/errands-task07-verify-accepted.log`; reproducible scripts are `/tmp/errands-task07-verify.sh` and `/tmp/errands-task07-lint.sh`. `gofmt -l` reports no owned files and `git diff --check` passes. Documentation validation passed (422 decisions and 1862 specifications), and all specification files passed lint; receipts are `/tmp/errands-task07-docs-validate.log` and `/tmp/errands-task07-spec-lint.log`. Initial before hashes are `/tmp/errands-task07-before.txt`; final tested SHA-256 values are `/tmp/errands-task07-verified-source.sha256`, verified unchanged after lint.

### File inventory and RED evidence

The exact newline-separated Go inventory is `/tmp/errands-task07-files.txt` (25 files): `internal/db/errandsettlement.go`; `internal/errands/{results.go,reconcile.go,terminal_decisions.go}`; `internal/orchestrator/{event_handlers_agent.go,event_handlers_workflow.go,errand_settlement.go,errand_admission.go,errand_terminal_admission.go,errand_terminal_event_guard.go,agent_delivery_prepared_prompt.go,task_operations.go,queue_send_now.go,errand_queue_settlement_test.go,errand_queue_fences_test.go,errand_parent_queue_test.go,errand_settlement_test.go}`; `internal/orchestrator/messagequeue/{repository_sqlite.go,errand_settlement.go}`; and `internal/task/repository/sqlite/{session.go,errands_admission.go,errands_turn_settlement.go,errands_turn_terminal_evidence.go,errands_turn_settlement_test.go,errands_turn_fences_test.go}`, all beneath `apps/backend/`. The existing settlement test changed only to assert the new returned errors. This work order is the only documentation file changed by this packet.

Behavioral RED logs: `/tmp/errands-task07-red-valid.log` (A missing at B admission and capture faults claiming B), `/tmp/errands-task07-green2.log` (stale completed A retiring B), `/tmp/errands-task07-fences-red.log` (legacy completion snapshot, SQLite writer ordering and conflicting receipts), `/tmp/errands-task07-successor-red.log` (pre-existing successor), `/tmp/errands-task07-replay-red.log` (obsolete publication row), `/tmp/errands-task07-metrics-red.log` (missing existing counter), `/tmp/errands-task07-projection-red.log`, `/tmp/errands-task07-projected-queue-red.log` and `/tmp/errands-task07-cursor-red.log` (exact durable recovery). Every final affected regression reran after the final production edit.

Fixture-only timeouts, the initial partial delivery-identity fixture, a script syntax failure, the deliberately interrupted premature run, and the two resolved lint rounds are recorded in `/tmp/errands-task07-progress.md`; they are not acceptance passes. The inherited lifecycle-drain test can log a post-cleanup closed-DB acknowledgement while passing; the new packet fixtures join their complete queue workers and emit no such diagnostic.

No Task07 acceptance gap remains. Missing durable evidence intentionally stays fenced. The separate parent-terminal producer mismatch in `/tmp/errands-turn-mismatch-trace.md` and `/tmp/errands-restart-stall-2026-11-01.md` remains pending: these tests preserve successor identity but do not establish that producer's cause or fix it. Changes remain uncommitted and undeployed; shared manifest, public docs, frontend and live instances were not changed by this packet.
