---
id: "22-preview-menu-isolation"
title: "Hide ownership actions for retained errands"
status: done
wave: 22
depends_on:
  - "21-preview-navigation"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.2
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 22: Hide ownership actions for retained errands

## Scope and current contract

Repairs review finding 16 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. Task29 owns the overlapping production implementation; this packet closes Task22 acceptance against the approved temporary-conversation navigation. Errands have no regular session tab, so the old child-tab right-click path is superseded and must not be restored for testing.

The selected child exposes its actual conversation controls: Back, Stop while live, and Send result when eligible. Neither the desktop header nor the phone actions menu offers Set as Primary or Handoff. Normal eligible secondary sessions retain those ownership actions as a positive control.

## Role protection

The shared `SessionContextMenuItems` role guard remains authoritative and independent of the release flag. `PreviewSessionTabMenu` derives `isErrand` from the retained session's `role`; it does not depend on the feature flag or the current errand list. The real rendered selectors keep role-marked children out of regular tabs, the phone Sessions picker, and reopen menus, including after the feature is disabled.

## Results

Task22 is done. No production source change was needed; Task29 already filters children from regular session selectors and the shared menu preserves the role guard. The actual temporary child surfaces and the ordinary-session positive control are covered without creating a child tab solely to expose an obsolete menu.

- The fresh preview E2E checks the rendered desktop child header has its allowed Stop action but no Set as Primary or Handoff, retains the parent and returns through Back; it also exercises Plan-to-parent reselection and confirms primary ownership, `/tmp/kandev-run.e2e.jAuaVBKV.log` (wrapper `/tmp/errands-task21-22-preview-e2e.log`).
- The existing real shared-menu unit controls verify an errand omits Set as Primary and Handoff while retaining lifecycle actions, and an ordinary eligible secondary exposes both ownership actions. The phone ErrandHeader test now asserts its rendered actions menu omits both; adjacent suites passed 77 tests across 5 files, `/tmp/errands-task21-22-unit.log`.
- Current-production Task29 browser acceptance passed 11/11, including retained role access with the flag off, parent Back navigation, no child session tab, and exclusion from the reopen menu, `/tmp/kandev-run.e2e.CAUesc1l.log`.
- Current-production native Pixel5 acceptance passed 8/8, including the phone child header, Back, and its 44px Stop action, `/tmp/kandev-run.e2e.RnwKOqu5.log`.
- ESLint passed with zero warnings for the two changed frontend test files, `/tmp/errands-task21-22-eslint.log`; Prettier check passed, `/tmp/errands-task21-22-prettier.log`.

The full feature-off route receipt is `two clients remove deleted errands and flag-off restart retains detached sessions` in the Task29 desktop run; the role stays `errand`, the transcript and Back remain accessible, and it stays out of regular/reopen entries. No backend/API/DB change, shared manifest edit, public-doc edit, or live-instance mutation was made in this packet.
