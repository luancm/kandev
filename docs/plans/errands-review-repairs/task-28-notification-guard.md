---
id: "28-notification-guard"
title: "Avoid notification re-entry during terminal settlement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.11
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 28: Notification completion guard

Incident evidence is in `/tmp/errands-stalled-delivery-investigation.md`. Parent completion holds its session cancellation guard and passes a guard-ownership context through settlement. Notification admission queues the result then synchronously requests readiness, which acquires that same non-reentrant guard. The queued notification survives but its caller cannot finish settlement. This is a conformance repair under the existing approved implementation; no new product behavior.

Own `apps/backend/internal/orchestrator/errand_parent_deliverer.go` and new `errand_notification_guard_test.go`. Follow the existing `autoStartAgentCallback.drainIfTurnEnded` guard convention: when the context proves this exact session's guard is already held, leave draining to the completion owner; unguarded callers retain the immediate readiness recheck. No detached goroutine, new queue or unchecked cross-session guard bypass. Preserve Auto-run and incarnation checks.

Behavioral regression: admit while the real guard is held, require return without dispatch, then release and run readiness to prove queued delivery. Cover both supported guard-context types; keep existing unguarded immediate-dispatch and Auto-run-off controls. RED uses a bounded deadline solely to detect deadlock and always releases the guard before cleanup.

## Results

- RED: the two same-session ownership cases failed behaviorally as expected because admission waited on its caller's held guard (`/tmp/errands-task28-red.log`).
- Cross-session control: both workflow and cancellation markers still triggered readiness on the original implementation (`/tmp/errands-task28-control.log`).
- GREEN: focused race suite passed 6 cases: both same-session markers, both unrelated-session controls, unguarded immediate dispatch, and Auto-run-off retention (`/tmp/errands-task28-green.log`).
- Changed-file lint passed with `0 issues` for `errand_parent_deliverer.go` and `errand_notification_guard_test.go` (`/tmp/errands-task28-lint.log`).
