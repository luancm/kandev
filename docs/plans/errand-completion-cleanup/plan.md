---
created: 2026-11-03
status: draft
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

The current parent-bound MCP endpoint was inspected read-only: initialization succeeded and `tools/list` returned 43 tools, with no `start_errand_kandev`, `await_errands_kandev` or `cancel_errand_kandev`. No implementation errand has been launched. Do not replace it with a persistent task or change the live installation/feature flag silently. The requested Errand execution can start once the current parent advertises that capability.

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

Design package only; no production or permanent test changes. Exact TDD, browser and documentation commands are in the work order. Root-cause evidence is source inspection plus the read-only runtime/database observations above; behavioral RED is required before implementation. Native Errand launch is unavailable in this parent's currently advertised tool catalog. The repository's design-package handoff applies before permanent implementation.

Design validation passed on 2026-11-03: catalog validation (422 decisions, 1,864 specifications), all specification structure/size lint, whitespace checks, and full-range documentation coverage (732 changed paths, 92 reference documents, `covered`). Receipts: `/tmp/errand-completion-plan-catalog.log`, `/tmp/errand-completion-plan-lint.log`, `/tmp/errand-completion-plan-coverage.json`. Four new package/specification files and two owning-document links remain unstaged and uncommitted. These checks validate the design package only; no product acceptance is claimed.

## Risks

The existing teardown path is cancellation-specific; reusing it unchanged would overwrite successful outcome or discard results. Closing while holding completion ownership can reproduce guard re-entry deadlocks. A stale close can kill a resumed turn unless the physical effect shares admission arbitration. Marking a session completed without authorizing the existing explicit resume paths can make retained history impossible to continue. Legacy repair needs positive identity/absence evidence; old stopped rows alone are insufficient.
