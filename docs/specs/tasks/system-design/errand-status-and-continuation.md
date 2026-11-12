---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
created: 2026-11-10
owners:
  - kandev
---

# Errand status and continuation

## Authority and mapping

The task system owns status evidence, admission and capacity. This design implements [REQ-TASKS-ERRANDS-002](../requirements/errand-status-and-continuation.md). [Recovery settlement](errand-recovery-and-delivery.md) and the [settlement-authority ADR](../../../decisions/2026-11-09-errand-recovery-settlement-authority.md) remain authoritative for durable outcomes. Attention is a read-derived observation, never a settlement or replay authorization.

## Shared projection

Extend the common input to `errands.ProjectStatus` with a typed recovery-attention observation assembled from repository evidence. `Service.Project` and `Service.list` must use the same classifier. Match unresolved relevant completed non-lifecycle work to retained admission/recovery identity and missing exact receipts; consult current typed `AgentDeliveryRecovery` phase and authoritative newer results. Exclude active dispatch/reconnection and ordinary terminal reconciliation still in progress. `AwaitingResult`, an open turn, a runtime ID or a reserved/unknown admission alone is insufficient. Evidence load failures propagate errors and cannot become a guessed classification. No status read mutates settlement or recovery.

Preserve cancellation, capacity-queued and failed/missing-session precedence. Active work with a pending permission maps to `needs_attention`/`permission`; other active work remains `running`. Identified idle unresolved recovery maps to `needs_attention`/`recovery_unresolved` before an older result can mask subsequent unresolved delegation. With no relevant later unresolved work, retain the previous settled outcome. Historical receipt-only uncertainty cannot overwrite a newer authoritative result.

Add optional closed-set `attention_reason` to backend snapshots/DTOs, list/event/await projections and browser types. Emit it only with attention status. Include the reason in parent context and session listings; absence remains compatible with older permission-only presentations. Do not conflate it with `delivery_state=uncertain`, which concerns parent receipt observation.

`IsLive`, `IsOutstanding`, `liveErrandPredicate` and batched summaries must remain consistent with projection. Unresolved recovery retains capacity and ownership, including idle unresolved follow-ups after a prior outcome. Share the evidence predicate where possible; test SQL and service counts against the same fixtures rather than approximating with `last_outcome=''` or `awaiting_result` alone.

## Continuation and transaction ordering

Keep all existing exact-settlement gates at user prompt, queue readiness, parent reservation and repository admission. Proven interruption permits only a newly requested follow-up after every relevant prior decision is committed. Review `loadResumeSessionCandidate`: a missing executor row may be allowed only for an exactly settled interrupted Errand with supported retained native identity and otherwise valid restoration; do not inherit successful-completion cleanup semantics or generalize `AllowCompletedSessionResume`.

Both `CreateTurnWithConversationReceipt` and `CreateTurnWithStepStampConversationReceipt` must call `guardErrandTurnReservation` before `prepareConversationMutation` takes the session-turn lock, under the same owning transaction. The unstamped fallback uses the same guarded start. Do not guard every conversation mutation: recovery/message updates must remain available. Rejected continuation must not insert a turn, advance conversation revision or publish a start. Preserve lock order: task/Errand ownership before session-turn mutation.

Map unresolved-settlement refusal to `errand_recovery_unresolved` through existing API/MCP error paths and localized frontend handling. Explain that earlier work and possible external effects are unknown, recovery must verify it before continuation, and inspection/cancellation remain available. Stop does not promise settlement or later continuation. No user override bypasses this guard.

## Desktop and phone presentation

Reuse the shipped Errands row, desktop popover, phone drawer, full-screen child conversation and Back navigation. Keep existing Open and Stop actions; no new modal, layout or resume button is needed. Show a recovery-specific explanatory line and accessible attention indicator instead of implying permission approval. The opened child shows the same explanation and preserves its transcript. A rejected send shows the stable localized refusal without a new turn. Supported explicit follow-up after proven interruption uses the existing composer.

This is content/status work within existing compositions. Shared projection/state owns behavior on both viewport sizes. Preserve the existing scroll owner, focus return, touch targets and safe-area treatment. Add all required translations and focused rendered desktop/phone checks; extend `errands.spec.ts` and `mobile-errands.spec.ts` for attention → Open → explanation → refused send, and separately settled interruption → supported follow-up.

## Verification

Cover startup, active permission/reconnection, pending terminal reconciliation, idle unresolved first/follow-up work, older/newer result precedence, stale recovery identity, mixed historical receipts, cancellation and evidence-read errors. Assert status/reason agreement across DTO/MCP/parent/browser surfaces and SQL live counts. Test both conversation-receipt writers and fallback with repeated refusals and zero side effects, SQLite/PostgreSQL lock ordering, explicit user/parent continuation after proven interruption, supported missing-runtime restoration, no original replay and unchanged workflow/result revisions. PostgreSQL without a DSN is a reported skip.

### Activity counts and individual panel cancellation

For AC-TASKS-ERRANDS-010.9, derive running, queued and attention counts from the same selected-parent inventory in `errands-chip.tsx`. Compose localized nonzero counts in attention, running, queued order; all-settled inventories keep the existing history/idle label. Attention tone and the recovery-specific accessible explanation remain intact. Keep `isLiveErrand` and task-wide capacity/Stop-all counts unchanged: live includes queued and attention, and is not synonymous with executing. A missing attention reason retains generic attention behavior.

For AC-TASKS-ERRANDS-010.12 and 011.8, `errands-panel-row.tsx` exposes Stop for `isLiveErrand` through the existing `useErrandActions().cancelErrand(errand_id)`. Reuse the header's cancellation/error conventions and existing `errand.cancel` transport; do not add a deletion endpoint or optimistic terminal state. Each row owns its pending presentation. The authoritative `errand.updated` snapshot drives history placement and chip counts. Stop never dismisses the panel or triggers Open. Settled history retains Open without Stop. Stop-all remains task-wide even when the displayed parent is scoped.

Desktop rows place Open and Stop in a compact action group. Phone rows use a noninteractive card containing a summary/Open button and a separate Stop button, rather than nesting Stop inside the current whole-row button. Reuse the Errands drawer and mobile picker sheet geometry: list scrolls once, footer remains fixed, safe areas and focus return remain owned by the existing surface. Open remains the primary navigation action; Stop is visible without hover. Use 28px desktop actions and at least 44px phone/coarse-pointer targets. Show cancellation progress and a localized error without removing the recovery warning or claiming uncertain effects were resolved.

Regression coverage includes attention-only, queued-only, running-only, settled-only, and mixed inventories; parent scoping; permission attention without a reason; cancellation success/failure/duplicate click; and isolation between two live errands. Desktop and phone rendered tests exercise direct panel Stop with transcript retention. No backend status, capacity, persistence or recovery-admission changes are required.
