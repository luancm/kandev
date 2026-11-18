---
status: current
system: tasks
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
created: 2026-11-03
owners:
  - kandev
---

# Completed errand execution cleanup

[Errands design map](errands.md#errands-design-map) lists shared contracts and companion ownership.

## Boundary and existing behavior

The [completion requirements](../requirements/errand-completion-cleanup.md) apply to Errand-owned conversations. The [task-owned worktree decision](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md) governs the workspace. Completion releases execution resources. The durable transcript and native conversation binding remain available for explicit continuation.

| Acceptance | Owner                                                              |
| ---------- | ------------------------------------------------------------------ |
| .1         | Exact terminal settlement and durable close obligation             |
| .2         | Confirmed physical close of the recorded execution                 |
| .3         | Admission arbitration and explicit same-conversation continuation   |
| .4         | Startup and periodic retries of recorded obligations                |

## Durable close authority

An `ErrandTeardownIntent` with completion purpose records the exact task, Errand, execution generation and ID, turn, submission and prompt generation. Settlement persists the obligation alongside the exact terminal decision and result or no-result disposition. SQLite and PostgreSQL use the same versioned representation. Parent result reception and runtime absence remain independent observations.

The completion and cancellation paths use the recorded execution identity and confirmation boundary. Cancellation that overtakes completion retains its own decision and outcome. Result delivery, an unrelated historical receipt, and UI status confer no close authority.

## Ordering and races

Persist settlement and teardown authority before external calls. Revalidate the target under the owning operation guard and stop the recorded execution through the runtime. The runtime uses its authenticated binding to resolve retained execution evidence and closes its own stream, client and lifecycle indexes. The task owns its workspace. Publish the outcome outside the operation guard.

The next execution generation is admitted only after the recorded predecessor teardown has completed. A late callback can confirm only its recorded operation. Missing or uncertain runtime evidence keeps the intent pending.

## Recovery and explicit continuation

Reuse the existing bounded startup and errand teardown sweep, including flag-off cleanup of already accepted work. Retry pending completion intents without rerunning provider prompts or result delivery. Provider-stop, inventory and receipt failures leave durable work pending; neither scheduling nor elapsed time proves absence.

The task-prompt-authorized `errand.continue` action uses a client message ID to reserve one user turn and successor generation together. Admission requires completed teardown of the exact previous execution and its retained native binding. The launch checks the recorded predecessor against the selected runtime and attached task workspace before it loads that same native conversation. An exact retry returns the original turn when prompt acceptance or terminal projection is durable. A user-origin settlement leaves the original parent result and its delivery receipt intact.

The authenticated ordinary parent addresses an owned Errand through the existing explicit `session_id` argument of `message_task_kandev`. The successor reservation saves the parent, turn and submission in the existing admission ledger. Durable prompt acceptance or the exact projected terminal receipt authorizes the parent result transition. Recovery can recognize those saved receipts and finish the admission without dispatching a second prompt.

## Presentation and validation

Opening a completed Errand reads its owned transcript and leaves the execution idle. An explicit follow-up may restore the same native conversation and starts a new owned turn. The existing desktop panel and phone drawer open the same Errand view with Back to its ordinary parent.

Use production SQLite/service integration with controlled physical-provider boundaries for settlement/stop/receipt failures, restart, cancellation precedence and successor races. Exercise both ready and process-exit events. Reuse PostgreSQL store conformance for the extended intent codec and record unavailable DSNs as skips. Real browser checks prove retained transcript access, completed status, explicit continuation and shared workspace continuity on desktop and phone; mocked runtime checks do not certify real-provider restoration.

## Implementation package

[Completion cleanup plan and work order](../../../plans/errand-completion-cleanup/plan.md) and [accepted integration evidence](../../../plans/errands-remaining/final-integration-handoff.md). The behavior-preserving [simplification package](../../../plans/errands-simplification/plan.md) strengthens contract coverage before making the installed controller capabilities explicit; it does not change these ownership or recovery rules.
