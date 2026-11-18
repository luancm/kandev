---
status: draft
system: tasks
created: 2026-10-31
owners:
  - kandev
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
---

# Errand conversation navigation and historical events

[Errands design map](errands.md#errands-design-map) lists shared contracts and companion ownership.

## Boundary and rationale

Normal task sessions are peers inside a task. An Errand owns its transcript, runtime and selection within its parent’s conversation surface. The parent remains an ordinary task session. The [Errand ownership decision](../../../decisions/2026-11-17-errand-owned-conversations.md) defines this boundary.

## Implementation

- **Conversation navigation.** Normal task sessions own regular tabs. `useOpenTaskSession` resolves an Errand by its task and Errand ID and hands the inspection to the parent’s conversation surface. The ordinary session selection remains the parent while the URL names the explicitly opened Errand. Direct links validate Errand ownership before loading the ordinary parent. Every saved, default and maximized layout restore applies the authoritative ordinary-session allowlist before panel materialization. If membership is unresolved, restoration holds session panels and admits non-session panels.
- **Embedded ownership.** Preview and Threads retain their own ordinary session selection. Their parent chat uses the same scoped Errand overlay and Back action as the task workbench.
- **Transcript history.** START, completed WAIT and COMPLETION share a muted borderless semantic disclosure with no repeated collapsed title. Stored start responses and delivered results remain historical snapshots; live lookups may supply navigation availability but cannot rewrite recorded statuses. Expand for title/profile/objective, outcomes/results and Open. Preserve Task27's measured duration, optional end reason, static completed-wait icons and returned-result markers. Keep events at their existing transcript positions and retain one visible chat scroller.
- **Errand view.** `errand-header.tsx` shows explicit Back to parent, objective, profile, status, workspace/branch, Stop while live or Send result when eligible. An owned transcript remains inspectable with the feature off. Actions follow the feature and lifecycle gates. The composer supports follow-up and MCP selection.
- **Chip/panel.** `errands-chip.tsx` uses the rendered chat's session identity to select that parent's outstanding rows and authoritative counts from the compact browser summary. `errands-panel.tsx` shows outstanding work and initially collapsed finished history in the existing popover/Drawer. Opening history requests bounded pages under the visible surface's owner; loaded rows are not a complete task inventory. Summary/history/inspection attempts share scoped ownership, while Stop all remains a task-wide server action. No duplicate inventory in transcripts. The parent composer remains enabled.
- **Task rows.** Board/sidebar/Threads continue consuming task-wide bounded `statusSummary.errands`. The ordinary session inventory supplies normal-conversation selectors.

## Navigation state and recovery

The active task session remains the ordinary parent while its Errand is open. Selecting another ordinary session leaves the current overlay, and Back reveals the retained parent. The URL preserves explicit links independently of ordinary selection. `replaceTaskConversationUrl` preserves other query parameters, hash and history state and only updates the matching task route. Layout restoration preserves ordinary panel geometry and honors explicit conversation links. Temporary filtering during unresolved membership stays out of saved layout preferences.

`TaskConversationPanel` resolves the known normal parent and `TaskConversationView` retains its complete mounted conversation region underneath the temporary child. On phones this region includes the Sessions picker and chat together; removing a picker outside the retained region would resize the covered transcript and disturb native reading position. The covered parent is invisible, inert and excluded from accessibility, rich detail reads, read tracking, announcements, recovery and workbench shortcuts. `TaskConversationChat` renders a chat body inside a region that already owns retention, avoiding nested retained copies. Back removes the child view and reactivates the same parent element and geometry, preserving its reading position, follow intent and unsent local state. The child remains the only visible transcript. Switching to a different normal conversation replaces the retained host; missing or invalid parents never cause an unrelated conversation to be retained.

`useRetainedReaderPosition` owns an above-bottom parent's visible message anchor after Back. It preserves the anchor through late browser correction and content reflow, coalescing work through the existing restoration scheduler. Native anchoring is suppressed only while this explicit reader owner is active. New user input is observed before child controls can stop propagation; explicit host/local targets, history generation changes, leaving the visible surface and unmount release the owner and restore its CSS, listeners and observers. Bottom followers and parents first mounted behind a direct child route retain their existing placement behavior. A missing anchor with changed history fails closed. Preview and Threads key their conversation body by task, so selecting a child cannot remount the retained parent; normal-session host keys still isolate local state between normal conversations.

A direct Errand link can display its owned transcript when the recorded parent is unavailable. Back is available when that ordinary parent is present.

## Phone composition

Reuse the current Errands bottom Drawer and mobile Sessions picker. Normal conversations stay in Sessions; delegated children stay in their parent's panel. Open leads directly to full-screen child chat with its Back header while the covered parent's picker and transcript retain their geometry; Back restores that parent. One visible list scroll owner, `dvh` containment, safe-area footer clearance and 44px phone/coarse-pointer actions preserve touch access. Shared bounded summary/history/inspection owners, selection and status models serve both viewports; no mobile fallback is persisted over desktop layout preferences.

## Verification

[Task29](../../../plans/errands-review-repairs/task-29-conversation-navigation-and-history.md) maps implementation and evidence to AC-TASKS-ERRANDS-010.2/010.4/010.7, 011.1/011.2/011.7 and 012.3. Pure navigation and mounted owner tests exercise parent hosting, sibling/foreign-scope isolation and restoration. Fresh desktop and native Pixel5 browser tests exercise many children and normal conversations, Open/Back, direct links/reload, embedded preview/Threads navigation, static expandable historical markers, drawer scrolling and containment. Preserve [Task27](../../../plans/errands-review-repairs/task-27-history-disclosure.md)'s measured WAIT semantics and browser proof. Rendering hidden tabs is never transcript deletion or runtime cleanup.

## Normal conversation selection

Summary-source menus in New Agent and New Subtask share one option projection and number sessions from the ordinary inventory. Route, preview and task-picker fallbacks select from the same membership. An explicit Errand link uses the compact inspection and owned transcript endpoints.

[Normal-session consumer repair](../../../plans/errand-session-consumers/plan.md) records summary, replacement and fallback regressions for AC-TASKS-ERRANDS-005.6/005.7.
