---
id: "01-recovery-settlement"
title: "Preserve exact settlement across recovery"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-002.4
  - AC-TASKS-ERRANDS-002.5
  - AC-TASKS-ERRANDS-002.6
  - AC-TASKS-ERRANDS-002.7
  - AC-TASKS-ERRANDS-002.8
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.6
  - AC-TASKS-ERRANDS-003.7
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
---

# Task 01: Preserve exact settlement across recovery

## Summary

Recovery status/continuation checkpoint (2026-11-11): AC002.4–002.8 implementation and scoped acceptance are complete, including real desktop/phone admission and passthrough refusal coverage. Era Sol reviews found no remaining actionable findings. See [accepted checkpoint](recovery-status-checkpoint.md) for the acceptance map and verification limits. This work order remains `in_progress` only for delivery and integration of the parent-owned terminal-identity regression correction; historical live repair eligibility is not claimed. Earlier summaries below record prior checkpoints.

Reproduce the captured orphan-settlement gap, then connect authoritative recovery decisions to exact Errand settlement. Ensure eligible stranded records converge without synthesizing successful completion or replaying an uncertain prompt.

## In scope

- Exact, durable recovery interruption decisions and recovery-generated turn classification.
- Idempotent reconciliation of historical records with sufficient ownership evidence.
- One result and consistent status across parent-facing projection and delivery.

## Out of scope

Live runtime/database changes, the staged upstream merge, generic timeout policy, and successful-session cleanup.

## Acceptance

1. The captured restart sequence fails before the patch and settles the exact interrupted delegation once afterward, preserving workflow isolation and uncertainty.
2. Valid mixed evidence settles only proven identities; missing, stale-generation or contradictory identities never permit successor admission or provider replay. Cancellation and later results cannot be overwritten.
3. Persistence failures and repeated restart are retryable. Recovery-only turns cannot strand unrelated proven settlements or be confused with genuine agent work.

## Verification

Run from repository root with heavy checks serialized:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 -race ./internal/orchestrator -run 'TestErrandRestartAbandonmentSettlesExactInterruptedTurn')
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 -race ./internal/errands ./internal/task/repository/sqlite -run 'Errand.*(Settlement|Terminal|Recovery)|Reconcile|AbandonTurn')
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 ./internal/orchestrator -run 'Errand|ReconcileRetracked|AbandonOpenTurns')
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 -v ./internal/task/repository/sqlite -run 'TestPostgres.*Errand.*(Settlement|Terminal|Recovery)')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/errand-orphan-settlement
```

Run `bash scripts/lint-go-changed` with the exact implementation-owned Go paths after the patch; do not select all staged upstream changes. Record the final invocation and results here. Run available PostgreSQL conformance cases for changed repository operations and report unavailable-DSN cases as skips.

## Files likely touched

- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/errands_turn_settlement.go`
- `apps/backend/internal/task/repository/sqlite/errands_turn_terminal_evidence.go`
- `apps/backend/internal/task/repository/sqlite/errands_turn_settlement_test.go`
- `apps/backend/internal/errands/terminal_decisions.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/errand_recovery_interruption_test.go` (new)

## Dependencies

No prerequisite work order. The staged merge exists only in the parent checkout and is read-only compatibility evidence. Preserve its foreground-idle submission behavior without importing the merge. Primary session owns integration; assign explicit disjoint ownership before any supporting Errand.

## Risks

Do not infer terminal provider output from session state or zero-duration timestamps. Audit recovery-created turns before introducing lifecycle-only classification. Do not weaken the all-preceding-decisions admission fence to make this test pass.

## Parallelism

`sequential`

## Inputs

[Incident evidence](evidence.md), [plan](plan.md), and their linked Errands requirements and designs.

## Results

