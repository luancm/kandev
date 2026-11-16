---
id: 02-lifecycle-controller
title: Strengthen lifecycle contracts and make controller capabilities explicit
status: done
wave: 2
depends_on:
  - 01-private-browser-state
plan: plan.md
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

# Task02: lifecycle contract and explicit controller

First fill the wiring gap between successful close, real parent delivery and explicit continuation. Repair the existing-contract defect exposed by that coverage as a separately reviewed phase, then declare the installed service's required lifecycle capabilities in `ErrandController` and remove optional discovery/fallback branches that have no production consumer. Keep the existing ownership and locking design. No schema, protocol, provider-admission or new product behavior is selected.

## Test phase

Use real SQLite, the real parent deliverer and queue, and a controlled runtime/provider boundary. Drive completion through subscribed events and follow-up through public `PromptTask`, with user and authenticated-parent cases. Assert durable result at stop entry, acknowledged close before `COMPLETED`, one parent delivery, retained conversation identity, only the new instruction dispatched, successful second settlement and distinct user no-result versus parent-result behavior. Keep parent/workflow state intact. Prefer existing fixtures; do not add a public production API solely for tests.

Strengthen the misleading `TestRestartDeliversAResultThatWasStillPending` with real fresh-owner/reopened persistence evidence, or rename it as reconciliation retry and add the missing restart scenario beside the contract test. Prove pending result and cleanup convergence with new admission disabled, then one delivery after re-enabling. An in-memory recorder reset is not restart proof. Return for parent inspection after the test phase before production refactoring.

Reuse `newErrandLaunchFixture` seed data, `settlementTranscript`, and existing queue/task-service adapters. Construct `NewService` with the real queue, retain its executor callbacks, and use `StartEventWatcher`/`StopEventWatcher` for the subscribed boundary. A successful launch fake must honor allocation and boot-ready callbacks and maintain an execution inventory consistent with stop acknowledgement; returning nil from `LaunchAgent` alone is insufficient. Existing `interrupted_session_restart_test.go` demonstrates that callback pattern. Parent-origin context proves trusted parent provenance, not MCP transport authentication. For reopen, drain old workers/watchers, close the pool and reconstruct repository, queue, orchestrator, Errands service and runtime boundary from the same file. This proves reopened persistence, not full backend startup, native-provider restoration or physical OS-resource release. Share construction only; keep scenario setup and assertions visible.

## Bounded repair before refactoring

The new public `PromptTask` contract is a valid RED for AC-TASKS-ERRAND-COMPLETION-001.1 and .3: a legacy-provider user follow-up records its exact successful no-result decision, yet cleanup cannot resolve its submission and leaves the runtime open. Legacy delivery intentionally creates no durable submission row. `teardownSubmission` currently considers delivery inventory, parent admission metadata and the first-turn reservation, so it misses this later user turn. Preserve the regression unchanged.

Own only the missing-identity resolution near `classifyObservedPrompt` in `apps/backend/internal/orchestrator/errand_teardown.go` and focused cleanup/teardown tests in this phase. Reuse existing exact-decision predicates/decoders where their contracts fit. If factoring a helper from `errand_completion_cleanup.go` is necessary, preserve its existing callers and checks. No store/query/schema change, new coordinator, manufactured ID, provider parsing change or controller refactor belongs to this phase.

After a successful submission read with no selected identity, resolve from the exact completed successful decision only when the verified completed current turn, latest settled turn and persisted target agree, including the decision's captured execution. Do not replace any nonempty conflicting identity or mask inventory/receipt read errors. Preserve all existing incarnation, successor, operation-ownership, acknowledgement and post-stop checks. Cancellation inheriting the same pending completion target also needs this identity resolution while preserving its cancellation outcome. The [completion design](../../specs/tasks/system-design/errand-completion-cleanup.md#durable-close-authority) owns these conditions.

Use focused cases named `TestErrandTeardownLegacyCompletedIdentity...` for missing/unsealed/failed/cancelled/conflicting decisions, incomplete/current-versus-historical turn, read errors and conflicting durable submission. Prove cancellation overtaking the same pending completion target can still close exactly once without restoring success. Retain existing successor, replacement execution/incarnation, blocked-stop, receipt-failure and projection-failure controls; extend only where those tests do not exercise the new resolution. A successful result or `LastOutcome` without the exact decision is insufficient. The unchanged public continuation test must turn GREEN. Return for acceptance of this bounded repair before starting the capability refactor.

This is the separate repair decision required by the test-phase handoff. It is recorded here to keep one lifecycle work order rather than adding another package for the same contract. Test evidence and repair acceptance remain distinct from controller-refactor acceptance.

## Refactor phase

