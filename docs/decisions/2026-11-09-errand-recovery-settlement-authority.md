# ADR-2026-11-09-errand-recovery-settlement-authority: Separate interrupted delegation from provider outcome

**Status:** accepted
**Date:** 2026-11-09
**Area:** backend, protocol

## Context

Recovery can abandon an Errand turn without the exact settlement decision required by successor admission. Native-resume evidence permits retirement of uncertain admission history but does not prove the provider outcome. Later unattributed activity can also prevent committing an independently proven older decision. Treating admission retirement as terminal proof, or replacing current results with historical ones, would erase uncertainty or newer work.

## Decision

Keep native-resume delivery evidence admission-only. A separate typed interrupted-delegation assessment requires positive termination proof for the exact original owner and authenticated inventory/projection reconciliation of the exact admitted submission. Bind task, session, incarnation, execution, native identity, harness and prompt generation, turn, submission, stream and payload hash; preserve proof provenance. Persist the assessment before cleanup, binding replacement or retirement destroys its source evidence. A pending assessment alone cannot settle a delegation: reconciliation must validate its exact identity and completed recovery conditions under the owning transaction locks. Keep historical SQL uncertainty, provider labels and payload unchanged. A failed/interrupted Errand result describes the delegation and makes no claim that external effects did not occur.

Use a distinct versioned effect in the existing durable delivery effect mechanism; do not synthesize a provider terminal event or sequence. Validate its structured evidence separately from terminal outcomes. Persistence failure prevents evidence-destroying cleanup; subsequent failures retain retryable evidence. Historical repair requires retained exact evidence or its safe rederivation. Missing, conflicting or unsupported proof remains fenced.

Separate historical receipt commit from current result capture. An independently proven completed turn can receive its receipt while a later independent or unclassified turn remains unresolved. This receipt-only operation does not mutate the Errand row, result, revision, delivery state, notifications or counters. Keep the current-result authority guard and cancellation semantics. Successor admission continues to require every relevant preceding decision; completing one historical receipt does not excuse another missing decision.

Preserve immutable admitted-turn attribution for replayed work. Unowned real activity remains unowned and cannot be assigned to an interrupted submission by proximity. Lifecycle-only classification requires typed producer provenance covering all activity in the turn; warning text, idle state and generation changes are insufficient.

Retain general admission ownership separately from interruption proof in `agent_delivery.prompt_admission.v1`, committed atomically before dispatch through running-row persistence. Retain unowned tool-start correlation in `agent_delivery.unowned_tool_source.v1` with inbox acceptance. Both are versioned sequence-zero effects that survive event compaction and mutable running-row replacement, with session cleanup owning their lifetime. Neither confers terminal or death authority. The storage tradeoff is compact session-lifetime provenance instead of inferring ownership from mutable state or already-pruned event bodies. Shell message lookup and checkpoint updates must remain inside the resolved turn and parent-tool boundary; ambiguous source correlation fails closed.

## Consequences

Unresolved recovery is presented as `needs_attention` with `attention_reason=recovery_unresolved`; permission attention uses `permission`. Reusing the existing nonterminal status preserves capacity and cancellation behavior without inventing an outcome or conflating it with parent receipt uncertainty. The [status and continuation design](../specs/tasks/system-design/errand-status-and-continuation.md) defines evidence-aware projection and rejection before turn creation. Explicit continuation still requires all exact receipts; accepting uncertainty as an override is excluded.

Recovery producers must retain the exact turn and owner provenance through cleanup and retries. Older data without sufficient proof may remain unresolved. Historical receipts can converge without falsely publishing a current result. Implementation is tracked in [the orphan-settlement plan](../plans/errand-orphan-settlement/plan.md); this decision does not claim the live incident is eligible for automatic repair.

## Alternatives Considered

- Reuse admission-retirement proof as terminal authority: some valid retirement paths do not prove original-owner termination.
- Mark every abandoned turn interrupted: generic abandonment is timestamp cleanup, not outcome authority.
- Relax the current-result guard: could overwrite later work or cancellation.
- Block every historical decision on any later missing receipt: strands independently proven decisions unnecessarily.
- Reclassify recovery warnings by text: ordinary work can contain identical text and mixed activity.
