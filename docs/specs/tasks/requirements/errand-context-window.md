---
status: draft
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Errand Context Window Requirements

## Purpose and ownership

Show enough context-window information in the task-wide Errands panel to identify a child approaching its window limit without opening that conversation. The task system owns this extension to its grouped errand management surface. Existing [Errands requirements](errands.md) continue to own identity, lifecycle, access and delivered history; the [unmeasured-state](../../ui/requirements/context-window-unmeasured-state.md) and [reset-freshness](../../ui/requirements/context-window-reset-freshness.md) contracts supply the existing context-display semantics.

### REQ-TASKS-ERRANDS-CONTEXT-001: Context occupancy in errand rows

**Intent:** Let a user compare available context occupancy for individual errands while preserving the compact management flow.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-CONTEXT-001.1:** When an errand has a usable context-window reading, its row in the desktop Errands panel and phone drawer shall show compact localized used-token and window-capacity counts plus a passive usage bar for that child. Each row shall use its own child session's reading, including retained completed errands; context occupancy shall not be derived from the parent's context or cumulative token accounting.
- **AC-TASKS-ERRANDS-CONTEXT-001.2:** The readout shall follow the existing context-display semantics: unavailable or impossible reports are omitted, a known window with zero reported usage is explicitly unmeasured rather than numeric zero, and an exactly full window displays 100%. A newer sample shall update the row without opening the child. An explicit clear shall remove the old reading until a fresh sample; retained metadata shall restore the same information after reload.
- **AC-TASKS-ERRANDS-CONTEXT-001.3:** Showing context information shall remain passive: opening the Errands panel shall not open child chats, start or resume agents, create child-detail subscriptions, or introduce additional context requests or polling. Context occupancy shall not alter errand status, attention/live counts, cancellation or workflow ownership.
- **AC-TASKS-ERRANDS-CONTEXT-001.4:** Phone and coarse-pointer rows shall retain one Open target of at least 44px with readable context information and no nested interactive controls. Desktop keyboard access, narrow fine-pointer phone layouts, grouped list scrolling, the fixed bulk-action footer and viewport containment shall remain intact. Context information shall be accessible without hover; Open shall retain the existing full child view and Back path.

## Exclusions

No aggregate context counter, extra pill, context indicator in historical start/finished cards, new row action, automatic reset, warning notification, context-limit inference or new provider telemetry contract is introduced. Existing persisted telemetry may be included in the compact session summary needed for reload. Source and compaction details remain in the opened child's existing context display. Provider/executor support is inherited from existing session context reporting; missing telemetry remains unavailable.

## Presentation refinement, 2026-10-30

Measured rows show used tokens and capacity together (for example, `68k / 100k tokens`) with a thin passive bar. Accessible text supplies unambiguous counts and occupancy. Known capacity with unmeasured usage shows the capacity and explicit unmeasured wording, without a filled bar implying zero. Fresh dark-theme demo images and videos must seed representative child readings and visibly show these rows on desktop and phone; simulated readings must be identified as demo data.
