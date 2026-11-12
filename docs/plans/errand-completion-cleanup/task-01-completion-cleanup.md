---
id: "01-completion-cleanup"
title: "Close completed errand runtimes and preserve continuation"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-ERRAND-COMPLETION-001.1
  - AC-TASKS-ERRAND-COMPLETION-001.2
  - AC-TASKS-ERRAND-COMPLETION-001.3
  - AC-TASKS-ERRAND-COMPLETION-001.4
system_design:
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
---

# Task 01: Close completed errand runtimes and preserve continuation

## Outcome and scope

Close only the successful settled errand execution, preserve its result and conversation, and expose a completed retained session that can explicitly resume. Extend the existing durable teardown boundary with success effects and reuse its bounded recovery. Do not use cancellation to represent success. Include positive-evidence repair of older retained successful sessions and ordinary-session controls.

## Acceptance

1. Ready and process-exit paths commit exact parent-result or user-no-result settlement before close. Confirmed cleanup publishes `COMPLETED` without cancellation notices, lost/duplicate delivery, task/workflow writes or shared-workspace removal. Settlement/stop/receipt failure retains recoverable evidence and does not claim completed cleanup.
2. Durable retry survives restart and flag-off admission. Legacy cancellation intents preserve their meaning; cancellation wins correctly over concurrent success cleanup. Controlled barriers prove a same-execution successor and replacement incarnation cannot be stopped or overwritten. Legacy retained-session repair needs exact successful-turn and authenticated absence evidence; mixed valid/unknown sessions are handled independently.
3. Explicit user and parent follow-up resume the same retained errand using existing authority/recovery controls; result supersession and user opt-in remain correct. Desktop and native-phone history show completed state, preserve transcript/context and Open/Back, and inspecting a child starts no runtime. Ordinary sessions retain their waiting policy.

## Files and ownership

The implementation Errand owns only the following responsibility areas, with additions limited to adjacent focused tests/helpers when needed:

- `apps/backend/internal/orchestrator/event_handlers_agent.go`, `errand_settlement.go`, `errand_teardown.go`, `errand_adapter.go`, `task_operations.go`, and their existing completion/admission/recovery integration seams.
- `apps/backend/internal/errands/` successful settlement and existing teardown reconciliation; `apps/backend/internal/task/models/errand_teardown.go` and the existing SQLite errand intent persistence/receipt methods and conformance tests.
- Existing parent-message and user-follow-up admission callers only where required to permit a validated completed errand resume. No broad normal-session policy changes.
- `apps/backend/internal/orchestrator/errand_isolation_test.go`, `errand_queue_fences_test.go`, `errand_queue_settlement_test.go`, `errand_teardown_test.go`, `errand_teardown_integration_test.go`, `completed_session_resume_test.go`; add a focused `errand_completion_cleanup_test.go` for the new fault/race matrix.
- `apps/web/e2e/tests/session/errands.spec.ts` and `mobile-errands.spec.ts`; existing web state/continuation components only if the existing completed-session flow requires an errand-specific correction. Localize any new copy in every supported locale.
- Public completion/recovery wording in `docs/public/coordination.md` and `docs/public/agent-communication.md` after implementation.

The primary owns this manifest, requirements/design and final integration. The worker is not alone in the checkout: preserve others' changes, do not revert unrelated work, do not spawn further agents, and return exact changed paths, hashes, command results and released process/lock evidence. No staging, commit, push or live-instance mutation by the worker. If the required change crosses a materially different boundary, report it before expanding scope.

## TDD and verification

First add behavioral RED tests under `TestErrandCompletionCleanup*` using production SQLite/service paths and controlled runtime boundaries. Assert both ready and process-exit cleanup and completed state, result/no-result retention, failed close/receipt retry, concurrent successor protection, cancellation precedence, flag-off/restart recovery, legacy absence evidence, and explicit user/parent continuation. Update the two existing successful-errand isolation assertions that currently expect `WAITING_FOR_INPUT`; retain ordinary-session controls. Use channel barriers, not timing sleeps. Keep new storage/model test names under `TestErrandCompletionCleanup*` as well.

