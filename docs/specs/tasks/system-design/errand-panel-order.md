---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRAND-ORDER-001
created: 2026-10-30
owners:
  - kandev
---

# Errand Panel Ordering Design

Select the rendered conversation's parent scope from the existing shared task cache as defined by [conversation navigation](errand-conversation-navigation.md), then partition by `isLiveErrand` and `parent_session_id`. Render active sections before an initially collapsed finished-history disclosure. Sort each parent group by `created_at` descending, breaking equal or invalid timestamps deterministically by errand ID; order parent groups by their newest valid creation time descending with parent ID as tie-breaker. Put invalid creation times after valid times. Do not mutate store arrays or use `updated_at`, which would reorder rows when telemetry changes. Preserve parent headings in both sections, localize section headings across shipped locales, and use accessible section headings. This is presentation ordering only; backend admission and queue semantics stay unchanged.

The desktop popover and phone drawer share the same derivation. The existing grouped drawer remains the mobile exemplar, retaining its one internal list scroller, fixed Stop all footer, safe-area clearance and 44px row targets. A live-to-finished change moves the row across sections; stable IDs preserve row identity where React permits. Validate switching among multiple parents without leaking rows, ties, invalid dates, mixed statuses, live transitions into collapsed history and no reorder on context updates on desktop and mobile. Shared derivation still accepts task-wide input, but the rendered parent panel passes only its own children.
