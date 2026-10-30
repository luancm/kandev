---
id: "16-context-token-readout"
title: "Show used tokens and context capacity with fresh dark media"
status: in_progress
wave: 17
depends_on:
  - "15-context-window"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-CONTEXT-001
acceptance_criteria:
  - AC-TASKS-ERRANDS-CONTEXT-001.1
  - AC-TASKS-ERRANDS-CONTEXT-001.2
  - AC-TASKS-ERRANDS-CONTEXT-001.3
  - AC-TASKS-ERRANDS-CONTEXT-001.4
system_design:
  - ../../specs/tasks/system-design/errand-context-window.md
---

# Task 16: Token counts and visible demo context

## Scope

Replace percentage-only row copy with compact used/capacity token counts and a passive thin bar. Preserve current data, missing/unmeasured/clear semantics and mobile interaction. No backend changes or additional telemetry requests. Own errands-panel-row.tsx, shared helper/tests if needed, relevant locale catalogs, focused existing desktop/phone specs, public coordination copy and disposable capture harness/media. The existing child widget stays unchanged.

## Acceptance

1. Desktop panel and native phone drawer show each child's used tokens and window capacity with readable compact formatting, passive bar and accessible counts. Known but unmeasured capacity is visible with explicit unmeasured wording; missing/invalid readings remain absent. Exact-full, update, reload and clear behavior remain covered.
2. Phone preserves one 44px row Open target, fixed footer, single scroller and no overflow or nested controls. Desktop keyboard behavior and passive subscription boundary remain intact.
3. New dark desktop/phone images and videos visibly show seeded context usage. Demonstrate parent follow-up and response while the errand remains running; identify synthetic telemetry and capture exact source SHA. Inspect artifacts and stop/release all capture processes.

## Preview

Desktop panel and phone drawer use the same row structure; phone Open is the whole row.

```text
Audit release checklist          Open (desktop)
Reviewer       Running
Context: 68k / 100k tokens
[ usage bar at 68%              ]
```

## Verification

Use TDD for formatter/model changes. Extend existing measured/unmeasured/update/reload/clear browser assertions for counts and bar. Run from apps/web:

```bash
pnpm exec vitest run lib/context-window-usage.test.ts components/task/chat/token-usage-display.test.tsx hooks/domains/session/use-session-context-window.test.ts components/task/chat/errands/errands-chip.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Run exact-file ESLint for changed TS/TSX. From root, run sequentially:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies and risks

Task15 complete. Localize new copy in every supported locale. Shared rows require sequential ownership. Capture uses an isolated current-task-revision demo, dark theme, synthetic metadata and real UI interaction. Existing capture relay failure must be resolved before claiming the parent-follow-up video. Do not reuse older images without visible context.


## Results and remaining work

Implementation is present. Focused formatter/ordering tests pass 6/6 and existing chip/widget/hook tests pass 34/34. Typecheck, exact-file lint and i18n checks passed. Desktop Errands suite passed 8/8 (`/tmp/kandev-run.e2e.H64y3Jis.log`); final phone suite passed 6/6 (`/tmp/kandev-run.e2e.YzSKukDM.log`), including context updates/reload/clear and await history. The initial phone viewport assertion assumed oldest-first order; scrolling to each target fixed that fixture assumption.
Fresh dark images/video visibly showing seeded context and parent conversation during a running errand remain outstanding. This work order remains `in_progress`; existing older captures are not acceptance for the refined readout.