Run from repository root with the established shared build lock, shared caches, `GOFLAGS=-p=1`, `GOMAXPROCS=2`, and the documented compiler memory allowance. Retain each process handle to terminal status. Exact acceptance commands:

```bash
(cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run 'Test(ErrandCompletionCleanup|ErrandAgentReady|ErrandAgentCompleted|CompletedSessionResume|ErrandTeardown|StaleErrandTeardown|QueuedErrand)' -count=1 -v)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/errands ./internal/task/models ./internal/task/repository/sqlite -run 'ErrandCompletionCleanup|Teardown|Reconcile|Terminal' -count=1 -v)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator/executor -run 'StopSession|StopExecution|Errand' -count=1 -v)
(cd apps/backend && go build -trimpath -p 1 ./internal/errands ./internal/orchestrator ./internal/backendapp)
make build-web
make build-backend
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run `bash scripts/lint-go-changed` with every changed Go file explicitly listed. If web source changes, run typecheck, exact-file ESLint, `i18n:check` and `i18n:ratchet` from `apps/web`, plus every changed focused test. Run the existing PostgreSQL-backed intent conformance cases when a DSN is available; otherwise record exact skips and their unverified limits. Final selectors must include every added/changed test, including any queue-control name not matched by the block above; record the exact supplemental selector in Results. Rebuild from tested source before browser checks and retain mock-provider disclosure.

## UI-01: Completed child

```text
Parent panel / phone drawer:  Completed -> Done -> Open
Opened child:                 Back to parent
                              Session: Completed
                              Retained transcript
                              Existing explicit continuation controls
