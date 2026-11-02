---
id: "01-summary-sources"
title: "Filter summary context sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
system_design:
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.7
---

# Filter summary context sources

## Scope and acceptance

Implement the corresponding boundary in [the plan](plan.md), preserving explicit child transcript access and existing workflow ownership. Cover normal/errand mixtures, errands-only inputs and retained-role behavior independent of the release toggle. No backend or layout changes.

## Files likely touched

All paths under `apps/web/`: `components/task/new-session-dialog.tsx`, `new-subtask-dialog.tsx`, shared summary option hook and tests, existing desktop/phone Errands browser specs.

## ASCII UI preview

Use UI-01 in [the plan](plan.md#ascii-ui-preview). Desktop and phone retain the current controls and composition; source lists and automatic selection are shared data behavior.

## Verification

From repository root with pnpm 9.15.9 on PATH:

```bash
pnpm --dir apps/web exec vitest run components/task/new-session-dialog.test.tsx components/task/new-subtask-dialog.test.tsx
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped ESLint and Prettier on changed web files. Task01 browser acceptance uses `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --grep 'normal context sources' --retries=0` and the same command for `mobile-chrome` / `mobile-errands.spec.ts`. Build once through the managed runner and reuse only that exact build for the phone run.

## Dependencies and parallelism

None; sequential. Primary session owns implementation and integration.

## Results

Complete. The exact commands above passed as part of the final 142-test focused suite; scoped lint, formatting, typecheck, documentation and coverage gates also passed. Desktop and phone browser acceptance each passed one test without retries or skips. See [the manifest receipts](plan.md#verification-results) for logs, the behavioral RED evidence and validation limitations.
