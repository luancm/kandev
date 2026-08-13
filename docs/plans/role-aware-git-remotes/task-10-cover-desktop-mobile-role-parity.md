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

Implemented the desktop/mobile parity coverage in the existing production-shaped E2E surfaces. `git-helper.ts` now materializes a triangular checkout with provider-shaped unrelated `origin`, writable/publish, tracking, and comparison remotes while preserving provider identities through local URL rewriting. Desktop and mobile Changes-panel specs assert the action, tracking, and comparison identities, role-derived counts, and writable Push safety. Existing provider-head rewrite scenarios now capture the initial provider head, move the mocked provider head, and reassert role-specific safety without mutating the local checkout on both form factors. Desktop and mobile sidebar specs assert `+1 -0` against the canonical comparison head, excluding the already-landed comparison commit. GitHub external-link fixtures now persist fork source and canonical base identities, materialize the fork as the writable action remote, and assert fork links for added/modified files plus canonical links for deleted and renamed-old files; the mobile file-link fixture covers all four statuses. GitLab desktop/mobile create-and-autolink specs now inspect the persisted task-MR row for distinct fork source and canonical target identities. The existing workflow IDs, workflow-step IDs, repository IDs, executor profiles, and `prevent_auto_start_agent_on_open: false` fixture behavior were preserved.

Changed files: `apps/web/e2e/helpers/api-client.ts`, `apps/web/e2e/helpers/git-helper.ts`, `apps/web/e2e/tests/git/git-changes-panel.spec.ts`, `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`, `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts`, `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`, `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts`, `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts`, `apps/web/e2e/tests/pr/pr-push-autolink.spec.ts`, `apps/web/e2e/tests/gitlab/gitlab-mr-creation.spec.ts`, and `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`.

Verification: `(cd apps && pnpm install --frozen-lockfile)` completed successfully. The managed Chromium runner completed the required backend, pseudo-locale Vite, and fixture production build after these corrections; the subsequent focused Chromium and mobile-chrome runs were attempted with `--no-build`. TypeScript typecheck, focused ESLint, and `git diff --check` passed. Both browser runs were blocked before test execution because the Playwright headless shell cannot load the host library `libnspr4.so` (`error while loading shared libraries: libnspr4.so: cannot open shared object file: No such file or directory`). No browser assertions ran in this environment.

Final fixture corrections keep provider-head rewrites keyed to the moved commit SHA, validate the persisted GitHub workspace/repository and head/base identities after reload, and configure GitLab's distinct writable fork before creation so the untouched auto-linked row carries exact source and target identities. External-link tests now require the live session worktree, mobile reopens the file after renamed-old navigation, and desktop/mobile sidebar tests share one canonical triangular identity while continuing to assert no repository pollution. The focused static checks were rerun successfully after these edits; browser execution remains blocked solely by the missing `libnspr4.so` host library.
