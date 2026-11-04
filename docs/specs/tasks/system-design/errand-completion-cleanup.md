---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
created: 2026-11-03
owners:
  - kandev
---

# Completed errand execution cleanup

## Boundary and existing behavior

This design implements [completed errand sessions](../requirements/errand-completion-cleanup.md), reusing the exact identity, durable settlement and admission boundaries in [errand recovery and delivery](errand-recovery-and-delivery.md). The [task-owned worktree decision](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md) remains authoritative. Conversation retention and execution lifetime are separate: completion releases execution resources, while explicit continuation may restore the retained conversation.

`settleDetachedAgentReadyTurn` currently settles an errand result and publishes `WAITING_FOR_INPUT`; it leaves a promptable runtime. The process-exit path through `finishAgentCompletedTurn` schedules execution cleanup but also leaves an errand session waiting. `TaskSessionStateCompleted` already exists. Generic completed-session resume is deliberately gated by `AllowCompletedSessionResume`; the errand follow-up paths must supply that permission only after validating the exact completed errand and existing caller authority.

| Acceptance | Owner |
| --- | --- |
| .1 | Exact terminal settlement followed by durable success-close intent |
| .2 | Confirmed execution close and conditional session projection |
| .3 | Admission arbitration and explicit same-conversation resume |
| .4 | Existing startup/sweep recovery and conservative retained-session repair |

## Durable close authority

Extend the existing private `ErrandTeardownIntent` boundary with an explicit completion purpose. A missing purpose remains cancellation for compatibility with stored intents. Keep one versioned JSON contract and exact target identity rather than adding another teardown table or independent coordinator. Any schema/codec changes must preserve both SQLite and PostgreSQL behavior.

Capture task, immutable role/parent, session incarnation, execution, exact terminal turn and submission from existing positive evidence. Commit the completion obligation alongside the exact terminal settlement decision, or recover it idempotently from that committed decision before successor admission. A result's delivery state is independent: neither awaiting nor uncertain parent reception delays or authorizes a different close target. A user-origin turn uses its existing no-result settlement receipt and never creates a parent notification merely because its runtime closes.

Successful completion must not call the cancellation operation unchanged. `errand_teardown.go` currently uses `cancelSession`, cancellation reasons and failed execution state. Share the exact-target validation, admission arbitration, confirmed-stop and receipt machinery, while selecting distinct completion effects. Completion preserves errand lifecycle/outcome, result revision, delivery claims, transcript, provider resume identity and task-owned environment. Cancellation takes its existing semantic precedence; retries must not convert a cancellation into successful completion or lose an outstanding physical-stop obligation.

## Ordering, races and session state

Use the existing session cancellation/admission ownership and durable task guard in their established order. Persist settlement and close authority before external I/O; never hold SQL or a non-reentrant completion guard across runtime calls or state publication. Stop only the captured execution, and close its stream/client and lifecycle indexes through the existing runtime API.

Revalidate the exact target under operation ownership before the physical stop. An admitted successor on the same execution invalidates the old close authority. A replacement execution/incarnation must never be stopped or projected completed by an old callback. A queued follow-up admitted before closing may reuse the existing runtime; once close owns the operation, follow-up waits for the close decision and then uses normal recovery. Queue sources and persisted results survive both orderings.

After an acknowledged stop or authenticated absence, conditionally publish the existing `COMPLETED` session state for the unchanged successful turn. Apply that rule to both ready and process-exit paths. Do not use the generic errand Stop action: it means cancellation. Publish outside ownership locks so synchronous subscribers and queue readiness cannot re-enter the same guard. Ordinary sessions retain their current idle policy.

## Recovery and explicit continuation

Reuse the existing bounded startup and errand teardown sweep, including flag-off cleanup of already accepted work. Retry pending completion intents without rerunning provider prompts or result delivery. Provider-stop, inventory and receipt failures leave durable work pending; neither scheduling nor elapsed time proves absence.

For older successful retained sessions, derive repair only from exact durable terminal evidence plus authenticated runtime inventory and unchanged session/turn ownership. Stopped executor SQL rows alone are insufficient physical-absence evidence. Missing provenance remains observable and conservative. Preserve failed/cancelled outcomes and avoid changing unrelated ordinary sessions.

User follow-up and parent `message_task_kandev` use existing authorization, queue, result-supersession, recovery and launch-isolation paths. Permit completed resume only for the admitted retained errand; keep normal-session completion policy unchanged. Restore the same provider conversation when supported; existing explicit recovery errors or fallback policy remain authoritative when restoration is unavailable. New errand admission flags must not disable required cleanup or retained role isolation.

## Presentation and validation

Use existing session status rendering and completed-session continuation controls. No new session-state enum, management surface or phone layout is required. The parent still displays the established errand outcome and completed history; the opened child displays a completed session after confirmed cleanup. Opening it adds no runtime. Explicit continuation returns that child to normal running/attention states, then closes again after settlement. Desktop panel/Open/Back and phone drawer/full-screen child/Back retain their current composition.

Use production SQLite/service integration with controlled physical-provider boundaries for settlement/stop/receipt failures, restart, cancellation precedence and successor races. Exercise both ready and process-exit events. Reuse PostgreSQL store conformance for the extended intent codec and record unavailable DSNs as skips. Real browser checks prove retained transcript access, completed status, explicit continuation and shared workspace continuity on desktop and phone; mocked runtime checks do not certify real-provider restoration.

## Implementation package

[Completion cleanup plan and work order](../../../plans/errand-completion-cleanup/plan.md).