```

See [the combined preview](plan.md#ui-01-retained-completed-conversation). Existing desktop and phone composition stays intact; prove completed status, no runtime on inspection, and explicit continuation on both surfaces.

## Dependencies, parallelism and risks

Recovery acceptance and integration are complete at `67b6b9ba9d`. The reviewed regression packet is ready for the backend implementation. `sequential`: this vertical change crosses shared settlement/admission/storage boundaries and needs one production implementation owner. Follow the [design](../../specs/tasks/system-design/errand-completion-cleanup.md) and preserve exact target authority, ordinary-session behavior and provider recovery limitations.

### Recovery integration review (2026-11-10)

Era Sol Que Me Faltava reviewed the contract against `b2bbe9d5b7` in Errand `99941b7b-68b8-459c-bfdd-8a52a8d55c9c`. The contract remains compatible; the implementation must resolve these existing seams:

- SQLite currently forbids replacing even a completed teardown intent. Define conditional intent replacement for repeated completion cycles and cancellation precedence while preserving outstanding stop ownership. Cover complete/continue/complete and cancellation overtaking pending success cleanup.
- Exact terminal settlement can succeed by recording a historical receipt without changing current state. User-origin settlement can preserve an earlier parent outcome. Authorize cleanup from the exact successful decision and unchanged current ownership, never a nil settlement error or `LastOutcome`. Cover historical success beside a newer unresolved turn and opposite parent/user outcomes.
- Existing teardown applies cancellation effects, while agent completion also schedules asynchronous execution cleanup. Give successful Errands one durable close owner with distinct success projection, leaving ordinary sessions unchanged. Retry must finish projection after a crash between physical close, receipt and `COMPLETED` publication.
- Idle-focus resume recognizes idle-suspension provenance. Success cleanup must not qualify for passive resume. Coordinate explicit user/parent continuation with the recovery subtask and prove inspection starts no runtime.

Use controlled barriers for same-execution successors, replacement incarnations and delayed callbacks. Test restart and flag-off retry at settlement, intent, stop acknowledgement, receipt and projection boundaries. Mixed legacy inventory must converge eligible rows independently and preserve unknown evidence. These are implementation and verification obligations within the recorded contract, not additional product scope.

## Results

The synchronization correction is accepted as a bounded RED packet by Era Sol review `4fa0283f-f14a-437c-adca-e55ef9f0b10a`: callback-entry SQLite capture, `synctest.Wait` before stop acknowledgement, and joined/quiescent successor/cancellation assertions resolve all three earlier findings. The two expected missing-COMPLETED failures remain. This does not prove future timer-driven retry behavior or production cleanup, which is still pending.

Implementation in progress, not accepted. Lunaldinho Errand `92617298-5bb8-4a7c-8fc1-62cc9e6fa95c` completed bounded regression preparation and released ownership. Its [handoff](regression-handoff.md) records a compiling race-enabled `TestErrandCompletionCleanup` selector with two expected behavioral failures: ready does not close the runtime or project completed, and process exit closes the runtime but does not project completed. Admitted-successor and cancellation-preservation controls passed; no race report occurred. Formatting and whitespace checks passed. Parent review of the corrected tests is accepted; these RED tests do not establish production acceptance. The read-only contract review above is complete and ran no tests. The recovery dependency is now resolved; production cleanup is ready for the bounded backend packet. Current coordination is recorded in [the remaining-work plan](../errands-remaining/plan.md).

Parent lint remediation removed redundant embedded selector names only; synchronization and behavior are unchanged. Current test SHA-256: `b83d51acd269cbb8d4c6aaef364186b91b1604a78021ebdeb271344ab051aa48`. The corrected selector passed terminal-fix integration lint; the hash identifies the test before cleanup implementation.

## Current bounded backend packet

Implement backend acceptance on `67b6b9ba9d` before frontend/browser changes. Ownership is the backend files above, related admission/focus guards, adjacent tests and a new `backend-handoff.md` here. Preserve the already integrated terminal identity, attention/refusal, duration and delivery contracts. Exclude frontend, E2E and public docs in this packet; the primary owns these manifests and later browser acceptance. The backend packet remains incomplete until every required backend edge below has executable coverage or an explicitly reported blocker.

- Extend the existing versioned teardown intent with completion purpose. Missing purpose preserves legacy cancellation meaning. Reuse existing serialization, admission ownership, bounded recovery and runtime stop APIs; do not add an independent queue/coordinator.
- Authorize creation from the exact committed successful terminal decision and unchanged current ownership, including successful user no-result decisions. Historical receipts, nil settlement errors and aggregate `LastOutcome` are insufficient. Keep result delivery independent of physical cleanup.
- Same exact pending completion is idempotent. A new completion cycle may replace a terminal/superseded obligation only under version/identity checks. Cancellation must take semantic precedence over pending completion without discarding an outstanding physical-stop obligation; stale completion callbacks cannot restore successful projection after cancellation. Do not weaken cancellation lifecycle constraints or silently replace unknown pending ownership.
- Keep physical close, stop receipt and session projection distinguishable so retries converge after crashes at every boundary. Publishing `COMPLETED` requires confirmed exact close or authenticated absence and unchanged terminal ownership. Retry must also finish a projection interrupted after close/receipt, without another prompt or duplicate result.
- Cover ready/process exit, two complete/continue cycles, pending-success cancellation, historical success beside a newer unresolved turn, opposite parent/user outcomes, same-execution successor, replacement incarnation, delayed callbacks, failures at settlement/intent/stop/receipt/projection, restart with admission disabled, mixed eligible/unknown legacy rows, passive inspection and explicit parent/user continuation. Preserve ordinary-session controls.

Use the backend commands above with the shared lock and resource limits. Add the exact successful-cleanup/continued-admission tests not matched by those selectors, and retain parent terminal/recovery/duration controls affected by implementation. SQL guard and store conformance cover any changed persistence contract; record PostgreSQL skips. Run changed-file lint and build, retaining every handle. Do not rerun broad unrelated suites or weaken a failing test to obtain GREEN. If a necessary change exceeds the existing exact-authority/durable-teardown design, return the concrete boundary and evidence before expanding it.

On 2026-11-12 the user requested direct primary execution of all remaining work. The active backend Errand was cancelled through the supported API with edits preserved. The primary now owns backend completion, verification, browser checks, integration and delivery; no further implementation/review delegation is planned.
