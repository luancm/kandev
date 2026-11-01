---
status: draft
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Errand Panel Ordering Requirements

### REQ-TASKS-ERRAND-ORDER-001: Active errands first

The parent conversation's Errands panel and phone drawer show that parent's active errands above an initially collapsed completed-history section. Each section sorts newest created first and retains parent identity. Switching normal conversations switches the parent scope without leaking another parent's rows. Task-wide summary counts and Stop all semantics remain as defined by the Errands contract.

- **AC-TASKS-ERRAND-ORDER-001.1:** Queued, running and needs-attention errands are active; done, failed and cancelled errands are finished. Both desktop and phone use the same ordering and retain parent identity for each row.
- **AC-TASKS-ERRAND-ORDER-001.2:** Within each section and parent group creation time determines newest first; ties use a deterministic errand ID order. Updates to status move a row to its appropriate section immediately, while unrelated metadata and context updates do not change ordering. Reload restores the same order.
- **AC-TASKS-ERRAND-ORDER-001.3:** Ordering preserves existing Open, Stop all, accessible grouping, mobile single scroller and fixed footer behavior. It does not alter lifecycle or backend queue ordering.
