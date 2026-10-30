---
status: draft
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Errand Panel Ordering Requirements

### REQ-TASKS-ERRAND-ORDER-001: Active errands first

The task-wide Errands pill panel and phone drawer show active errands above finished errands across all parent sessions. Within each of those two sections, errands are grouped by parent session and sorted newest created first within each parent group. Parent groups are ordered by their newest errand in the section, newest first. A parent with both active and finished errands appears in both sections so no finished row precedes an active row.

- **AC-TASKS-ERRAND-ORDER-001.1:** Queued, running and needs-attention errands are active; done, failed and cancelled errands are finished. Both desktop and phone use the same ordering and retain parent identity for each row.
- **AC-TASKS-ERRAND-ORDER-001.2:** Within each section and parent group creation time determines newest first; ties use a deterministic errand ID order. Updates to status move a row to its appropriate section immediately, while unrelated metadata and context updates do not change ordering. Reload restores the same order.
- **AC-TASKS-ERRAND-ORDER-001.3:** Ordering preserves existing Open, Stop all, accessible grouping, mobile single scroller and fixed footer behavior. It does not alter lifecycle or backend queue ordering.
