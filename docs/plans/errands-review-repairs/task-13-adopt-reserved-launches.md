---
id: "13-adopt-reserved-launches"
title: "Recover launch bookkeeping without replay"
status: done
wave: 13
depends_on:
  - "08-created-errand-isolation"
  - "11-recover-lost-cascades"
  - "12-reserved-launch-authority"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.1
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.2
  - AC-TASKS-ERRANDS-008.2
  - AC-TASKS-ERRANDS-008.3
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 13: Recover launch bookkeeping without replay

## Scope and authority

Repairs review finding(s) 7 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation and final acceptance receipts for this packet are recorded in Results below.

## Low-level implementation and regression plan

**Owned:** `internal/errands/{start.go,reconcile.go,drain.go,interfaces.go}`, `internal/orchestrator/{errand_launch.go,errand_adapter.go,errand_launch_recovery.go}`; real-stack integration and `start_test.go`/`reconcile_test.go`/`drain_test.go`. Limit generic `task_operations.go` changes to reserved-turn continuation only if the existing turn service cannot return an exact pre-existing turn safely.

**Private contract:** Change the existing capability to `Launcher.LaunchErrand(ctx context.Context, request LaunchRequest) (LaunchResult, error)`; do not add a competing reconcile capability. `LaunchResult` carries `Disposition` (closed `queued`, `admitted`, `settled`, `blocked`) and the verified `SessionID`, `TurnID`, `SubmissionID`. `queued` means a definite capacity refusal before any effect; `admitted` includes known accepted or dispatch-uncertain effects and prohibits replay; `settled` requires exact terminal evidence; `blocked` retains inconclusive/mismatched evidence. Only `admitted`/`settled` permit queued-to-launched bookkeeping, never a nil error alone. Transient inspection/write errors return `blocked` plus the error. A typed `ErrLaunchRejected` may authorize failure recording only when durable evidence proves the initial attempt definitively failed before admission; other errors never synthesize a failed result.

Migrate `start.go`, `drain.go` and startup `reconcile.go`, the `ErrandAdapter`, and all Launcher fakes together. Put classification, final cancellation/owner checks and effect admission in one private orchestrator operation used by both the adapter and `LaunchErrandSession`; the latter retains its external response signature, maps known queued refusal to the existing ceiling error and blocked results to a typed recovery error. Direct callers cannot bypass the classifier. External Start response shape remains unchanged. The shared operation owns the per-session guard once, avoiding nested guard acquisition by wrappers.

**Algorithm:**

1. Own reservation via existing `launchGuard` and the same per-session admission/cancellation arbitration before inspect/create. The guard map alone is process-local and cannot substitute for durable evidence. Re-read current row and Task 11 owner state; a cancelled/missing row never launches.
2. Inspect Task 12 evidence **before** calling `PrepareSession`. `admitted/settled` -> CAS only queued->launched; keep result/outcome/awaiting fields unchanged until exact reconciliation. Log/count admission once after durable write. A repeated record failure keeps evidence recoverable and must not route to `applyLaunchFailure`.
3. `prepared` -> continue the existing errand session through Task 08 detached start policy; reserve/use exactly `FirstTurnID` and `SubmissionID`. If the exact turn already exists unattempted, reuse it with the existing reserved-turn APIs; do not create a new UUID or complete/delete it to evade a uniqueness constraint. Preserve accepted prompt/MCP snapshot and workspace. Before external dispatch atomically establish durable dispatch/admission evidence through existing delivery callbacks.
4. `absent` -> existing first launch path, still capacity-checked and FIFO. Ceiling refusal creates no session or submission and leaves the errand queued without touching task deferral.
5. `blocked` -> persist/use the existing recovery-block seam on a matching session where safe, log full IDs, leave retry evidence and stop this task's FIFO walk. Do not convert ownership ambiguity or duplicate-key rejection into a failed result, and do not stop an unknown/successor runtime. Other verified tasks may continue.
6. Startup: ownership cancellation recovery first, queued reservation adoption second, existing launched-result recovery third, then claims/delivery. Preserve upstream native pending-terminal/SQL generation/effect proof before classifying interruption; no blanket "idle means failed" fallback.
7. If cancellation wins during launch, `recordLaunched` must preserve cancelled lifecycle and service Task 10's exact stop intent. Failure after runtime admission cannot remove the errand reservation. `recordLaunchFailure` may create a failure only on positively proved refusal/failed launch, not on an inspection/write failure or an existing working runtime.
8. Unknown/accepted/uncertain submission never causes a new provider prompt. Physical runtime recovery is handled by existing authenticated runtime adoption/reconciliation, not Errands-specific ACP initialize/new/load calls.

