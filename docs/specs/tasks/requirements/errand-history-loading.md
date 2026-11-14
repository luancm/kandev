---
status: draft
system: tasks
created: 2026-11-12
owners:
  - kandev
---

# Errand history loading

The task system owns the retained parent-child relationship and its inspection surfaces. This vertical contract refines [Errands](errands.md) and [conversation navigation](../system-design/errand-conversation-navigation.md), under the [Errands ADR](../../../decisions/2026-10-23-errands-first-class-delegation.md). Ordinary conversations remain peers; existing ordinary detail laziness remains unchanged.

### REQ-TASKS-ERRAND-HISTORY-001: Bounded retained-history inspection

Users can inspect any retained Errand without paying the cost of every finished child during ordinary task navigation. Finished means no live work, outstanding result, or unresolved recovery; a terminal status alone is insufficient. History loading does not settle work, acknowledge results, or release runtimes.

- **AC-TASKS-ERRAND-HISTORY-001.1:** Zero-Errand tasks and ordinary siblings on tasks with Errands preserve current boot/list contents for ordinary sessions, tab order, selection, direct ordinary links, saved geometry and maximized layouts, reload/reconnect/task-switch behavior and ordinary subscription lifetime. No conversion of normal sessions to lazy summaries and no history reads on these paths are permitted.
- **AC-TASKS-ERRAND-HISTORY-001.2:** Initial navigation exposes compact live, queued and attention summaries and authoritative task/parent counts, capacity, pending-result and unread indicators independently of loaded history. Terminal children with undelivered results or unresolved recovery remain visible as outstanding work. Stop all acts on the complete authoritative task population, including unloaded children, using its existing cancellation eligibility.
- **AC-TASKS-ERRAND-HISTORY-001.3:** Finished summaries load only when the user opens history, with bounded explicit next-page reads and newest-created-first deterministic ordering. Mount, reconnect and background refresh never enumerate history. Loading, empty, unavailable, retry, changed-history and end-of-history states are distinguishable. Retained summaries and child detail caches remain bounded after repeated navigation.
- **AC-TASKS-ERRAND-HISTORY-001.4:** Opening one child loads only that conversation and minimal parent identity. Direct historical child URLs preserve that child through reload without enumerating history, creating an ordinary child tab or selecting an unrelated ordinary conversation. Missing parents preserve authorized child inspection with an explicit unavailable-parent state. Retained read-only inspection works with the Errands flag off, while mutations retain existing gates.
- **AC-TASKS-ERRAND-HISTORY-001.5:** No saved-layout restoration path materializes an Errand as an ordinary panel, including maximized/pre-maximized layouts and the interval before role identity resolves. Unknown identities cannot be assumed ordinary. Ordinary panels become available through the existing ordinary list without scanning child history.
- **AC-TASKS-ERRAND-HISTORY-001.6:** Delayed task, parent, cursor, connection or authorization-scope responses cannot overwrite newer state, resurrect deleted children or leak another scope. Continuation, delivery, deletion and recovery move children between outstanding and history truthfully; reconnect refreshes bounded live data and invalidates stale history without eagerly refilling it. Internal delivery, cancellation, recovery and MCP inventories remain complete.
- **AC-TASKS-ERRAND-HISTORY-001.7:** Desktop popover and phone drawer expose the same counts and demand history actions. Phone Open navigates to full-screen child chat and Back restores the parent. A single internal list scroller, safe areas, dynamic viewport containment, fixed Stop-all footer, 44px touch controls, keyboard/focus access and localized loading/error/next-page text remain available without horizontal overflow.
- **AC-TASKS-ERRAND-HISTORY-001.8:** Saved result cards remain readable, including recorded duration, outcome and legacy omissions, without child detail/history reads. Summary transport omits transcript, full result text and MCP attachment detail; explicit transcript inspection retains those existing capabilities.
- **AC-TASKS-ERRAND-HISTORY-001.9:** Before production edits, record a reproducible isolated baseline with fixed ordinary sessions and zero, 100 and 1000 finished children, stable build/network/cache conditions, repeated-run variance and numeric timing/memory acceptance tolerances. Post-change runs measure the same scenarios. Closed-history boot/list row processing, panel mounts and requests remain independent of finished-history size; no unmeasured speedup claim qualifies as acceptance.

## Exclusions and delivery

No general asset compression, bundle splitting, settings-page repair, provider rewrite, normal-session loading redesign, runtime cleanup replacement, or changes to delivery/settlement authority. Existing successful-runtime cleanup is verified separately. [Design](../system-design/errand-history-loading.md) and [implementation package](../../../plans/errand-history-loading/plan.md) define the bounded follow-up; no implementation is claimed.
