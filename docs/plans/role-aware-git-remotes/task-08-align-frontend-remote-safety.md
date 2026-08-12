---
id: "08-align-frontend-remote-safety"
title: "Align frontend status and remote-action safety"
status: pending
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

Pending.
