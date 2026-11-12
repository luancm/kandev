---
created: 2026-11-03
status: in_progress
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
system_design:
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
legacy_specs: []
---

# Complete errand sessions and release idle runtimes

## Current state and authorization

The user approved automatic runtime close after durable result capture, retained transcripts/results, clear completed session state and explicit continuation, and requested an Errand to implement it. This is new lifecycle behavior: review-repair Task10 explicitly excluded successful-errand runtime parking. The existing 29 repair work orders remain complete; this package owns the new work.

At source `18c03cc4cf591908ffb125beb7a24c50a089514c`, `settleDetachedAgentReadyTurn` publishes `WAITING_FOR_INPUT` after settlement and leaves a promptable runtime. `finishAgentCompletedTurn` schedules cleanup but leaves the same waiting state. Inspection found 81 retained errand sessions in that state, all with stopped executor records; live runtime inventory contained only the parent. Those observations demonstrate stale retained state, not 81 live runtime leaks.

Current execution base is parent branch commit `67b6b9ba9d`, which integrates reviewed recovery attention/refusal/continuation, exact parent terminal repair and immutable duration. Combined backend, persistence, frontend and desktop/phone acceptance passed; the checkpoint is pushed and remote-verified. The user subsequently requested direct primary execution. The previous implementation Errand was cancelled through the supported API and its code was preserved. Production cleanup and targeted regressions are implemented; final race, desktop/phone acceptance and delivery are running in the primary session. Earlier source/runtime observations above are historical evidence, not current runtime counts.

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

- [ ] [Task 01: Close completed errand runtimes and preserve continuation](task-01-completion-cleanup.md)

| Acceptance | Regression boundary |
| --- | --- |
| AC.1 | Ready/process-exit settlement, close errors, parent/user origins and delivery retention |
| AC.2 | Completed state after confirmed close, existing history/navigation/workspace and ordinary-session controls |
| AC.3 | User and parent follow-up, same-execution successor and replacement-incarnation races |
| AC.4 | Restart/flag-off retry, legacy intent decoding, existing-session repair with positive/unknown evidence |

## Verification and execution status

The user authorized implementation after the design package. The corrected regression packet and contract review are accepted; see the work order and regression handoff. Recovery integration is complete at `67b6b9ba9d`, so the former dependency is resolved.

The primary now owns the complete work order directly: durable completion intent, exact-authority cleanup, retry/projection, explicit continuation, regressions, desktop/phone acceptance, public documentation, upstream integration and push. Earlier reviewer feedback on the regression packet remains accepted. No new delegation, independent cleanup coordinator, workflow or shared-workspace removal is introduced.

Existing tests capture durable state at stop entry and use deterministic `synctest` quiescence to prove no premature completion and no late successor/cancellation effects. Retain those barriers. Production acceptance additionally requires the retry/crash/legacy/repeated-continuation matrix in the work order. PostgreSQL skips and mock-provider limits must remain explicit. No successful-cleanup acceptance or live deployment is claimed yet.

## Risks

The existing teardown path is cancellation-specific; reusing it unchanged would overwrite successful outcome or discard results. Closing while holding completion ownership can reproduce guard re-entry deadlocks. A stale close can kill a resumed turn unless the physical effect shares admission arbitration. Marking a session completed without authorizing the existing explicit resume paths can make retained history impossible to continue. Legacy repair needs positive identity/absence evidence; old stopped rows alone are insufficient.
