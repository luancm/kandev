---
id: "21-preview-navigation"
title: "Keep errand navigation inside its preview"
status: done
wave: 21
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-011.7
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 21: Keep errand navigation inside its preview

## Scope and current contract

Repairs review finding 15 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. Task29 owns the overlapping production implementation; this packet closes Task21 acceptance against the approved temporary-conversation navigation. Normal conversations remain preview tabs, while Open displays the child inside the task-scoped preview and Back restores its parent without changing primary or workflow ownership.

The phone follows the existing Kanban direct-to-task route and uses its full-screen child view with Back. A desktop hover preview or child entry in the regular phone picker is not part of the accepted contract.

## Acceptance mapping

- The real preview browser scenario opens an MCP-created errand from both the parent chip and its transcript entry, shows exactly one child conversation, retains the parent element, returns to the parent with Back, and never renders a child tab.
- The scenario selects Plan, reselects the parent tab, and then opens the child, covering the preview's local tab/Plan selector. It checks that the parent remains after Back and that the persisted primary session is unchanged.
- The embedded `useOpenTaskSession` owner test verifies Open and Back are routed through the owner without calling global `setActiveSession` or adding a Dockview panel. The scoped-selector test verifies a foreign task and a missing controlled callback are no-ops.
- The phone acceptance remains the existing full-screen child/Back path; no desktop preview is mounted on phone and saved desktop preview preferences are not rewritten.

## Results

Task21 is done. No production source change was needed; Task29 already implements the accepted temporary child view and scoped preview owner. The preview E2E was extended with the Plan-to-parent path and the Task22 child-action assertions, while Task29's implementation and remaining integration stay with their existing owners.

- Fresh managed chromium E2E, zero retries: 1/1 passed for `task29 preview owns errand Open and Back without adding a child tab`, `/tmp/kandev-run.e2e.jAuaVBKV.log` (wrapper `/tmp/errands-task21-22-preview-e2e.log`).
- Adjacent navigation/menu/header unit suites: 77 tests in 5 files passed, `/tmp/errands-task21-22-unit.log`; includes preview Plan behavior, scoped owner behavior, foreign/missing callback no-op, shared menu role controls, and phone ErrandHeader actions.
- Task29 current-production desktop acceptance: 11/11 passed, including the real preview scenario and retained flag-off route, `/tmp/kandev-run.e2e.CAUesc1l.log`.
- Task29 current-production native Pixel5 acceptance: 8/8 passed, including full-screen child navigation, Back, reader continuity, and touch controls, `/tmp/kandev-run.e2e.RnwKOqu5.log`.
- ESLint passed with zero warnings for the two changed frontend test files, `/tmp/errands-task21-22-eslint.log`; Prettier check passed, `/tmp/errands-task21-22-prettier.log`.

The first browser invocation was rejected before discovery because it ran from the repository root with pnpm 12 against the apps workspace's pnpm 9.15.9 requirement. The managed run from `apps/web` used the pinned pnpm, reused verified Task29 production assets with `--no-build`, and passed. No unrelated workspace state, shared manifest, public docs, production code, or live instance was changed in this packet.
