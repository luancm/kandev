---
created: 2026-11-02
status: draft
requirements:
  - REQ-TASKS-ERRANDS-005
system_design:
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
legacy_specs: []
---

# Repair normal-session consumers

## Overview

The summary menus map every session to indistinguishable profile labels; preview deletion and several route/task-picker fallbacks select raw first rows. Errands share the session collection, so these consumers must distinguish automatic normal-conversation choices from intentional child access. The user requested planning and execution together; implement sequentially after recording this package.

## Scope and technical approach

Share the summary option projection between New Agent and New Subtask, filtering by the existing errand classifier before ordering/numbering. Reuse the existing conversation classifier for automatic route, preview and desktop/phone fallback choices. Preserve raw session hydration, explicit child IDs, remembered selection, pending-input routing, runtime/workflow ownership and existing empty-state behavior. No backend, persistence, rollout or layout change is needed. Copy initial prompt continues to refer to the explicitly viewed conversation.

## ASCII UI preview

UI-01: Existing desktop dialog and phone dialog retain their composition and scroll owner. Only eligible options change; existing phone controls retain their touch targets.

```text
Context: Blank
Copy initial prompt
Summarize session
  1 Normal conversation A
  2 Normal conversation B
```

UI-02: Closing/deleting the selected normal preview session selects a remaining normal conversation, or the existing empty view. A running errand remains reachable through its parent panel and direct URL, but is not an automatic replacement.

## Tests and E2E

Task01 owns mixed-role summary options, contiguous numbering, flag-off and cached partial identity, and exact explicit summary selection. Task02 owns mixed/errands-only automatic defaults, explicit child selection, pending-input preservation, and deletion selection races. Extend existing desktop and phone Errands browser tests with a summary-menu check; existing Open/Back and ownership tests remain compatibility controls. Unit/component RED is the deterministic pre-fix gate; browser checks exercise the real shared picker after rebuilding.

## Work orders

- [x] [Task01: Filter summary context sources](task-01-summary-sources.md)
- [x] [Task02: Fence automatic conversation selection](task-02-automatic-selection.md)

## Verification results

Complete. Eight pre-fix behavioral assertions failed across all six affected entry points (`/tmp/errand-consumers-red.log`). Final focused verification passed 142 tests in eight files (`/tmp/errand-consumers-unit-final.log`). Scoped ESLint and formatting passed without warnings; web typecheck and rich-output contract checks passed with `NODE_OPTIONS=--max-old-space-size=6144` (`/tmp/errand-consumers-typecheck.log`). The first typecheck exhausted Node's default 2 GiB heap; it was rerun with available host memory, without changing code or skipping checks.

Fresh managed desktop build and final retries-disabled desktop/phone acceptance passed one test per viewport, no skips. Final logs: `/tmp/kandev-run.e2e.N2xI620R.log` and `/tmp/kandev-run.e2e.gWl0TF9A.log`. Both prove the real summary menu excludes real child sessions and explicit child transcript navigation remains available. The first phone attempt passed menu assertions but timed out on an optional Cancel cleanup step; the final test navigates directly to the retained child instead. No dialog-dismissal claim is inferred from that attempt. Existing focused handoff tests preserve exact explicit source selection and Blank initialization.

Catalog validation passed (422 decisions, 1,862 specifications), all specification lint passed, 63 public-doc tests passed, and 54 published pages validated. Full feature-range documentation coverage passed (728 changed paths, 88 reference documents); whitespace checks passed. Existing Task24 acceptance remains historical; this packet does not reopen dark media work. No PostgreSQL/provider behavior changes.

## Risks

Do not filter the shared session collection or reject explicit child selection. Do not hide input-needed errands. No-normal-session cases must not manufacture a child target. Preserve the preview deletion callback's current-selection guard.
