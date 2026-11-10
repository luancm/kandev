---
created: 2026-11-08
status: in_progress
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
legacy_specs: []
---

# Implementation Plan: Errand orphan settlement

## Overview

Repair restart recovery that leaves an idle Errand reporting running indefinitely. Preserve exact-turn settlement and uncertain-submission safety. This is a conformance repair for AC-TASKS-ERRANDS-002.1 and AC-TASKS-ERRANDS-003.1, not permission to infer success from an idle session. The existing requirements and exact-settlement design remain authoritative.

## Evidence and current state

See [incident evidence](evidence.md). Historical receipts, exact recovery assessments, immutable ownership, shell isolation and actual child-to-parent provider delivery are implemented and have prior independent verification. The user requested committing and pushing this completed core for a new runtime build on 2026-11-10. Primary is checking the recovered receipt-writer guard and typed-warning tests before the commit; final evidence is recorded in [the delivery checkpoint](delivery-checkpoint.md). The parent checkout and staged merge remain untouched.

The broader work order remains in progress. Evidence-aware `needs_attention`, `attention_reason`, SQL capacity parity, localized desktop/phone explanations, stable refusal mapping and explicit-resume eligibility remain designed but unimplemented. The turn-creation guard is included in this checkpoint, but does not complete the broader continuation contract. Historical incident repair eligibility remains unproven. A restart must use a build containing this branch and cannot reconstruct missing historical evidence by itself.

## Scope

Repair the recovery-to-settlement boundary and provide safe convergence for already stranded records with sufficient durable evidence. Include evidence-aware nonterminal recovery attention, rejection before creating an inadmissible continuation turn, supported explicit follow-up and explanatory copy on existing desktop/phone surfaces. Exclude generic idle timeouts, successful-completion cleanup changes, uncertain replay/override, frontend redesign, live database edits and completion of the pending Git merge. The sole live-state exception was the user's explicitly authorized cancellation/replacement of this task's stranded implementation Errand.

## Technical approach

Trace `AbandonOpenTurns` in `internal/task/service/service_turns.go`, its repository `AbandonTurn` implementation, and startup/resume callers in `internal/orchestrator/service.go` and `task_operations.go`. At the authoritative recovery decision, persist an exact interruption decision with the same turn closure or an equivalent durable retryable effect. Preserve task/session/incarnation/generation/turn/submission ownership and cancellation precedence. Do not treat every call to generic abandonment as proof that an admitted submission has safely terminated.

Integrate with `internal/task/repository/sqlite/errands_turn_settlement.go`, `errands_turn_terminal_evidence.go`, and `internal/errands/terminal_decisions.go`. Classify recovery-generated turns at their source: bind genuine work to its proven admission identity; mark genuinely lifecycle-only activity using the existing lifecycle-only contract. Never blanket-exempt recovered turns or use the mutable latest turn.

For existing records, require durable recovery evidence tying the exact original admission to the interrupted and fenced runtime. `interrupted_unknown` alone does not authorize replay or success. Missing or conflicting identity stays blocked. Ensure one unrelated unclassifiable turn does not prevent committing independently proven earlier interruption decisions; successor admission must still require all preceding relevant decisions. Reconciliation must remain idempotent and retain uncertainty without resubmission.

## Tests

Add `TestErrandRestartAbandonmentSettlesExactInterruptedTurn` in `apps/backend/internal/orchestrator/errand_recovery_interruption_test.go`, reproducing real admission, recovery abandonment, replay-created turns, and Errand reconciliation using SQLite. Assert failed/interrupted status, exactly one parent result, unchanged task workflow, and no provider prompt replay. Add repository cases in `errands_turn_settlement_test.go` for ambiguous and mixed evidence, generation mismatch, cancellation races, transaction failure, and repeated recovery.

## End-to-end evidence

The orchestrator integration exercises admission through durable recovery, result capture and parent delivery. It must read the same Errand projection consumed by the panel and injected parent status. No rendered layout changes are planned; browser automation is not required to prove this backend mismatch.

## Work orders

- [ ] [Task 01: Preserve exact settlement across recovery](task-01-recovery-settlement.md)

## Verification results

Prior slice verification is recorded in the sibling handoffs. Final checkpoint checks and commit/push receipts are tracked in [delivery-checkpoint.md](delivery-checkpoint.md). PostgreSQL runtime tests require a DSN and were skipped previously; two broader orchestrator Send Now failures were independently reproduced against the base commit. No complete-task or universally green-suite claim is made.

