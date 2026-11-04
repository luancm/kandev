---
status: draft
system: tasks
created: 2026-11-03
owners:
  - kandev
---

# Completed errand sessions

## Scope

The task system owns the lifetime of an errand conversation and its relationship to delegated results. Successful completion releases idle execution resources while retaining the conversation for inspection and explicit continuation. This extends [Errands](errands.md); cancellation, failed-turn handling and [result delivery](errand-result-delivery.md) retain their existing contracts.

### REQ-TASKS-ERRAND-COMPLETION-001: Close successful errand execution

**Intent:** A finished errand should not leave an idle agent runtime or appear to be waiting for input merely to keep its transcript accessible.

#### Acceptance criteria

- **AC-TASKS-ERRAND-COMPLETION-001.1:** After a successful errand turn is durably settled and no successor has taken execution ownership, the system shall close that turn's runtime and release its execution resources automatically. A parent-initiated turn's result, or a user-initiated turn's no-result decision, shall be saved before closing. Closing shall not cancel the errand, emit a cancellation notice, discard or duplicate its result, or wait for the parent to receive that result.
- **AC-TASKS-ERRAND-COMPLETION-001.2:** After successful close or authoritative confirmation that the matching runtime is absent, the retained session shall report `COMPLETED`. Its transcript, result, context history and resume identity shall remain available on desktop and phone. The completed errand remains accessible through existing Open and Back navigation. Cleanup shall not change task/workflow state, parent/sibling sessions, or the shared workspace.
- **AC-TASKS-ERRAND-COMPLETION-001.3:** An explicit user follow-up or authorized parent message shall be able to resume the same completed errand conversation through existing admission and recovery controls. Inspection alone shall not launch an agent. A delayed completion or cleanup attempt shall never stop, overwrite or mark completed a newer turn or replacement session. Parent-result supersession and user-result opt-in remain as defined by Errands.
- **AC-TASKS-ERRAND-COMPLETION-001.4:** Failed or interrupted cleanup shall remain retryable after restart, including while new Errands admission is disabled. Cleanup shall not be reported as confirmed from a timeout or scheduling alone. Existing retained successful sessions shall converge to `COMPLETED` only when their exact latest settled turn, unchanged session identity and runtime absence are established; uncertain or superseded evidence shall not be fabricated into authority.

## Exclusions

No session/transcript deletion, worktree removal, new global session-state value, new periodic worker, or changes to ordinary-session idle behavior. Failed and cancelled errands retain their existing outcome semantics. Native provider restoration limits remain explicit rather than promising universal provider resume support.

## Design and implementation

- [System design](../system-design/errand-completion-cleanup.md)
- [Implementation package](../../../plans/errand-completion-cleanup/plan.md)
