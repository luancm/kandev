---
id: "02-automatic-selection"
title: "Fence automatic conversation selection"
status: done
wave: 2
depends_on: ["01-summary-sources"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
system_design:
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.6
---

# Fence automatic conversation selection

## Scope and acceptance

Implement the corresponding boundary in [the plan](plan.md), preserving explicit child transcript access and existing workflow ownership. Cover normal/errand mixtures, errands-only inputs and retained-role behavior independent of the release toggle. No backend or layout changes.

## Files likely touched

All paths under `apps/web/`: `lib/routing/resolve-task-route.ts`, `components/task/session-sort.ts`, `task-select-helpers.ts`, `preview-session-tabs.tsx`, `mobile/session-task-switcher-sheet-selection.ts` and their existing tests.

## ASCII UI preview

Use UI-02 in [the plan](plan.md#ascii-ui-preview). Desktop and phone retain the current controls and composition; source lists and automatic selection are shared data behavior.

## Verification

From repository root with pnpm 9.15.9 on PATH:

```bash
pnpm --dir apps/web exec vitest run lib/routing/resolve-task-route.test.ts components/task/session-sort.test.ts components/task/task-select-helpers.test.ts components/task/preview-session-tabs.test.tsx components/task/mobile/session-task-switcher-sheet-archived-selection.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped ESLint and Prettier on changed web files. Task01 browser acceptance uses `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --grep 'normal context sources' --retries=0` and the same command for `mobile-chrome` / `mobile-errands.spec.ts`. Build once through the managed runner and reuse only that exact build for the phone run.

## Dependencies and parallelism

Task01; sequential. Primary session owns implementation and integration.

## Results

Complete. The exact commands above passed as part of the final 142-test focused suite; scoped lint, formatting, typecheck, documentation and coverage gates also passed. Desktop and phone browser acceptance each passed one test without retries or skips. See [the manifest receipts](plan.md#verification-results) for logs, the behavioral RED evidence and validation limitations.
