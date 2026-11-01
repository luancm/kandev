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

## Boundary and rationale

The task system owns the parent-child relationship and selected conversation; no backend identity, permissions, transcript or workspace changes are required. Normal conversations are peers inside a task. Delegated work is inspected within its parent, so retained children do not compete for regular tabs. Existing role/ownership isolation remains authoritative in [the Errands design](errands.md). This local presentation choice is sufficiently recorded here and in its acceptance criteria; no separate ADR is needed.

## Implementation

- **Conversation navigation.** Normal task sessions own regular tabs. `runAutoSessionTabEffect` resolves a selected errand to its normal parent tab, removes legacy child panels after authoritative session hydration and never creates a regular child tab, independent of the release toggle. `useOpenTaskSession` selects the child conversation while focusing its parent surface. Dockview chat content resolves the selected child only within its parent's panel; switching normal tabs restores normal conversations. Route/session selection remains the child for reload and direct links. A missing parent retains direct child access in the generic chat surface rather than manufacturing workflow ownership.
- **Embedded ownership.** `PreviewSessionTabs` provides `TaskSessionNavigationContext` scoped to its task and routes Open/Back through the same session/Plan selector; missing callbacks and foreign targets are no-ops. Threads retains its existing per-column owner. Neither embedded path selects another workbench or changes primary. Mobile and Threads regular selectors omit errands by persisted role, including retained sessions while disabled.
- **Transcript history.** START, completed WAIT and COMPLETION share a muted borderless semantic disclosure with no repeated collapsed title. Stored start responses and delivered results remain historical snapshots; live lookups may supply navigation availability but cannot rewrite recorded statuses. Expand for title/profile/objective, outcomes/results and Open. Preserve Task27's measured duration, closed end reason, legacy omission behavior, static completed-wait icons and returned-result markers. Keep events at their existing transcript positions and retain one visible chat scroller.
- **Errand view.** `errand-header.tsx` shows explicit Back to parent, objective, profile, status, workspace/branch, Stop while live or Send result when eligible. Persisted role supplies retained navigation even with the feature off; actions retain existing feature/lifecycle gates. The composer supports follow-up and MCP selection.
- **Chip/panel.** `errands-chip.tsx` uses the rendered chat's session identity to select that parent's children from the shared task cache. `errands-panel.tsx` shows active work and initially collapsed completed history in the existing popover/Drawer, preserving the shared request owner and task-wide Stop-all action. No duplicate inventory in transcripts. The parent composer remains enabled.
- **Task rows.** Board/sidebar/Threads continue consuming task-wide bounded `statusSummary.errands`; normal-conversation selectors omit child sessions.

## Navigation state and recovery

The actual selected session remains the child in task route/store state. `conversationTabSessionId` maps that selection to a normal host and `panelConversationSessionId` restricts temporary child content to that host. A normal tab selection exits the child view; Back selects the exact parent. These render-only projections never rewrite persisted primary/workflow state. `replaceTaskConversationUrl` mirrors explicit Open/Back and normal-tab/picker selections into the current task URL without remounting the transcript; it preserves other query parameters, hash and history state and never updates a foreign task, preview or Threads route. Normal-tab ordinal badges and counts exclude errands. A late session role must invalidate tab membership. Prune restored child panels only after the session list is authoritative, including panels that bypassed automatic creation. Keep the displayed child across layout restoration; a stale restored peer must not override an explicit child route, including before role hydration identifies it.

`TaskConversationPanel` resolves the known normal parent and `TaskConversationView` retains its complete mounted conversation region underneath the temporary child. On phones this region includes the Sessions picker and chat together; removing a picker outside the retained region would resize the covered transcript and disturb native reading position. The covered parent is invisible, inert and excluded from accessibility, rich detail reads, read tracking, announcements, recovery and workbench shortcuts. `TaskConversationChat` renders a chat body inside a region that already owns retention, avoiding nested retained copies. Back removes the child view and reactivates the same parent element and geometry, preserving its reading position, follow intent and unsent local state. The child remains the only visible transcript. Switching to a different normal conversation replaces the retained host; missing or invalid parents never cause an unrelated conversation to be retained.

`useRetainedReaderPosition` owns an above-bottom parent's visible message anchor after Back. It preserves the anchor through late browser correction and content reflow, coalescing work through the existing restoration scheduler. Native anchoring is suppressed only while this explicit reader owner is active. New user input is observed before child controls can stop propagation; explicit host/local targets, history generation changes, leaving the visible surface and unmount release the owner and restore its CSS, listeners and observers. Bottom followers and parents first mounted behind a direct child route retain their existing placement behavior. A missing anchor with changed history fails closed. Preview and Threads key their conversation body by task, so selecting a child cannot remount the retained parent; normal-session host keys still isolate local state between normal conversations.

If the recorded parent is missing, retain the child transcript in an available normal surface or generic chat surface. Do not manufacture a parent, promote a child, or erase history. Role identity is independent of the rollout flag; `isAutoSelectionExcludedErrand` in the WebSocket session handler excludes retained errands from automatic adoption, replacement selection and terminal-event handoff in both flag states. Disabled actions remain subject to existing gates while transcript and Back navigation stay available.

## Phone composition

Reuse the current Errands bottom Drawer and mobile Sessions picker. Normal conversations stay in Sessions; delegated children stay in their parent's panel. Open leads directly to full-screen child chat with its Back header while the covered parent's picker and transcript retain their geometry; Back restores that parent. One visible list scroll owner, `dvh` containment, safe-area footer clearance and 44px phone/coarse-pointer actions preserve touch access. Shared cache, selection and status models serve both viewports; no mobile fallback is persisted over desktop layout preferences.

## Verification

[Task29](../../../plans/errands-review-repairs/task-29-conversation-navigation-and-history.md) maps implementation and evidence to AC-TASKS-ERRANDS-010.2/010.4/010.7, 011.1/011.2/011.7 and 012.3. Pure navigation and mounted owner tests exercise parent hosting, sibling/foreign-scope isolation and restoration. Fresh desktop and native Pixel5 browser tests exercise many children and normal conversations, Open/Back, direct links/reload, embedded preview/Threads navigation, static expandable historical markers, drawer scrolling and containment. Preserve [Task27](../../../plans/errands-review-repairs/task-27-history-disclosure.md)'s measured WAIT semantics and browser proof. Rendering hidden tabs is never transcript deletion or runtime cleanup.