**Regressions:** Port `TestReviewDiagnosticRealStackLaunchBeforeErrandRecord` into `TestErrandLaunchRecordFailureAdoptsReservedSessionWithoutReplay`: production SQLite/adapter/executor, successful runtime launch, fail first UpdateErrand queued->launched, then fresh Errands service and Reconcile + real ceiling tick. Assert lifecycle launched, original FirstTurnID/SubmissionID preserved, awaiting stays true, no synthetic failed result, exactly one actual LaunchAgent/start/initial-prompt admission, stop count zero. `TestErrandLaunchRecoveryAfterPreparedSessionBeforeFirstDispatch` persists only prepared session + authority, recreates service, proves same session/turn/submission continues once. `TestErrandLaunchRecoveryUncertainSubmissionNeverReplays` simulates acceptance with response loss and proves no duplicate StartAgentProcess or Prompt. `TestErrandLaunchRecoveryRejectsMismatchedAuthority` proves zero side effects for wrong tuple, with matching positive control. `TestErrandLaunchRecordFailureCancellationWins` barriers around runtime success/record and cancellation; assert eventual exact teardown and no resurrection. These tests must seed actual authority/receipt shapes, not boolean fake acceptance.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands -run 'Start|Launch|DrainQueued|Reconcile|Cancel' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(ErrandLaunch|LaunchErrandSession|ErrandServiceQueues|QueuedErrandCancellation|CreatedErrand|ErrandMissingExecutionFallback)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/backendapp ./internal/errands -run 'NativeResumePendingTerminal|RuntimeTurnChangeSummaryProcessing|Reconcile|Restart' -count=1)
bash scripts/lint-go-changed apps/backend/internal/errands/start.go apps/backend/internal/errands/reconcile.go apps/backend/internal/errands/drain.go apps/backend/internal/errands/interfaces.go apps/backend/internal/orchestrator/errand_launch.go apps/backend/internal/orchestrator/errand_adapter.go apps/backend/internal/orchestrator/errand_launch_recovery.go
git diff --check
```

**ACs:** `001.1`, `002.1`, `004.8`, `007.1`, `007.2`, `008.2`, `008.3`, `012.3`. Dependencies Task 08/Task 11/Task 12. Tasks 03, 04 and 07 own exact result capture and claim reconciliation; this packet supplies truthful launch evidence, not a second interruption policy.

## Dependencies and ownership

[Task 08](task-08-created-errand-isolation.md), [Task 11](task-11-recover-lost-cascades.md), [Task 12](task-12-reserved-launch-authority.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Implemented and verified on 2026-11-01 against unchanged HEAD `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. Changes remain uncommitted. This packet changes no schema, SQL dialect behavior, runtime protocol, shared repair manifest, public documentation or live instance.

### Behavior and acceptance

The private Launcher contract now returns a closed typed disposition and verified session/turn/submission identity. Direct `LaunchErrandSession` and adapter calls share guarded evidence collection and the unchanged Task 12 classifier before preparation. The collector independently reads the reserved, active and latest turns, normalized submission metadata, immutable session authority and restored runtime/persisted inventory evidence. Missing inventory, pending native recovery, conflicting ownership or successors, and failed reads remain blocked; a runtime map miss alone cannot authorize a launch. Existing native recovery blocks remain authoritative.

