---
id: 06-prompt-outcome-owner
title: Resolve prompt admission evidence once inside orchestration
status: done
wave: 6
depends_on:
  - 05-definite-non-delivery
plan: plan.md
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.1
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.3
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.4
  - AC-PLATFORM-DELIVERY-RETENTION-001.3
  - AC-TASKS-ERRANDS-002.7
  - AC-TASKS-ERRANDS-002.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
system_design:
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task06: one prompt outcome owner inside orchestration

The repaired Task05 flow still requires prepared dispatch, initial callbacks, finalization and reserved-turn rollback to interpret overlapping booleans and joined errors. Its eight failed uncertainty controls during integration showed the maintenance cost: one cleanup path acted before another applied the uncertainty guard. This is an internal ownership refactor of the accepted behavior, not a newly demonstrated bug or a change to recovery authority.

## Selected design

Deepen the existing `promptDispatchOutcome` into one in-process record per prompt attempt. Preserve the existing protocol and persistence evidence producers. Represent pending evidence, rejection before dispatch, committed non-delivery, observed acceptance and unresolved delivery explicitly. Keep admission-bookkeeping failures and publication errors separate from the evidence state. A committed non-delivery receipt whose parent-refusal write fails is not equivalent to unknown transport delivery; existing cleanup stays fenced until its required bookkeeping succeeds.

Only an admission callback can establish pre-dispatch rejection. Only successful exact conditional settlement or an exact retained receipt read can establish committed non-delivery. Only the provider acceptance callback can establish acceptance, which later publication, transport or decision-write failure cannot revoke. Missing or conflicting evidence, attempted writes, failed conditional settlement and existing uncertainty guards retain unresolved ownership. A generic error setter must not infer evidence or authorize rollback.

For initial prompts, `SessionManager.sendPrompt` and `settleUnsentInitialDelivery` remain the receipt producers before failure notification. The cause-free `OnInitialPromptFailed` remains a notification; the orchestrator reads the receipt against its captured prepared submission. `OnInitialPromptAccepted` remains the positive acceptance producer. Initial and fallback attempts keep distinct captured owners and outcome instances. Preserve `DetachedResumeAccepted`, `ErrandParentPromptUncertain` and other externally consumed error behavior.

Migrate follow-up finalization first, then initial orchestration callbacks. Cleanup consumes the resolved record under its existing cancellation, successor and reservation guards. Remove superseded internal classification and duplicate acceptance storage after all real callers migrate. Do not introduce a second outcome coordinator, durable state, generic framework or compatibility wrapper for unused private paths. Do not merge result settlement, delivery receipt and physical close.

Preserve the existing non-durable ordinary-session failure policy, including detached clarification restoration after an unaccepted generic dispatch error. That policy is not proof of non-delivery. Resolve its rollback eligibility separately from transport evidence at the same outcome boundary, then pass that eligibility to cleanup; never extend it to Errand or durable unresolved attempts. Accepted work, publication failures, explicit uncertainty and existing cancellation/successor guards remain controlling. Replay-payload retention spans fallback attempts and keeps its independent lifetime while observing the resolved acceptance.

For compatibility, ordinary failure rollback requires the negotiated legacy delivery protocol as well as a non-Errand session with no prepared durable submission or parent reservation. A durable-protocol attempt whose preparation fails before a submission object exists remains unresolved. An initial pre-dispatch rejection remains eligible for guarded cleanup when its required Refused bookkeeping fails; the cause-free failure notification still attempts the existing bounded Unknown decision and retains its error diagnostically. This eligibility does not assert that a durable reservation was cleared or authorize replay. Successful Refused decisions remain terminal under their existing decision guard.

Keeping the repaired implementation has the lowest immediate migration risk but preserves the demonstrated multi-caller obligation. A runtime or database redesign would widen this packet without reducing that obligation. This design keeps evidence at its current authority and makes the downstream interpretation explicit once.

## Ownership

Own the existing outcome definition and affected prompt finalization/rollback sections in `apps/backend/internal/orchestrator/task_operations.go`, `agent_delivery_prepared_prompt.go`, `undelivered_prompt.go`, their actual internal consumers and focused tests. A small adjacent module for the existing outcome is allowed if it keeps this large file readable. Preserve unrelated ordinary-session paths. Runtime/executor callback signatures, persistence queries/schema, browser code and public contracts are outside this packet; report a concrete need before widening them.

Own this Results section and one concise receipt containing source paths/hashes, exact commands, counts and limitations. The parent owns the package manifest, quality roadmap, design decisions, integration and Git. You are not alone in the checkout; preserve parent documentation and all other work. No staging, commits, pushes, live provider/database operations or child delegation. One writer owns the shared checkout including documentation.

## Acceptance