Slice 1 accepted by primary after source inspection and independent focused race verification. Historical receipt commits preserve the Errand row, current result authority and successor fence. See [historical receipts handoff](historical-receipts-handoff.md) for the complete eight-file write set, corrected RED/GREEN, combined race checks, store conformance, SQL guard and passing changed-Go lint. PostgreSQL fixtures were selected but skipped because KANDEV_TEST_POSTGRES_DSN is unset.

Primary command from apps/backend: `GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 -race ./internal/errands ./internal/task/repository/sqlite ./internal/orchestrator -run 'ErrandHistoricalReceipt|ErrandRestartAbandonmentSettlesExactInterruptedTurn|ErrandExactTerminalDecisionPreexistingSuccessor' -count=1`, handle 96826, exit 0 (Errands 1.349s, SQLite 5.452s, orchestrator 1.813s).

The broader orchestrator selection fails two established/claim cases of TestSendNowErrandCanonicalPersistenceFailurePreventsProviderIO (legacy and durable_v1, expected two queue entries, got one). The child reproduced both against HEAD using a Go overlay. Primary inspected the overlay and byte-verified all restored production files against HEAD. These remain recorded baseline failures, not a green broader suite.

Slice 2 continuation is implemented and ready for acceptance. Its 22-file write set, effect format, producer/consumer/retry wiring and RED/GREEN evidence are in [recovery proof handoff](recovery-proof-handoff.md). The real admitted-submission/original-process-death/native-recovery test yields an interrupted stored result and exactly one notification request; actual parent-provider delivery is still slice 3. Final-source lifecycle/SQLite race (70586), stable cross-package race (97602), changed-Go lint groups (74078/75868/11813), SQL guard (13423) and store conformance (89777) pass. PostgreSQL fixtures (46038) skip for missing DSN. Broad orchestrator (80770) retains only the two documented baseline Send Now failures.

Primary independently confirmed the Errand session is done and inspected the proof model, repository transaction and lifecycle/retry adapters. A focused read-only authority audit precedes acceptance and slice 3. The work order remains in progress. Disk is down to 2.8 GiB free; no new heavy builds until headroom is established. Large inspected temporary directories belong to other tasks or lack proof of ownership, so none was deleted.

