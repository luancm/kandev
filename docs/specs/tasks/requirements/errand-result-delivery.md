---
status: draft
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Errand result delivery

This is the result-delivery portion of [Errands](errands.md). Existing IDs and scope are preserved; the user-selected uncertainty boundary is recorded in the linked ADR.

### REQ-TASKS-ERRANDS-004: Result delivery

**Intent:** Deliver each result once, when the parent can use it.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-004.1:** Superseded by criterion 004.9.
- **AC-TASKS-ERRANDS-004.2:** An await call names errands and a mode. Mode
  `all` shall return when every named errand is settled; mode `any` shall
  return when at least one named errand has an undelivered result or all are
  settled. The response shall report every named errand's status and include
  the text of each undelivered result.
- **AC-TASKS-ERRANDS-004.3:** An await call shall wait at most 600 seconds, or
  a shorter `max_wait_seconds` from 0 to 600. When the wait ends first, the
  response shall mark unsettled errands `pending`. The call shall send
  periodic progress notifications while it waits.
- **AC-TASKS-ERRANDS-004.4:** When an await call is proven abandoned before
  its successful response reaches the parent, the results it claimed shall
  become deliverable again after late successful delivery is fenced. Missing
  receipt evidence alone shall not be treated as proof of abandonment.
- **AC-TASKS-ERRANDS-004.5:** An await for an errand whose result was already
  delivered shall report its status and delivery route without repeating the
  text.
- **AC-TASKS-ERRANDS-004.6:** Superseded by criteria 004.10 and 004.11.
- **AC-TASKS-ERRANDS-004.7:** When the parent's queue is full, results shall
  remain pending and be retried without loss.
- **AC-TASKS-ERRANDS-004.8:** After a backend restart, pending results and
  claims proven undelivered shall still be delivered. Claims with uncertain
  delivery shall follow criterion 004.13. An errand whose parent-initiated turn was
  interrupted shall become `failed` and produce an interruption result.
- **AC-TASKS-ERRANDS-004.9:** Each result shall be delivered at most once, by
  the first of: an await call in progress that names its errand, the parent's
  next turn, or a notification.
- **AC-TASKS-ERRANDS-004.10:** When a parent turn starts with results
  pending, whatever starts it, every pending result shall be attached to its
  context with the errand status block instead of a separate message.
- **AC-TASKS-ERRANDS-004.11:** When a result is pending, the parent is idle,
  and no other entry waits in its queue, the system shall queue one
  notification, and no second one while it waits. With Auto-run on, it shall
  start the parent's next turn at once; with Auto-run off, it shall wait in
  the queue. Its turn shall carry every result then pending. When another
  parent turn takes those results first, the notification shall be removed
  without starting a turn.
- **AC-TASKS-ERRANDS-004.12:** A result that becomes pending while a parent
  turn is running shall not enter that turn unless an await call in that turn
  returns it.
- **AC-TASKS-ERRANDS-004.13:** When await delivery is genuinely uncertain,
  the system shall retain the result without automatic redelivery, expose the
  uncertainty separately from errand lifecycle status, and keep its transcript
  available for deliberate user recovery through existing conversation
  controls on desktop and phone. Legacy claims without receipt evidence shall
  not be silently classified as received or abandoned. This policy follows
  [the await uncertainty decision](../../../decisions/2026-10-30-errand-await-delivery-uncertainty.md).

## System design

- [Errands](../system-design/errands.md)
- [Recovery and delivery](../system-design/errand-recovery-and-delivery.md)
- [Await reception evidence](../system-design/errand-await-receipts.md)

## Implementation plans

- [Review repairs](../../../plans/errands-review-repairs/plan.md)
- [Original delivery](../../../plans/errands/plan.md)