Known admission and exact terminal proof authorize queued-to-launched bookkeeping without another prepare, process start or provider prompt. A failed record write leaves the reservation recoverable. Counting occurs after the committed CAS, once, even when a later projection fails. Prepared recovery uses the existing detached errand policy, retained profile/mode/environment/MCP and accepted prompt, and the exact first turn/submission; it reuses an existing unattempted turn. Existing durable delivery callbacks establish dispatch evidence before provider I/O. Definite capacity refusal preserves the queue and ordinary task deferral behavior. Unknown inspection/write errors do not produce failed results; only typed positively proved rejection permits that path.

Startup now performs owner/cancellation recovery, queued adoption, exact launched-result reconciliation, then await/claims/delivery recovery. A blocked reservation stops only its task's FIFO walk. Cancellation-winning records cannot resurrect an errand and retain the exact Task 10 teardown obligation. The accepted Task 08 detached-start and Task 11 owner fences remain covered.

- `TestErrandLaunchRecordFailureAdoptsReservedSessionWithoutReplay` exercises real SQLite, the production service/adapter/executor and real dispatch callbacks. The first launch bookkeeping write fails after admission; a fresh service, startup reconciliation, actual ceiling tick and repeated reconciliation adopt the same IDs. Awaiting remains true, no result is synthesized, and allocation/process start/initial dispatch occur once with no follow-up prompt or stop. The started counter changes once, after commit.
- `TestErrandLaunchRecoveryAfterPreparedSessionBeforeFirstDispatch` covers prepared-only and existing unattempted reserved-turn states after SQLite reopen. Both resume once with the same session/turn/submission, preserved accepted prompt and environment, an accepted submission receipt and unchanged pre-existing turn timestamps.
- `TestErrandLaunchRecoveryUncertainSubmissionNeverReplays` uses actual admission callbacks, then canonical interrupted/unknown SQL evidence representing response loss and a failed session projection. Recovery neither dispatches again nor creates a false failed result.
- Mismatched authority, independent active/latest successor conflict, nil inventory, transient submission reads, task authorization, concurrent direct callers and an existing native recovery block exercise blocked or retried paths with zero forbidden launch effects and matching positive controls.
- `TestErrandLaunchRecordFailureCancellationWins` places a barrier between admission and bookkeeping, then proves exact eventual stop and no resurrection. Unit coverage also proves affected-task FIFO blocking with an unrelated task continuing, unknown-error retention, and committed counting despite projection failure. All 92 Task 12 classifier subcases remain passing.

### TDD receipts

Behavioral failures preceded the corresponding production repairs. `/tmp/errands-task13-red-bookkeeping.log` reproduced admitted work left queued after record failure; `/tmp/errands-task13-red-prepared-uncertain.log` reproduced both prepared-continuation failures; `/tmp/errands-task13-green-prepared-red-uncertain.log` reproduced uncertain delivery after tightening the projection barrier; `/tmp/errands-task13-green-continuation-red-fences.log` reproduced authorization bypass and a hidden attempted successor; `/tmp/errands-task13-green-fences-red-snapshot.log` reproduced missing acceptance receipts on prepared continuation; `/tmp/errands-task13-red-commit-metric.log` reproduced the lost postcommit counter; `/tmp/errands-task13-red-native-block.log` reproduced bypass of an existing native recovery gate. Initial wrong-directory/compile errors, fixture corrections and the two corrected staticcheck findings are not acceptance passes; their dispositions remain in `/tmp/errands-task13-progress.md`.

### Final commands and observed results

All following Go commands ran from `apps/backend` through `/tmp/errands-task13-run`, which reads `/tmp/errands-verification-resource-policy.md`, checks memory/disk, holds `/tmp/kandev-errands-repair-build.lock` for the command's full lifetime, and sets `GOMAXPROCS=2 GOFLAGS=-p=1 GOCACHE=/home/luancm/.cache/go-build GOMODCACHE=/home/luancm/go/pkg/mod`. Compilation/export warming used `GOMEMLIMIT=8GiB`; lint used `GOMAXPROCS=1 GOMEMLIMIT=3GiB` and concurrency 1. Final disk checks showed at least 21 GiB available. No concurrent cache cleanup or live database access occurred.