The following scoped lifecycle command supplements the original gates; final checks must include all new/changed test names from the handoff:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go test -trimpath -p 1 -race ./internal/agent/runtime/lifecycle -run 'InterruptedDelegation|AlreadyResumed|LoadedNative|InterruptedSubmission|PromptTurnBinding' -count=1)
```

Confirm this selection includes every changed regression; report absent cases rather than treating no tests as a pass. Any changed runtime/executor/backendapp plumbing must compile and run its affected focused tests too. No production or test changes from slice 1 may be discarded to implement slice 2.

## Execution constraints

Check free disk before and after heavy verification; serialize builds and retain/poll every process handle. The PostgreSQL command must select actual conformance tests for the changed operation; add parity cases if absent and record their exact names. No selected tests is not a pass. Missing `KANDEV_TEST_POSTGRES_DSN` is an explicit skip. Install dependencies from `apps/` only if web tools or hooks require them. No live repair, cancellation, restart, configuration mutation or deployment is authorized.

## Sequential implementation slices

1. Historical receipt convergence: correct the later-unclassified regression to assert an exact completed receipt and unchanged Errand/result fields; keep its successor blocked. Implement independent proven-candidate processing, sealing of existing intent and receipt-only commit. Preserve the current result-writing API's pre-existing-successor refusal. Verify cancellation, newer result, exact identity, rollback and PostgreSQL parity. This slice uses already-authoritative terminal evidence and must not invent recovery proof.
2. Exact recovery assessment: carry original admitted turn/owner/native identity through the lifecycle proof paths and fresh-generation cleanup; persist separate retryable assessment before losing source evidence, validate recovered inventory, then seal interruption with SQL uncertainty intact. Read the accepted recovery-authority ADR and extend this work order with exact changed lifecycle test names before running them.
3. Generated-turn attribution and integrated acceptance: preserve exact work attribution and typed lifecycle-only provenance, then prove actual admission through owner proof, recovery, result projection and one parent delivery. Historical data without sufficient proof remains fenced. Run the complete work-order gates after integration.

The primary session owns durable contracts and accepts each slice before the next writer starts. The first slice's GREEN is not completion of this work order or a claim that the historical incident is repaired.

## Slice 2 audit remediation

Read-only audit b9251920-2866-4774-951e-9c2858f5df77 found two blockers, confirmed by primary source inspection. Slice 2 is not accepted until both are fixed with RED/GREEN evidence:

- PreserveErrandInterruptedDelegation returns nil on OriginalOwnerTerminated operational errors, allowing its caller to delete the original running row or replace its binding. Propagate operational inspection failures before destructive cleanup; prove the original evidence/retirement state survives and retry succeeds. Do not treat unsupported historical proof as successful termination.
- A real terminal arriving after assessment persistence leaves the assessment pending, so ReconcileTerminalDecisions retains a recovery error even after committing the real terminal receipt. Resolve only the exact superseded assessment once authoritative terminal settlement is established under the owning locks. Preserve the actual outcome; verify repeated reconciliation and successor admission with the production recovery callback wired. Ambiguous or mismatched terminal evidence must still fence.

The repair Errand owns only this remediation and its progress file recovery-audit-remediation.md. Previous implementation/check receipts remain historical evidence, not acceptance of these paths. Slice 3 waits. Tests may use task-owned executable RAM scratch at /dev/shm/kandev-errand-remediation-0n6s5t7i via GOTMPDIR, with serialized checks and disk/memory monitoring; root currently has 2.8 GiB free, and shared caches/other tasks' scratch must not be deleted.

Disk cleanup requested by the user during remediation: primary removed 11 abandoned Go build/link temporary directories older than 24 hours, after checking process references twice. Reclaimed 5.72 GiB; root now has 7.5 GiB free. Active RAM scratch, source worktrees, databases and shared caches were preserved. This supersedes the earlier disk-headroom measurement.

## Slice 2 acceptance and slice 3 dispatch

Both audit blockers are repaired. Primary inspected supersession identity/authority validation and operational-error propagation and independently ran `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 -race ./internal/agent/runtime/lifecycle -run 'TestErrandInterruptedDelegation(TerminalEvidenceWins|InspectionFailurePreservesBinding)' -count=1` from apps/backend: handle 25973, exit 0, 4.518s. The EMFILE subprocess test covers real inspection failure through replacement persistence and retry; supersession coverage includes unknown/adopted terminal, existing committed receipt, mismatch/ambiguity, rollback, cancellation, repeated reconciliation and successor reservation. Child final race/lint/SQL-guard evidence is in recovery-audit-remediation.md; PostgreSQL remains skipped for missing DSN. Slice 2 is accepted, with actual parent-provider delivery and historical incident eligibility still unproven.

Slice 3 owns exact immutable event-to-turn attribution for Errands, typed lifecycle-only activity, and integrated delivery through the real parent deliverer/queue/admission path to a fake external provider boundary. Do not use a fake ParentDeliverer as the acceptance endpoint. Preserve real unowned native work, reject conflicting identities and keep replay off a mutable unrelated open turn. Warning text alone must never classify historical turns as lifecycle-only. Add producer/SQLite regressions for late original output while a successor exists, cross-session/generation/submission mismatch, truly unowned work, typed status versus identical ordinary text and replay idempotency. Prove one interruption block reaches the captured parent provider request without replaying child work or affecting task workflow. New tests must be named and included in the final focused commands, alongside prior regression gates. No live provider or runtime mutation is required or authorized.

Root headroom is now 3.6 GiB after remediation. Continue using the task-owned executable RAM scratch, serialized checks, bounded memory and the 1.5 GiB root stop threshold. Slice 3 must report its own exact file/check evidence before final work-order acceptance.

## Recovery attention and safe continuation correction

Warning-stream Errand 3dc0a4d0-8c21-4a02-aa8c-9a3f92ffbafe was cancelled, independently confirmed at 2026-11-10 06:53 UTC. Partial real-handler tests and their service-backed fixture remain preserved; its handoff contains no completed-check evidence. Primary found no task-owned build/lint processes and started focused race verification as handle 42492 before releasing the worktree. Backend recovery-attention/receipt-guard/explicit-resume implementation is next, with no concurrent writer. Root headroom remains 28 GiB; cancellation is not treated as test completion.

The status/resume investigation (73329a72-016a-4aa6-ba04-b870029ac49e) found a concrete receipt-writer reservation bypass and insufficient status inputs. Primary confirmed both receipt turn writers omit the ordinary reservation guard and that SQL live counts miss unresolved follow-ups after an older result. The task system owns this correction because it owns Errand status, admission and capacity. REQ-TASKS-ERRANDS-002 moved without renumbering to [status and continuation requirements](../../specs/tasks/requirements/errand-status-and-continuation.md), freeing the nearly full Errands requirement. Its [design](../../specs/tasks/system-design/errand-status-and-continuation.md) owns evidence-aware attention, reason propagation, count parity, guard ordering and existing desktop/phone surfaces. AC002.4–002.8 define the additional outcomes. This refines the user's requested behavior; it does not authorize uncertain replay or a new terminal state.

After the active warning-stream writer releases, delegate sequentially: (1) backend projection evidence/reason propagation and count parity, both conversation-receipt guards and safe explicit continuation/restoration regressions; (2) frontend reason/explanation/refusal handling with all translations and desktop/mobile tests. One writer at a time. Backend scope includes errands, task models/repository/DTO/service, orchestrator and required MCP/error projections; frontend scope includes existing Errands row/opened-child surfaces, shared types/state and focused E2E. Primary owns specifications and acceptance. No unnecessary new UI or schema migration. Ordinary startup/active reconnection/terminal reconciliation must not be mislabeled; test errors and prior/newer-result preservation. Rejected continuation must produce no turn, revision, start event or provider request. Use the accepted receipt fence for proven interruption and reject unsupported/uncertain restoration.

Desktop and phone reuse the shipped Errands row → Open → child conversation, with the existing desktop popover and mobile drawer/full-screen child/Back composition. Add recovery explanation and an accurate refusal beside existing controls, not a new modal or action. Keep shared state, existing scroll owner, touch/focus and safe-area behavior. Run focused rendered/E2E checks on both layouts; preserve zero horizontal overflow and no hover-only access. This supersedes the earlier blanket no-UI scope only for the necessary content/status correction. Final acceptance also requires the outstanding warning-stream result, scoped checks, honest PostgreSQL skips and branch handoff.

Provider-boundary acceptance is implemented by Errand 30018aa5-a6f8-4d7d-878d-563506d9de49 and accepted after primary source inspection and independent race: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/backendapp -run '^TestErrandRecoveryActualProviderDelivery$' -count=1`, handle 76902, exit 0, 1.708s. Production-driven child follow-up dispatch, owned-process death, authenticated native recovery, exact interrupted settlement, persistent not-ready parent queue and real parent provider capture are covered, including exact revision/full block/no duplicates/no replay/unchanged workflow. Initial Errand creation and nonempty recovery output are outside this fixture; details in parent-delivery-handoff.md. Two test files only, no production fix needed. Remaining work is uncertain-status/explicit-resume contract and behavior, full warning-stream coverage and final checks. AC002.1 currently restricts needs_attention to active permission requests, so primary must reconcile that contract before changing public status semantics.