1. Initial and follow-up orchestration have one attempt outcome owner. Downstream cleanup no longer independently infers acceptance, committed refusal or uncertainty from joined error chains. Required bookkeeping and publication failure remain distinguishable, and migrated private classifiers/duplicate fields are removed rather than left as parallel paths.
2. Existing public-boundary oracles retain exact refusal, no replay, preserved accepted work, ambiguous-outcome protection, prior results/claims and successor/cancellation ownership. Add a missing boundary test only where this migration exposes a coverage gap; do not replace those oracles with enum/helper tests. A behavior-preserving refactor may use the retained Task05 RED evidence and a fresh unchanged baseline rather than manufacture a new failure.
3. Focused race tests, normal optimized compilation and scoped Go lint pass with exact matched counts and limits. No production browser or database contract changes are claimed. Final source review checks the removal map and unchanged authority, not merely line count.

## Verification

Before editing, capture the current focused baseline. From `apps/backend`, using the established pinned toolchain and private caches:

```bash
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 -gcflags=github.com/kandev/kandev/internal/orchestrator=-l ./internal/orchestrator -run '^(TestErrandParentPromptDefinitelyUnsent|TestErrandParentUnsentPromptPreservesReusedTurn|TestErrandParentUnsentFailureFences|TestErrandParentPromptPreparationRejectsStaleOwner|TestErrandInitialFailureUsesCommittedNonDelivery|TestErrandUnsentAdmissionRecoveryReopensAfterRetention|TestErrandUnsentAdmissionRecoveryKeepsConflictingOwner|TestSendNowCreatedClaimWaitsForPreparedAdmission|TestErrandCompletedConversationContract|TestResumeAttempt_AcceptedPublicationFailureDoesNotCleanup|TestAcceptedReservationStaysActiveWhenPublicationWriteFails|TestResumeDetachedClarificationReportsAcceptedPublicationFailure)$' -count=1 -v
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -trimpath -p 1 ./internal/orchestrator ./internal/backendapp -run '^$' -count=1
```

The first command retains the previously documented orchestrator-only race inlining accommodation; the second uses normal optimization. Repeat the focused selector after migration, add actual affected ordinary controls when caller changes justify them, and report zero-match packages or skipped cases explicitly. Existing Task05 SQLite/PostgreSQL receipt evidence remains prior evidence; no new both-engine claim follows from this orchestrator-only packet. A persistence change requires a separate scoped decision and its SQL guard/conformance checks.

Run `gofmt`, `git diff --check` and repository-root `bash scripts/lint-go-changed <exact changed Go paths>` under the same lock. Preserve every process handle until terminal. Record baseline failures separately; do not mask them by weakening assertions or silently expanding scope. Check disk before and after heavy work and remove only proven inactive owned regenerable output. Keep frozen browser/performance workloads and limits unchanged; no unrelated matrix rerun is required for this backend-only internal refactor.

## Results

The unchanged pre-edit work-order race baseline passed 12 top-level tests / 47 entries with zero failures or skips; normal optimized compilation passed for orchestrator and backendapp. Final focused race verification passed 19 top-level tests / 66 entries with zero skips or failures, including the ordinary detached clarification restoration oracle, legacy-versus-V1 rollback control, initial Unknown-notification boundary, SQLite-triggered pre-dispatch cleanup/replay boundary, Task05 refusal/uncertainty controls, Send Now, accepted-publication fences, and real admission consumers. Final optimized compilation passed for `internal/orchestrator` and `internal/backendapp` with no tests selected. Receipt root: `/tmp/kandev-task06-outcome.buplr3bh/`; baseline handle 41408, final race handle 54943, optimized compile handle 64552.

Scoped Go lint passed for `./internal/orchestrator` against `f52fbebd9b452e610f63734586cfa85d3424d647` with concurrency 2 and a 10-minute timeout; the repository wrapper's two 5-minute attempts timed out during package loading, so the same package/base/rules were rerun directly with the longer timeout. `gofmt -l` returned no paths and `git diff --check` passed. Failed compile/assertion attempts and both lint timeouts are retained in the receipt directory; they are superseded by the final green checks. No PostgreSQL, browser, or runtime claims are made.

Parent delivery check: the first normal commit hook corrected four field-alignment spaces in the new outcome file, despite the writer's earlier empty formatting report, and its five-minute lint package load timed out. The parent verified the hook delta is whitespace only. All fourteen owned Go files subsequently passed `gofmt -l`, and identical-scope post-format lint passed with the ten-minute allowance. The failed attempt is retained at `/tmp/kandev-roadmap-delivery-20261115.8BBoWfDc/task06-commit.log`; normal-hook retry and Git delivery remain parent-owned. Post-format source manifest: `/tmp/kandev-task06-outcome.buplr3bh/posthook-source.sha256`.

Changed production Go files total 13,118 lines before and 13,214 after (+96). The removed parallel paths are `errPromptAdmissionRejected` / `promptAdmissionRejection` / `markPromptAdmissionRejected`, `wrapAcceptedPromptDispatchFailure`, `settlePromptDispatchError`, `promptClaimRollback.reservedTurnAccepted`, the old accepted-only `promptDispatchOutcome`, and an unused write-only errand acceptance atomic. The new attempt record owns evidence and keeps admission/publication errors separate; operation-lifetime replay retention remains separately named `retainPromptPayload`. Final source manifest and per-file hashes: `/tmp/kandev-task06-outcome.buplr3bh/luna-final-source.sha256`.
