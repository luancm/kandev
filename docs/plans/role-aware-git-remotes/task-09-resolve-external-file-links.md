---
id: "09-resolve-external-file-links"
title: "Resolve external file links from exact provider sides"
status: pending
wave: 9
depends_on: ["04-persist-provider-source-identities", "08-align-frontend-remote-safety"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 09: Resolve external file links from exact provider sides

## Acceptance

- Added, modified, untracked, and renamed-new content uses the exact source/head repository and ref persisted by the linked GitHub, GitLab, or Azure DevOps change.
- Deleted and renamed-old content uses the exact canonical base repository/ref. When no linked change supplies a canonical base, unlinked base-side content may use the accepted comparison repository/ref for GitHub, GitLab, or Azure DevOps; any incomplete or ambiguous identity fails closed.
- Provider adapters preserve self-hosted GitLab origin, complete Azure organization/project/repository identity, nested paths, literal refs, and credential stripping.
- Missing, ambiguous, cross-host, or incomplete head-only identity fails closed instead of combining a head branch with the attached/base repository.
- Editor, diff, review, and mobile file actions consume the same resolver result and retain keyboard/touch reachability without overflow or layout changes.
- Upstream-added `workflowId` fixture fields remain present while hook fixtures gain source/base identity.

## TDD sequence

1. Run `cd apps && pnpm install --frozen-lockfile`.
2. Add failing URL utility tests for all providers, source/base side selection, rename direction, unsafe identity, self-hosted GitLab, and Azure query shape.
3. Add failing hook tests for exact provider association matching, partial/incomplete identity, multi-repository ambiguity, and accepted comparison fallback on GitHub, GitLab, and Azure DevOps.
4. Update toolbar consumers only as required to pass the common resolved target through, preserving existing interaction primitives.
5. Run focused tests, typecheck, lint, i18n checks, and ratchet.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/editors/external-vcs-file-link.test.tsx components/diff/diff-header-toolbar.test.tsx components/review/review-diff-toolbar.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-09-resolve-external-file-links.md`
- `apps/web/lib/utils/external-vcs-file-url.ts`
- `apps/web/lib/utils/external-vcs-file-url.test.ts`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx`
- `apps/web/components/editors/external-vcs-file-link.tsx`
- `apps/web/components/editors/external-vcs-file-link.test.tsx`
- `apps/web/components/diff/diff-header-toolbar.tsx`
- `apps/web/components/diff/diff-header-toolbar.test.tsx`
- `apps/web/components/review/review-diff-toolbar.tsx`
- `apps/web/components/review/review-diff-toolbar.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`

## Dependencies

Task 04 supplies omission-safe provider identities. Task 08 supplies the accepted comparison evidence consumed by unlinked base-side fallback.

## Parallelism

Sequential after Task 08 because the hook consumes its final frontend comparison shape.

## Output contract

Update only this task file's `## Results`. Report source/base selection by provider and file status, fail-closed cases, files changed, pnpm installation outcome, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Implemented the shared external-file resolver and workspace hook around explicit provider-neutral source/head, canonical base, and accepted comparison identities. Added/modified/untracked, modified, and renamed-new files require the exact source repository/ref; deleted and renamed-old files require the exact canonical base repository/ref, with comparison identity available only for complete accepted unlinked base-side evidence. GitHub, self-hosted GitLab, and Azure DevOps URL construction preserve nested paths, literal refs, organization/project/repository identity, credential stripping, and Azure query shape. Partial, missing, identity-less, ambiguous, mismatched, cross-host, and incomplete identities fail closed; Azure SSH action hosts normalize to canonical organization identities, Azure comparison identities preserve their explicit organization, and comparison links use the provider target ref. Editor, Changes diff, Review, and mobile consumers continue to receive the common resolver result and existing touch/keyboard primitives.

Files changed: `apps/web/lib/utils/external-vcs-file-url.ts`, `apps/web/lib/utils/external-vcs-file-url.test.ts`, `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts`, `apps/web/hooks/domains/workspace/use-external-vcs-file-link-candidates.ts`, `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx`, and this Results section. No toolbar production changes were required because all consumers already pass the common hook input; upstream `workflowId` fixtures remain untouched.

`(cd apps && CI=1 pnpm install --frozen-lockfile)` completed successfully. RED evidence: the new exact source/base utility and hook tests initially failed by resolving the attached repository/base branch. GREEN evidence: the focused suite now passes 108 tests across the seven files named by this task. `(cd apps/web && pnpm exec vitest run lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/editors/external-vcs-file-link.test.tsx components/diff/diff-header-toolbar.test.tsx components/review/review-diff-toolbar.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx)` passed.
`(cd apps && CI=1 pnpm install --frozen-lockfile)` completed successfully. RED evidence: the final regressions initially allowed SSH Azure action mismatches, executor-local comparison refs, incomplete linked candidate sets, and attached-repository head fallback. GREEN evidence: the focused suite now passes 121 tests across the seven files named by this task. `(cd apps/web && pnpm exec vitest run lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/editors/external-vcs-file-link.test.tsx components/diff/diff-header-toolbar.test.tsx components/review/review-diff-toolbar.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx)` passed.

`(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)` passed. The same typecheck without the heap setting hit the environment's Node heap limit (exit 134), so the successful command is recorded with the repository's established heap setting. `(cd apps && pnpm --filter @kandev/web lint)` reaches only existing repository warnings plus new complexity/size warnings in this resolver/test work and exits due to the repository-wide `--max-warnings 0` policy; the focused ESLint run for the changed production files reports no errors. `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)` passed with advisory locale parity notices. `git diff --check` passed.
`(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)` passed. The same typecheck without the heap setting hit the environment's Node heap limit (exit 134), so the successful command is recorded with the repository's established heap setting. `(cd apps/web && pnpm exec eslint lib/utils/external-vcs-file-url.ts hooks/domains/workspace/use-external-vcs-file-link.ts hooks/domains/workspace/use-external-vcs-file-link-candidates.ts --max-warnings 0)` passed cleanly. `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)` passed with advisory locale parity notices. `git diff --check` passed.