Shell remediation 20683b7d-49af-4bf0-b92c-6652df4b631b is accepted after primary source inspection and independent race handle 63424: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite -run '^TestErrandShell(Ownership|SourceEvidence)$' -count=1`, exit 0, 3.167s. The audit P1 is resolved by turn/parent-tool scoping and retained unowned-source correlation; exact implementation/checks are in shell-ownership-handoff.md. Consumer ownership is accepted within tested boundaries. The owning design and ADR now record the separate admission and unowned-tool-source effects and their session-owned lifetime. Parent-delivery acceptance remains unimplemented and is the next sole-writer assignment, using the concrete backendapp fixture path already documented; no further investigation-only handoff counts as completion. Public uncertain-status/explicit-resume and full stream-warning acceptance remain outstanding.

Parent-delivery Errand 25423afb-93ec-4c47-bb74-76f6c5b13b7e returned investigation only and released ownership; no regression, tests or concrete blocker. The backendapp fixture path documented in parent-delivery-handoff.md is the next implementation basis, not acceptance evidence. Proceed first with the confirmed shell-owner P1: sole writer owns SQLite shell start/output/lookup, necessary immutable source-correlation helpers and focused real inbox/canonical projection tests. Preserve ordinary-session behavior and prove admitted/unowned tool-ID reuse cannot mutate another turn. Then finish the documented real child-dispatch/parent-provider fixture; full task remains in progress.

Consumer audit 9c7d208f-22f7-4df3-a625-e4158a481b6e found one blocking P1, independently confirmed by primary source inspection. `persistCanonicalShellStartTx` and `persistCanonicalShellOutputTx` use `loadShellOutputMessageTx(sessionID, toolID)` without the immutable resolved turn. Reusing a tool ID in later admitted/unowned work can reuse A's message, and a base-zero checkpoint invokes `reconcileCanonicalShellIdentityTx` to replace A's output. Consumer acceptance is blocked until lookups, creation identity and updates respect proven owner boundaries. Add real inbox/canonical projection regressions for completed A and later admitted/unowned B sharing a tool ID, unchanged A after B checkpoint, delayed A output, replay equivalence and ordinary-session compatibility; unowned tool correlation must use source provenance rather than session-wide fallback. Remediation is queued after integration Errand 25423afb-93ec-4c47-bb74-76f6c5b13b7e releases the worktree. No second writer is permitted. The audit found no other concrete blocker; PostgreSQL and final parent-provider/public-state acceptance remain separately unproven.

Consumer continuation d2d37518-3881-40b1-8abf-1dd014364e46 implemented immutable admission ownership and released writer ownership. See consumer-ownership-handoff.md for its exact 13-Go-file scope, typed sequence-zero admission effect, retention and native-identity validation, callback/completion guards, RED/GREEN and final checks. Primary inspected the principal seams and independently ran `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'TestErrandAdmissionOwnership|TestErrandRetainedAdmission|TestRetainedTerminalCannotClaim' -count=1`: handle 14725, exit 0, SQLite 1.503s and lifecycle 1.683s. Focused read-only boundary audit precedes final consumer acceptance. Actual child-admission-to-parent-provider integration proceeds with one writer in new test files and narrowly necessary non-consumer wiring; consumer production files remain frozen while audited. Public uncertain-status/explicit-resume behavior, full stream warning coverage and contract reconciliation remain outstanding. Root has 32 GiB free; no primary checks remain running.

Continuation c805a140-946d-4b42-a4ff-8d903b9f8e61 finished incomplete with ownership released. Valid producer forwarding mutation RED and final focused producer/orchestrator race coverage pass; lint timed out twice and remains unverified. Two assertion-quality consumer probes demonstrate old-submission/current-turn fallback and explicit conflicting turn acceptance. Next implementation Errand is narrowed to immutable consumer ownership only; actual child-dispatch-to-parent-provider acceptance follows separately. General admitted prompt ownership must be retained independently of interrupted-delegation assessments, before successful replacement as well as recovery; saved binding alone never proves death. Primary accepts atomic retention at the existing running-row/admission transaction as an implementation seam, subject to exact identity validation and rollback tests. API propagation was inspected with no defect established. Preserve genuinely unowned native activity without assigning it to unrelated open work. A final integration fixture must use actual child dispatch and real parent delivery, since the existing recovery fixture seeds SQL submission transitions. Public uncertain-status and explicit-resume behavior remain an identified acceptance gap to resolve after backend ownership; no frontend redesign is delegated yet.

Restart continuation at 23:16 UTC: user explicitly authorized replacing the stuck implementation Errand. Primary verified c6663ad7-973c-45e1-9f38-ce0a9a67a152 has a stopped executor with zero PIDs and unchanged last activity, then cancelled it through the supported Errand API. Existing source/tests are preserved; producer code now includes an identity-error sentinel and forwarding rejection guard, but no final acceptance exists. Root has 47 GiB free. Replacement owns the prior slice scope plus narrowly necessary agentctl/server/api event-identity propagation and focused tests if inspection establishes that boundary is required; read scoped API guidance first. This scope extension does not authorize protocol/schema redesign. Complete immutable admission/inbox/projection identity and real parent-provider delivery, then report restart/interrupted presentation and explicit-resume behavior against the accepted contract. Do not infer a new frontend redesign from conversational clarification; report any remaining product gap to primary. No direct database repair, original-incident cancellation or deployment is authorized.

Resource blocker resolved on the next continuation: the user approved pruning shared Go build-cache entries whose access/modification times are over 24 hours old, excluding process-referenced files. Before deletion, primary found root already at 50 GiB free, the cache at 12 KiB and zero eligible entries. Space was recovered outside this session; primary deleted nothing. Resume slice 3 with serialized checks and expect cold-cache rebuilds. The successor-cancellation defect, consumer ownership validation and actual parent-provider acceptance remain outstanding.

Latest continuation b7bf04de-5ae0-45ad-9057-a55cb4aa74bb is done but incomplete. Producer retained-history attribution and separate typed warning turns have focused race coverage; neither is finally accepted. Primary inspected the diff and confirmed the session is done. The new conflict rejection can enter the generic storage-failure branch and cancel a successor; its forwarding probe panicked due to an incomplete fixture and is not valid RED evidence. Repair/test that path, finish consumer identity validation and actual parent-provider acceptance, then run remaining gates. Exact changes and handles are in turn-attribution-handoff.md. Root is now 1.2 GiB free and heavy verification is blocked. No large task-owned scratch remains; shared Go cache is 48 GiB, including 6.71 GiB of entries whose access and modification timestamps are over 24 hours old. Primary is requesting authorization to prune only those regenerable old cache entries because original cleanup scope was owned scratch only. No cache deletion has occurred.

Errand 78837b97-bb68-4cc8-9970-436323b15079 stopped without production changes after reproducing an attribution defect outside its initial ownership. Primary confirmed the session is done and independently reran both retained-output probes plus the existing terminal control using the handoff overlay: handle 30932, exit 1, 0.038s. Both probes fail because late A output after completion acquires active B's submission ID, including real journal append/replay. The existing terminal control passes. See [turn attribution handoff](turn-attribution-handoff.md) for exact source and commands.

Primary expands slice 3 ownership to `apps/backend/internal/agentctl/server/process/manager.go` and focused producer tests, in addition to its existing consumer/integration scope. `deliverySubmissionIDForEvent` deletes the active generation mapping on terminal and then falls back to the active submission without consulting retained generation history. Correct this producer boundary before consumer projection; downstream reassignment would contradict immutable journal evidence. Cover retained and evicted/unknown generations, explicit identity conflicts and ordinary async compatibility without guessing ownership or making history unbounded. No wire/schema redesign is authorized or currently needed. This is implementation of the accepted immutable-attribution contract, not a new product contract. Slice 3 and whole-task acceptance remain pending; no historical incident repair claim.