## Risks

Recovery can resume an existing native conversation without proving the prior prompt's outcome. An interruption result must describe interrupted delegation, never certify that underlying commands had no side effects. Already-created replay turns require evidence-based classification. Preserve PostgreSQL locking parity and report database-dependent skips honestly. Raw ACP logs were unavailable in the inspected log directory; the incident trace establishes the persistent settlement gap, not the provider's final protocol outcome.

## Implementation handoff, 2026-11-09

The design checkpoint and read-only existence check are complete. The user explicitly selected Errands for implementation. Task 01 is in progress. The first Astralopiteco implementation Errand returned a red regression and stopped at the lifecycle authority boundary with no production changes. A read-only recovery-authority investigation is next; current ownership and session IDs are tracked in the shared task plan. The primary session owns plan/spec updates and integration. The Errand owns the scoped backend recovery/settlement files and tests plus implementation-handoff.md; it must not delegate recursively or edit other documentation.

The primary session owns integration, plan/spec updates, final verification, and parent-merge compatibility. Execute the cohesive recovery/settlement slice sequentially. Bounded supporting Errands may own a named investigation or disjoint test file after dispatch; each brief must forbid recursive delegation and warn that others share the checkout. Record ownership before dispatch, inspect artifacts and test handles directly, and do not rely on the defective completion notifications to establish progress. Never implement a child's assigned files concurrently.

### Findings that constrain implementation

- `AbandonOpenTurns` delegates to timestamp-only `AbandonTurn`; startup and resume callers do not supply exact settlement authority. Do not turn generic abandonment into unconditional terminal proof.
- `PendingErrandTurnSettlements`, reservation's `RequireErrandTurnSettlements`, and commit's `guardObservedErrandTerminalTx` all need examination together. The commit guard currently requires the current authoritative turn, so allowing enumeration of an earlier proven interruption alone is insufficient. Any historical receipt path must preserve newer result revisions and leave successor admission blocked by remaining unresolved turns.
- The parent staged `finishPromptExecutorDispatch` change preserves dispatching submissions on `foreground_idle`, waiting for real terminal evidence. Preserve that distinction; idle is not interruption or completion proof. The parent's staged `session.go` edits concern context-window inference and active-task metadata, not `AbandonTurn`; do not import the file wholesale.
- ADRs `2026-10-12-total-session-recovery` and `2026-11-02-agent-process-group-termination-proof` separate retired admission from historical outcome and require exact owner evidence. Reuse their authoritative assessment; metadata labels and `interrupted_unknown` alone are insufficient.

### Material unknowns and next evidence

Determine which durable recovery assessment binds the original turn/submission to a fenced owner, and whether that evidence survives for the historical incident. Trace generation-one and generation-two turn creation to distinguish admitted work from lifecycle notices at their source. If historical ownership cannot be proven, retain a blocked record rather than promise automatic repair. Prove cancellation, successor-result preservation and transaction rollback in SQLite before altering repository guards; exercise PostgreSQL lock parity when a test DSN is available. The provider's original terminal outcome remains unknown.

Next: reproduce the gap with real SQLite admission/recovery, record the red result, implement the smallest authoritative recovery receipt path, classify recovery-created turns, then run the work order's serialized checks. Update owning design/requirements only if the selected mechanism changes their durable contracts. Record unrelated findings separately.

## Accepted recovery boundary

The [recovery authority ADR](../../decisions/2026-11-09-errand-recovery-settlement-authority.md) separates admission-only retirement, interrupted-delegation assessment, historical receipt commit and current result capture. AC-TASKS-ERRANDS-003.6/003.7 and the owning tasks/platform designs now state those contracts. Implement sequentially: proven historical receipts first, exact lifecycle assessment second, then generated-turn attribution and integrated acceptance. The next Errand owns only the first slice; no historical live repair is promised or authorized.

## Slice 1 acceptance

Historical receipts are implemented and accepted after primary diff inspection and independent race checks of receipt convergence, restart reproduction and unchanged successor protection. Full evidence and baseline failures are in [the work order](task-01-recovery-settlement.md#results) and [slice handoff](historical-receipts-handoff.md). PostgreSQL parity remains skipped for missing DSN. Slice 2 is next: exact lifecycle assessment, durable retry evidence and settlement consumption. Generated-turn attribution and full integrated acceptance remain slice 3; no live incident repair is claimed.