Own `internal/orchestrator/errand_controller.go`, optional capability use in `errand_settlement.go`, `errand_terminal_admission.go`, `errand_startup.go`, `ceiling_sweep.go` and `errand_hooks.go`, the corresponding completed-event fallback in `event_handlers_agent.go`, and affected test doubles. The production installation in `internal/backendapp/errands_wiring.go` remains the real service. Required method signatures come from that implementation; do not add speculative methods or another interface layer. Preserve nil/unwired behavior and feature-disabled cleanup distinctly. Keep service/repository revalidation, physical stop ownership and all fault/race assertions. Replace only tests whose purpose is support for an unreal partial installed controller. Composition, admission and profile interfaces remain outside this phase.

Own direct contract tests and this work order's Results. Parent owns durable design/doc updates. Any proposed persistence change or newly found behavior defect requires a separate scoped decision rather than hiding it in this refactor.

The source-comment audit permits three adjacent comment-only corrections in this phase: describe `TeardownIntentStore` in `errands/interfaces.go` as current conditional persistence rather than an unfinished migration; describe the completed-event branch by its Errand/deferred-workflow invariant rather than “pre-fix behavior”; and replace the AC-ID comment near `orchestrator/service.go` task-review recovery with the actual ordinary-session/unarchived/not-retracked conditions. Do not change those implementations or weaken their guards.

## Acceptance

1. The new boundary scenario and accurately named restart coverage protect the missing wiring before the production change. Retained tests still detect ordering, successor, cancellation and receipt/projection failures.
2. Installed lifecycle capabilities are compile-time requirements with no fixture-only optional fallback; nil/unwired ordinary sessions and flag-off retained Errands keep their behavior.
3. Focused race tests and affected-package compile/lint pass. No new coordinator, production test seam or removed identity/transaction fence.

## Verification

Run sequentially from `apps/backend`, capturing the baseline before edits and final results afterward:

```bash
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run '^Test(ErrandCompletedConversationContract|ErrandTeardownLegacyCompletedIdentity|ErrandCompletionCleanup|ErrandContinuation|ErrandReady|ErrandSettlement|ParentTerminalIdentityAutoRunsEmptyErrandNotificationAfterLifecycleRepair|ErrandTeardownServiceCrashAfterStopBeforeReceipt|StaleErrandTeardownServiceRecoveryPreservesSuccessor|SettlementHooks)' -count=1 -v
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run '^Test(ParentInitiatedTurnCapturesItsFinalMessageAsOneResult|UserTurnDoesNotProduceAResult|ReconcileRetriesPendingResultNotification|ErrandCompletionCleanupRecoversIntentFromSealedTerminal)$' -count=1 -v
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -trimpath -p 1 ./internal/backendapp ./internal/orchestrator ./internal/errands -run '^$'
```

Run `gofmt`, `git diff --check`, and `bash scripts/lint-go-changed` from the repository root with the exact changed Go paths, under the same lock. Extend the selector to the actual new restart name if needed; report matched counts and skips. No persistence query/schema change is planned, so prior conformance is retained rather than claimed as a fresh run. A real persistence change would require SQL guard and both-engine conformance.

## Results

Accepted and delivered after three separately reviewed phases. [The exact completed Results](evidence-index.md#task-02-lifecycle-controller) preserve every selector, source manifest, RED/intermediate failure, timeout, final command and ownership receipt.

Public real-SQLite continuation/reopened-owner tests exposed a supported legacy-provider user-follow-up defect: exact successful terminal settlement existed but cleanup could not resolve the retained submission. RED passed 27/28 top-level cases (58/60 entries); trusted-parent continuation and reopened-owner recovery passed while the user case failed. Tests use controlled authenticated provider/runtime boundaries; they do not certify MCP transport authentication, every provider's restoration or physical OS-resource release.

The accepted 21-line repair resolves a missing submission only from the matching committed successful decision after a successful empty inventory read. Exact task/session/incarnation/turn/submission/execution and latest-settled/teardown identity checks remain; conflicts/read errors never authorize close. Cancellation inherits the pending target while keeping cancellation precedence. Corrected repair RED exposed receipt-read/cancellation failures; GREEN passed 54 orchestrator cases (135 entries) plus four Errands controls under race checks. The public user-continuation oracle stayed unchanged; review found no issues.

The controller refactor requires all eight installed lifecycle methods and removes optional capability probes. Nil/unwired behavior, flag-off cleanup, error warnings and exact-target guards remain. Only obsolete missing-capability fixtures changed; distinct public, race, reopen, receipt-failure, cancellation and successor oracles remain. Final race checks passed 98 orchestrator cases (197 entries) plus four Errands controls, with no skips/race warnings; normal optimized compilation passed three dependent packages. Five-minute lint wrappers timed out and are not passes; identical scope/base/configuration passed with only a 15-minute allowance, zero issues. Initial mutex-copy fixture lint and a conservative cache-ownership guard failure remain in the original evidence.

The repair added 21 production lines; the controller alone removed 38 net lines (combined net minus 17 across ten production files). Accepted source/contract/frontend manifests stayed unchanged, review found no findings, and owners released processes/locks. PostgreSQL and provider limits remain those of the original scoped receipts. The [package plan](plan.md) owns delivery and later integrated acceptance.
