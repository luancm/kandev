---
id: "08-align-frontend-remote-safety"
title: "Align frontend status and remote-action safety"
status: completed
wave: 8
depends_on: ["06-compute-comparison-status", "07-route-git-operations"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 08: Align frontend status and remote-action safety

## Acceptance

- Git-status types, WebSocket handling, session runtime state, partial updates, reconnect, and multi-repository aggregation keep writable action, tracking upstream, and comparison evidence separate.
- Push availability and provider/local-history classification compare provider head with the writable action-head snapshot. Pull consumes tracking evidence. Changes/review scope and sidebar counts consume comparison evidence.
- A partial status update cannot erase a role it omits, and a repository update cannot affect a sibling worktree.
- Desktop Changes and mobile Changes/top-bar controls consume one shared derived view model and expose equivalent role-safe actions without changing mobile navigation, scroll ownership, or touch layout.
- Upstream-added remote-contribution relation hooks are retained and adjusted, while `changes-panel-data.tsx` remains the shared production view-model seam.

## TDD sequence

1. Run `cd apps && pnpm install --frozen-lockfile`.
2. Add failing hydration, omission, reconnect, multi-repository, provider-drift, and shared-view-model tests.
3. Extend the real Git status types/slice/hook paths and update the provider/local-history classifier to writable action evidence.
4. Make desktop and mobile controls consume the shared derived result, then run focused tests, typecheck, lint, i18n checks, and ratchet.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/session/use-session-git-derived.test.ts hooks/domains/session/use-session-git-summary.test.ts components/task/changes-panel-timeline-grouping.test.tsx components/task/mobile/session-mobile-top-bar-git-controls.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-08-align-frontend-remote-safety.md`
- `apps/web/lib/types/git-events.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/lib/ws/handlers/git-status.test.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-multi-repo.test.ts`
- `apps/web/hooks/domains/session/remote-contribution-relation.ts` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/remote-contribution-relation.test.ts` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/use-session-git-derived.ts` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/use-session-git-derived.test.ts` (available after Task 00 upstream integration)
- `apps/web/hooks/domains/session/use-session-git.ts`
- `apps/web/hooks/domains/session/use-session-git-summary.ts`
- `apps/web/hooks/domains/session/use-session-git-summary.test.ts`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/components/task/changes-panel-timeline-grouping.test.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx` (available after Task 00 upstream integration)
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx` (available after Task 00 upstream integration)

## Dependencies

Tasks 06 and 07 establish the status and operation semantics projected into this shared view model.

## Parallelism

Sequential before Task 09 because external links consume its accepted comparison evidence.

## Output contract

Update only this task file's `## Results`. Report state invariants, classifier and action mapping, desktop/mobile shared-view-model coverage, files changed, pnpm installation outcome, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Implemented and tested the frontend remote-role safety boundary. Git status derivation now keeps writable action-head, explicit tracking-upstream, comparison target, and remote-role generation evidence independent; unknown, unresolved, ambiguous, absent, missing-head, and missing-generation role observations fail closed for the corresponding controls. Push and provider contribution classification use the writable action head, Pull requires an explicit present tracking upstream, Rebase and Merge require resolved comparison evidence, and comparison-based Changes/review/sidebar counts do not fall back from unresolved comparison snapshots. Omission-retaining WebSocket state and repository-scoped aggregation remain covered, while desktop and mobile Git operations now consume the shared session model and per-repository menus apply the same role gates. Updated the shared relation hooks, session derived/summary models, Changes header/menu, mobile top bar/dropdown, session info comparison totals, and focused tests; `changes-panel-data.tsx` remains the shared production seam. `pnpm install --frozen-lockfile` passed. RED/GREEN evidence is recorded by 81 focused Vitest tests passing across WS hydration, multi-repository isolation, relation/provider drift, derived summaries, comparison/sidebar totals, timeline grouping, and mobile controls; `pnpm run typecheck` passed; `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. The full web lint command reaches only six existing Task07 warnings in `apps/web/hooks/use-git-operations.ts` (`max-params`, `max-lines-per-function`, `complexity`, and nested ternaries); all Task08-touched files pass targeted ESLint with zero warnings. `git diff --check` passed.
