---
id: "01-completion-cleanup"
title: "Close completed errand runtimes and preserve continuation"
status: pending
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

No predecessor. `sequential`: this vertical change crosses shared settlement/admission/storage boundaries and needs one implementation owner. The current parent must expose the native Errands capability before launching the requested worker. Follow the [design](../../specs/tasks/system-design/errand-completion-cleanup.md) and preserve exact target authority, ordinary-session behavior and provider recovery limitations.

## Results

Pending. No production changes or permanent regression tests have been written, and no implementation Errand was launched. Planning evidence and the missing live tool capability are recorded in the manifest.