```bash
go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'Start|Launch|DrainQueued|Reconcile|Cancel' -count=1 -v
go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run '^Test(ErrandLaunch|LaunchErrandSession|ErrandServiceQueues|QueuedErrandCancellation|CreatedErrand|ErrandMissingExecutionFallback|ErrandAdmissionRechecks|OrdinaryStartStillDefersAtTheCeiling|ErrandAdapter|ErrandMCPInheritance)' -count=1 -v -timeout 180s
go test -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/backendapp ./internal/errands -run 'NativeResumePendingTerminal|RuntimeTurnChangeSummaryProcessing|Reconcile|Restart' -count=1 -v -timeout 180s
go build -trimpath -p 1 ./internal/errands ./internal/orchestrator/executor ./internal/orchestrator ./internal/backendapp
go list -p 1 -export -deps -test -tags 'fts5 e2e' -buildvcs=false -pgo=off ./internal/errands ./internal/orchestrator ./internal/backendapp
env GOMAXPROCS=1 GOMEMLIMIT=3GiB golangci-lint run ./internal/errands ./internal/orchestrator ./internal/backendapp --new-from-rev=74a91cfc161916f0d5264b86762adae14e43119e --timeout=15m --concurrency=1
```

| Final check | Passed top-level tests / passed nodes | Skipped nodes | Log |
| --- | --- | --- | --- |
| Errands race | 58 / 70 | 0 | `/tmp/errands-task13-final3-race-errands.log` |
| Orchestrator race | 40 / 151 | 0 | `/tmp/errands-task13-final3-race-orchestrator.log` |
| Native lifecycle compatibility | 86 / 137 | 0 | `/tmp/errands-task13-final3-compatibility.log` |
| SQLite reconciliation/restart compatibility | 27 / 76 | 29 PostgreSQL | Same compatibility log |
| Backendapp reconciliation/restart compatibility | 10 / 15 | 0 | Same compatibility log |
| Errands reconciliation/restart compatibility | 12 / 12 | 0 | Same compatibility log |
| Caller build | Passed | Not applicable | `/tmp/errands-task13-final3-caller-build.log` |
| Lint export warm | Passed | Not applicable | `/tmp/errands-task13-final-lint-export-after-style.log` |
| Scoped lint | Passed, zero issues | Not applicable | `/tmp/errands-task13-final-lint-after-style.log` |

Passed nodes include the top-level tests and their subcases; they are not an additional independent-test count. All final test selectors executed tests, with zero failures or race reports. The compatibility selector includes native pending-terminal, generation/effect-proof and restart paths. `KANDEV_TEST_POSTGRES_DSN` was unset; the 29 PostgreSQL skips provide no PostgreSQL acceptance. This packet introduces no SQL/schema/dialect behavior requiring a PostgreSQL run. The final lint command uses the repository resolver's comparison base, recorded in the command above, and the changed-package scope used by `scripts/lint-go-changed`; the 15-minute allowance matches the accepted resource policy.

### Preservation, formatting and evidence limits

The before inventory records 361 pre-existing changed files in `/tmp/errands-task13-before-hashes.json` and 10,667 backend Go files in `/tmp/errands-task13-protected-backend-hashes.json`. Final owned changes comprise 14 existing Go files, three new Go test files and this work order. Hash comparison preserved all 10,653 unowned inventoried Go files and all 348 other pre-existing changed files, with no unexpected Go paths or unowned changes. The full final receipt records source/work-order SHA-256 hashes, command results, per-package test counts, preserved files and lock availability. No shared manifest, public documentation, commit, push, deployment or live instance was changed.

