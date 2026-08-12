---
id: "10-cover-desktop-mobile-role-parity"
title: "Cover desktop and mobile remote-role parity"
status: pending
wave: 10
depends_on: ["08-align-frontend-remote-safety", "09-resolve-external-file-links"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 10: Cover desktop and mobile remote-role parity

## Acceptance

- Desktop and mobile E2E fixtures create an unrelated `origin` plus custom comparison, writable, and tracking remotes, and assert identical role-derived counts and action safety.
- Extend the existing provider-head rewrite scenarios so a provider head moving after local status leaves Push tied to the writable action snapshot, Pull tied to tracking, and remote mutation disabled only by the correct evidence on both form factors.
- Desktop and mobile sidebar scenarios assert corrected comparison additions/deletions and exclude already-landed fork commits.
- GitHub and GitLab create/autolink scenarios prove the exact writable source and canonical target are sent and persisted; existing remote-contribution credential and force-push restrictions remain covered.
- Desktop and mobile external-link scenarios prove fork-source added/modified targets and canonical-base deleted/renamed-old targets for production-shaped provider mocks.
- The existing `prevent_auto_start_agent_on_open: false` fixture behavior and widened workflow-step API options from upstream remain intact.

## TDD sequence

1. Run `cd apps && pnpm install --frozen-lockfile`.
2. Extend the existing desktop drift, mobile drift, sidebar, create/autolink, and external-link specs with failing role-aware assertions. Create a new comparison fixture/spec only if the existing files cannot express the setup cleanly, and record why.
3. Update E2E fixtures/helpers only where necessary to materialize provider source/base identities and triangular remotes without bypassing production handlers.
4. Run focused Chromium and mobile-chrome specs through the managed runner; use `--no-build` only after one successful production build in the same worktree.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts tests/task/sidebar-diff-stats.spec.ts tests/review/external-vcs-file-link.spec.ts tests/pr/pr-push-autolink.spec.ts tests/gitlab/gitlab-mr-creation.spec.ts tests/gitlab/gitlab-mr-push-autolink.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-external-vcs-file-link.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-10-cover-desktop-mobile-role-parity.md`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` (available after Task 00 upstream integration)
- `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts`
- `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts`
- `apps/web/e2e/tests/pr/pr-push-autolink.spec.ts`
- `apps/web/e2e/tests/gitlab/gitlab-mr-creation.spec.ts`
- `apps/web/e2e/tests/gitlab/gitlab-mr-push-autolink.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

Tasks 08 and 09 must finish the shared frontend semantics and link resolution before E2E locks desktop/mobile parity.

## Parallelism

Sequential. These specs share backend/frontend builds, mock provider state, and E2E fixtures.

## Output contract

Update only this task file's `## Results`. Report each desktop/mobile scenario, production-shaped fixture changes, files changed, pnpm installation outcome, first-build and focused-run results, and failures to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
