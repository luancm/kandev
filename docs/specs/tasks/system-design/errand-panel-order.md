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

Partition the existing task-scoped errands by `isLiveErrand`, then by `parent_session_id`. Render active sections before finished sections. Sort each parent group by `created_at` descending, breaking equal or invalid timestamps deterministically by errand ID; order parent groups by their newest valid creation time descending with parent ID as tie-breaker. Put invalid creation times after valid times. Do not mutate store arrays or use `updated_at`, which would reorder rows when telemetry changes. Preserve parent headings in both sections, localize section headings across shipped locales, and use accessible section headings. This is presentation ordering only; backend admission and queue semantics stay unchanged.

The desktop popover and phone drawer share the same derivation. The existing grouped drawer remains the mobile exemplar, retaining its one internal list scroller, fixed Stop all footer, safe-area clearance and 44px row targets. A live-to-finished change moves the row across sections; stable IDs preserve row identity where React permits. Validate multiple parents, ties, invalid dates, mixed statuses, live transitions and no reorder on context updates on desktop and mobile.