`gofmt -l` on all 17 owned Go files, `git diff --check`, explicit whitespace checks for new files and the work order, and the protected-inventory comparison passed. All command handles were consumed and the build lock was released. Machine-readable receipts: `/tmp/errands-task13-final-receipt.json`, `/tmp/errands-task13-final-file-receipt.json` and `/tmp/errands-task13-final-test-counts.json`.

The integration tests use real SQLite and production service, adapter, orchestrator, turn/submission storage and executor logic with a controlled physical provider boundary. Response loss is injected into canonical durable evidence after real admission callbacks; it is not an observed provider/network crash. Fixed scheduler/profile fixtures establish test prerequisites and do not certify general profile routing. No live provider crash or real-provider teardown guarantee is claimed. Incomplete legacy authority and unresolved native recovery intentionally remain blocked. Runtime adoption stays in the existing authenticated runtime layer.

### Final source SHA-256 inventory

Paths are relative to the repository root. The final machine-readable receipt also includes this work order’s hash.

| Source | SHA-256 |
| --- | --- |
| `apps/backend/internal/backendapp/auth_errands_test.go` | `88116919c8d5e6ddd35e2f477209beefeb39ce014570044f0c314232dcc5dd35` |
| `apps/backend/internal/errands/drain.go` | `08696ab05aa124ca331c415293b584d68aa479d9334a227e184c61de0dcb703b` |
| `apps/backend/internal/errands/drain_test.go` | `88c928f0e109c9375f03024f1260fa04c2a4b9245e7ef75385308bda82152759` |
| `apps/backend/internal/errands/fakes_test.go` | `dbdf17281bae6051a5d0f3ec50ad078ffe133b3d2d513d5f0928b04ab6862f1c` |
| `apps/backend/internal/errands/interfaces.go` | `bf177cd9c739ece24891135a5a02cac4408f76e7c96cd6380b2cf5c79700270b` |
| `apps/backend/internal/errands/reconcile.go` | `62f15010d0e7880ae8850baf047884645b9dc73abc372f27f4f1b937a0baa014` |
| `apps/backend/internal/errands/start.go` | `0f8d6670ca04cdd9e417c611cf5e43cb6b49e3df58ff4635669f9b3dd6b8efbe` |
| `apps/backend/internal/errands/start_test.go` | `cdd8c3a136a68393e37e9e4ffff100a62d335f08c741690202657bf56c79cf57` |
| `apps/backend/internal/orchestrator/errand_adapter.go` | `fbd49bfb82b1b090138b633e2dce382f00a3e02fd51c0317ca042ab324a4cfcf` |
| `apps/backend/internal/orchestrator/errand_deletion_test.go` | `8bf891c1126324c345b47174e6dbd1ab63fd4a3e52445f80147c4fa06799abd9` |
| `apps/backend/internal/orchestrator/errand_launch.go` | `f54a2d0cf3428759c3b62b44d433d43a3a9a06be94c9a42e998070a0b635ecb3` |
| `apps/backend/internal/orchestrator/errand_launch_adoption_test.go` | `464d90dfb8d01cfd3dd8dbbab2ea220bbaa2d0b8f0e9308589ee829fb2e4ebcf` |
| `apps/backend/internal/orchestrator/errand_launch_faults_test.go` | `004e72676013b49148eb45316cbd57fa25503d42763bd5067c6abaee0343ac52` |
| `apps/backend/internal/orchestrator/errand_launch_prepared_test.go` | `81ec9537c1374f7fdea1bf5b783159e3ad5c498163defd0b2177927b9153c09b` |
| `apps/backend/internal/orchestrator/errand_launch_recovery.go` | `ed81687cb5bde3d33888bcf91f9ee270a6e5903d34105ce9a5c32b77a6b4099e` |
| `apps/backend/internal/orchestrator/errand_launch_test.go` | `4ce598e97a6a86bc5bc0022ccea79ad50fc51bf57b98415fc4425098ecdf1708` |
| `apps/backend/internal/orchestrator/task_operations.go` | `d3f6329632a67fd38c0319543207030d8d919eddaf7f7a89f79637f1e4bc642c` |
