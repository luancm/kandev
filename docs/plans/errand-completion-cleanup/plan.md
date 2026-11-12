---
created: 2026-11-03
status: done
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
system_design:
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
legacy_specs: []
---

# Complete errand sessions and release idle runtimes

## Current state and authorization

2026-11-12 follow-up: the user reported repeated cancelled-session projection and legacy admission-authority warnings, then explicitly requested Errands to implement the fix. [Task 02](task-02-terminal-compatibility.md) owns this bounded remediation under the existing cancellation-precedence and conservative-recovery contract. Implementation and review remediation are accepted: 77 parent race tests and scoped Go lint passed; the fresh combined desktop suite passed 19/19. Final phone and delivery evidence are tracked in the [parent integration handoff](../errand-panel-controls/final-integration-handoff.md). The panel-controls work remains a separate package.

The user approved automatic runtime close after durable result capture, retained transcripts/results, clear completed session state and explicit continuation, and requested an Errand to implement it. This is new lifecycle behavior: review-repair Task10 explicitly excluded successful-errand runtime parking. The existing 29 repair work orders remain complete; this package owns the new work.

At source `18c03cc4cf591908ffb125beb7a24c50a089514c`, `settleDetachedAgentReadyTurn` publishes `WAITING_FOR_INPUT` after settlement and leaves a promptable runtime. `finishAgentCompletedTurn` schedules cleanup but leaves the same waiting state. Inspection found 81 retained errand sessions in that state, all with stopped executor records; live runtime inventory contained only the parent. Those observations demonstrate stale retained state, not 81 live runtime leaks.

Cleanup is implemented in checkpoint `4becbc47a1` and subsequent upstream integration corrections through pushed `452c9d4f0d`. The branch includes upstream main `37f081a976`. Final fresh managed desktop 21/21 and phone 19/19 acceptance passed, including passive inspection, completed-child follow-up and repeated native resume. Affected backend/storage/frontend checks and the normal optimized build also passed; 228 focused frontend tests passed. The earlier runtime observations above are historical evidence, not current runtime counts. [The integration handoff](../errands-remaining/integration-handoff.md) owns the final receipt and limits.

## Scope and order

One sequential vertical work order covers successful settlement, exact runtime close, durable recovery, completed session projection and explicit continuation. The task system owns this outcome because it owns errand identity, result settlement and conversation lifetime. Reuse existing cancellation/admission and recovery machinery with separate success effects; preserve cancellation, ordinary-session idle behavior and shared-worktree ownership.

No transcript deletion, live database repair, new global state, workflow changes, deployment, or worktree removal is included. An isolated test instance owns browser fixtures. Existing historical sessions may be repaired only by the implemented evidence-gated reconciliation path, never by this planning session's direct SQL writes.

## Technical approach

The [system design](../../specs/tasks/system-design/errand-completion-cleanup.md) specifies settlement-before-close, completion-purpose teardown intent compatibility, successor fencing, state publication after confirmed close, and same-conversation resume. Existing `TaskSessionStateCompleted` and `AllowCompletedSessionResume` are the starting points; generic cancellation is not a successful-close implementation.

| Runtime boundary | Behavior and evidence |
| --- | --- |
| Ready event with a live provider | Save result/no-result, close exact execution, confirm removal, publish completed |
| Process exit | Reuse exact terminal evidence and confirmed absence; no duplicate result or cancellation |
| Explicit follow-up | Preserve conversation and role; provider restoration follows existing capability/recovery policy |
| Unknown/restored runtime inventory | Fail closed and retry; no fabricated absence or replacement prompt |
| Ordinary sessions | Existing waiting/idle behavior remains a positive control |

## UI-01: Retained completed conversation

```text
Desktop parent panel / phone Errands drawer
  Completed (1)
    Audit release checklist   Done   Open

Opened child (desktop pane / phone full-screen chat)
  Back to parent
  Session: Completed
  [retained transcript and context history]
  [existing explicit continuation controls]
```

This changes state semantics within existing surfaces, not layout. The phone retains its drawer, full-screen child and Back action; existing safe areas, scroll ownership and touch targets remain. Labels above are illustrative existing concepts, not new hardcoded copy. Opening history must not create a runtime; an explicit follow-up must resume the retained conversation. Maps to AC.2/.3 and desktop/phone Errands browser checks.

## Work order and acceptance mapping

- [x] [Task 01: Close completed errand runtimes and preserve continuation](task-01-completion-cleanup.md)
- [x] [Task 02: Reconcile cancelled and legacy cleanup obligations](task-02-terminal-compatibility.md)

| Acceptance | Regression boundary |
| --- | --- |
| AC.1 | Ready/process-exit settlement, close errors, parent/user origins and delivery retention |
| AC.2 | Completed state after confirmed close, existing history/navigation/workspace and ordinary-session controls |
| AC.3 | User and parent follow-up, same-execution successor and replacement-incarnation races |
| AC.4 | Restart/flag-off retry, legacy intent decoding, existing-session repair with positive/unknown evidence |

## Verification and execution status

The user authorized implementation after the design package, then requested direct primary execution. The primary completed durable intent ownership, exact-authority close, receipt/projection retry, conservative legacy repair, explicit retained-conversation continuation, tests and public coordination documentation. No additional agents were launched. Recovery integration was completed at `67b6b9ba9d` before cleanup integration.

Deterministic tests capture durable result state at stop entry, quiesce detached effects before assertions, and exercise close/receipt/projection/storage failures, cancellation precedence, stale successor/replacement protection and explicit versus passive resume. Post-upstream tests also reproduce a newer lifecycle-only row shadowing real work and continuation after removal of the old runtime row. The corrections reuse canonical conversational-turn ownership and existing exact task-bound completed-resume permission.

Final evidence is recorded in [the integration handoff](../errands-remaining/integration-handoff.md). PostgreSQL without a DSN and mock-provider certification limits remain explicit. No live deployment or historical database repair is part of acceptance.

## Preserved safety boundaries

Completion and cancellation have distinct effects under the existing durable teardown owner. Physical close shares admission arbitration; stale receipts cannot stop or complete a successor or replacement incarnation. Exact successful terminal authority precedes close, confirmed close precedes completed projection, and retries retain both obligations. Explicit continuation is task-bound and settlement-validated; passive inspection grants no permission. Legacy repair requires positive identity/absence evidence; old stopped rows alone remain insufficient.
