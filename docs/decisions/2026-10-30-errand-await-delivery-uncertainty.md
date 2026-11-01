# ADR-2026-10-30-errand-await-delivery-uncertainty: Preserve uncertain await delivery

**Status:** accepted
**Date:** 2026-10-30
**Area:** backend, frontend, protocol

## Context

Errands requires at-most-once result delivery and recovery of abandoned awaits. The supported MCP transports do not expose a complete durable receiver receipt. A provider tool-result observation can prove reception when correlated to the exact result, but its absence cannot prove abandonment. A crash before reception and a crash after reception but before journalling can leave identical backend evidence. Handler return, socket write, elapsed time and backend acknowledgement cursors are not native-client reception evidence.

The user selected preventing duplicates over automatic redelivery of uncertain results on 2026-10-30. This follows the existing [durable harness boundaries](2026-09-10-durable-harness-session-boundaries.md) and [honest delivery-loss recovery](2026-10-11-honest-delivery-loss-recovery.md) policies.

## Decision

Automatically release only claims for which non-exposure is positively established and late successful exposure is fenced. Preserve genuinely uncertain claims across restart without automatic replay. Keep uncertainty visible, preserve the underlying result and transcript, and allow the user to inspect them and deliberately recover through existing conversation controls. Manual recovery is a user decision, never an inferred acknowledgement or automatic retry.

Reception means acceptance by the parent agent's MCP client, not model cognition. Positive receiver evidence must bind the session, attempt and immutable result identity. Missing tool output or a timeout is not negative evidence. Legacy await claims without outcome evidence remain uncertain; migration must not infer receipt or abandonment.

Atomic batch claiming, exact-token release, cancellation fencing and known-error cleanup are required independently. A surviving MCP handler that commits to returning an error before successful result exposure can establish an abort, provided the backend fences late claims. Uncertain SQL commit and uncertain response exposure remain distinct states. The design must not promise receiver guarantees absent from the supported transport.

## Consequences

Automatic duplicate delivery is avoided at the cost of requiring explicit inspection/recovery for some crash windows. Errand lifecycle and result-delivery certainty remain separate. Public documentation and the parent/user presentation must state this limit. Recovery of definite failures remains automatic; uncertainty is not permission to leave ordinary partial-write failures unfixed.

## Alternatives Considered

- Automatically replay all unconfirmed awaits: rejected by the user because the parent may receive and act on a result twice.
- Treat successful writes or missing journal records as receipt/non-receipt: rejected because neither proves the receiver boundary.
- Add a universal receiver-admission protocol: no such capability exists across supported providers; not a prerequisite for these bounded repairs and not silently promised.

## References

- [Errand result delivery](../specs/tasks/requirements/errand-result-delivery.md), REQ-TASKS-ERRANDS-004.
- [Errands design](../specs/tasks/system-design/errands.md).
- [Repair plan](../plans/errands-review-repairs/plan.md).
