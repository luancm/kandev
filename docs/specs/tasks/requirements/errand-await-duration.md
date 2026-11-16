---
status: draft
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Errand Await Duration Requirements

### REQ-TASKS-ERRAND-AWAIT-DURATION-001: Show how long an await took

- **AC-TASKS-ERRAND-AWAIT-DURATION-001.1:** The completed chat entry for an errands await shows the actual elapsed time of that await when reliable duration data is available, on desktop and phone. The elapsed time is the wait operation itself, not an errand's age, configured timeout, or total agent turn duration.
- **AC-TASKS-ERRAND-AWAIT-DURATION-001.2:** A completed await with unavailable, invalid or legacy duration omits the duration without inventing a value. A zero or immediate wait may show zero. A completed card retains the same duration after reload; timeout and parent-ended successful responses report their actual elapsed wait.
- **AC-TASKS-ERRAND-AWAIT-DURATION-001.3:** The entry retains its errand list, navigation and error behavior. Duration is localized and accessible without hover. It adds no polling, per-errand clock, new control or change to await delivery semantics.

- **AC-TASKS-ERRAND-AWAIT-DURATION-001.4:** A completed wait with reliable outcome data identifies why it ended (all ready, first ready, timed out, or parent ended). Each listed errand shows its reported status and identifies results delivered by this await only when the response establishes that fact. Legacy or ambiguous outcomes are omitted rather than guessed.
