---
status: draft
system: tasks
created: 2026-11-10
owners:
  - kandev
---

# Errand status and continuation

## Overview

The task system owns the consistent status and safe continuation of an Errand.
This contract complements [Errand results and ownership](errands.md); attention
is not evidence of a terminal outcome.

## Requirements

### REQ-TASKS-ERRANDS-002: Errand status

**Intent:** Expose one consistent errand status.

The [completed-session contract](errand-completion-cleanup.md) separately defines successful runtime cleanup, retained session state and explicit continuation.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-002.1:** An errand shall report exactly one status:
  `queued` (waiting for session capacity, no runtime), `running` (starting or
  executing a turn), `needs_attention` (an active permission request or identified idle
  unresolved recovery requiring attention), `done` (its latest parent-initiated turn completed),
  `failed` (launch failed, its latest parent-initiated turn ended in error or
  interruption, or its session failed), or `cancelled`.
- **AC-TASKS-ERRANDS-002.2:** While a user-initiated errand turn is active,
  the errand shall report `running` or `needs_attention`. When that turn
  settles, the errand shall return to its previous settled status.
- **AC-TASKS-ERRANDS-002.3:** The parent agent's session listing shall report
  each session's role, parent session, and errand status.

- **AC-TASKS-ERRANDS-002.4:** An idle launched Errand with identified unresolved recovery shall report `needs_attention` with reason `recovery_unresolved`, not `running` or a fabricated terminal outcome. Active permission attention shall report reason `permission`. An ordinary startup, active reconnection or terminal reconciliation in progress shall not be classified as unresolved recovery solely because its result is absent. Failed evidence reads shall not manufacture a recovery classification.
- **AC-TASKS-ERRANDS-002.5:** Recovery attention shall preserve result identity, revision, delivery state, cancellation and capacity accounting. A later unresolved delegation may require attention despite an older result; unrelated historical uncertainty shall not replace a newer authoritative result. Idle sessions without subsequent unresolved work shall retain their settled status.
- **AC-TASKS-ERRANDS-002.6:** Lists, events, parent status blocks, session listings and desktop/phone presentations shall agree on the attention reason. Recovery attention shall explain that earlier work and its external effects remain uncertain, offer conversation inspection and existing cancellation, and not imply a pending permission request or promise that cancellation repairs recovery.
- **AC-TASKS-ERRANDS-002.7:** After interruption is durably settled and all preceding relevant work has exact decisions, an explicit user or authenticated parent follow-up shall admit only the newly requested instruction and restore the same conversation where supported. Missing runtime records alone shall not prevent restoration when exact retained identity establishes eligibility. Unsupported or ambiguous restoration shall fail without replaying the original uncertain instruction or changing workflow state.
- **AC-TASKS-ERRANDS-002.8:** Unresolved prior work shall refuse continuation with stable reason `errand_recovery_unresolved` and an actionable explanation. Repeated refused attempts shall create no turn, conversation revision, turn-start publication or provider request. The refusal shall not weaken ownership or settlement checks.

## Exclusions

No new terminal status, automatic replay, historical success inference, manual acceptance of uncertain effects or new screen is introduced. Recovery attention and parent delivery-receipt uncertainty remain distinct. Existing safe cancellation is not settlement proof.
