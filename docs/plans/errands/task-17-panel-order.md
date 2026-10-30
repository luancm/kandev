---
id: "17-panel-order"
title: "Order active errands before finished errands"
status: in_progress
wave: 18
depends_on:
  - "16-context-token-readout"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-ORDER-001
acceptance_criteria:
  - AC-TASKS-ERRAND-ORDER-001.1
  - AC-TASKS-ERRAND-ORDER-001.2
  - AC-TASKS-ERRAND-ORDER-001.3
system_design:
  - ../../specs/tasks/system-design/errand-panel-order.md
---

# Task 17: Order errands in the pill

## Scope

Partition active and finished errands globally, then retain parent groups within each section. Sort newest created first in each group, and order groups by newest child. Use shared presentation logic for desktop and phone; do not alter store arrays or backend queue behavior. Likely files: errands-panel.tsx, related hooks/selectors, adjacent tests, existing desktop/mobile errands specs and locale catalogs.

## Acceptance

1. Queued, running and needs-attention rows appear above all done, failed and cancelled rows across parents; each section preserves parent identity. Newest-created ordering and deterministic ties match the design.
2. Status changes move rows to the correct section; context and other unrelated updates leave their relative order stable. Reload and both viewport paths agree.
3. Existing accessible groups, Open, Stop all, 44px phone targets, single scroller and footer remain functional; no new request or mutation is introduced.

## Desktop and phone preview

```text
Active
  Parent A
    Newest running errand
    Older queued errand
  Parent B
    Running errand
Finished
  Parent A
    Newest completed errand
    Older failed errand
[Stop all] (fixed footer on phone)
```

## Verification

Add meaningful ordering and transition unit tests; extend existing desktop and mobile Errands browser scenarios to verify mixed-parent ordering and transition. From apps/web run the focused relevant Vitest tests, exact-file ESLint, typecheck, i18n:check and i18n:ratchet. From repository root run:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies and risks

Follow task16 to avoid overlapping edits to shared panel and browser fixtures. Translate any new headings in every supported locale. Preserve grouped parent information even when one parent spans both sections; no extra data fetch or polling.


## Results and remaining work

Implementation is present. Focused formatter/ordering tests pass 6/6 and existing chip/widget/hook tests pass 34/34. Typecheck, exact-file lint and i18n checks passed. Desktop Errands suite passed 8/8 (`/tmp/kandev-run.e2e.H64y3Jis.log`); final phone suite passed 6/6 (`/tmp/kandev-run.e2e.YzSKukDM.log`), including context updates/reload/clear and await history. The initial phone viewport assertion assumed oldest-first order; scrolling to each target fixed that fixture assumption.
Explicit mixed-parent ordering and transition browser coverage remains outstanding. Pure ordering tests cover multiple parents, status partition, ties and transition; this work order remains `in_progress`.
