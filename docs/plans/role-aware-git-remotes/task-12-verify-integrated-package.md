---
id: "12-verify-integrated-package"
title: "Verify the integrated role-aware package"
status: pending
wave: 12
depends_on: ["11-document-remote-roles"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 12: Verify the integrated role-aware package

## Acceptance

- Verification starts from the integrated worktree after Tasks 00 through 11 have recorded focused evidence.
- Run the backend focused suites, tagged race suite, frontend unit/type/lint/i18n checks, and focused desktop/mobile E2E listed below without changing production code, tests, public docs, `plan.md`, or another task file.
- Inspect changed paths against task ownership and confirm every plan/task link, dependency ID, own-task-file entry, and planned real/new path remains coherent.
- Classify every failure by owning task and report it to the primary. Do not repair, loosen, skip, snapshot-update, or reassign a failure in this task.
- Update only this task's Results with commands, environment constraints, pass/fail totals, and owner routing.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/common/gitremote ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor ./internal/task/service ./internal/task/statussummary ./internal/github ./internal/gitlab ./internal/azuredevops -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/github ./internal/gitlab ./internal/azuredevops -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts lib/state/slices/github/github-slice.test.ts lib/ws/handlers/github.test.ts lib/state/slices/gitlab/gitlab-slice.test.ts lib/ws/handlers/gitlab.test.ts lib/state/slices/azure-devops/azure-devops-slice.test.ts hooks/domains/azure-devops/use-azure-devops-task-pull-requests.test.tsx hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/session/use-session-git-derived.test.ts hooks/domains/session/use-session-git-summary.test.ts lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/task/mobile/session-mobile-top-bar-git-controls.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts tests/task/sidebar-diff-stats.spec.ts tests/review/external-vcs-file-link.spec.ts tests/pr/pr-push-autolink.spec.ts tests/gitlab/gitlab-mr-creation.spec.ts tests/gitlab/gitlab-mr-push-autolink.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-external-vcs-file-link.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts)
```

## Failure routing

- Resolver or normalization failures: Task 01.
- Exact GitHub discovery failures: Task 02.
- GitHub watch transition/race failures: Task 03.
- Provider schema/model/state/mock failures: Task 04.
- Launch, resume, restart, live-update, agentctl config/API, or context-presence failures: Task 05.
- Comparison status/count failures: Task 06.
- Push/Pull/Rebase/Merge/create routing failures: Task 07.
- Frontend status/action/classifier failures: Task 08.
- External file target failures: Task 09.
- Desktop/mobile E2E fixture or parity failures: Task 10.
- Public documentation or scoped guidance failures: Task 11.
- Cross-owner integration or ownership-audit failures: primary conversation.

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-12-verify-integrated-package.md`

## Dependencies

Task 11 completes the last implementation/documentation owner. All earlier dependencies are transitive.

## Parallelism

Sequential and verification-only. Stop after reporting failures to the primary; owners perform fixes in their own tasks.

## Output contract

Update only this task file's `## Results`. Report exact commands, environment constraints, pass/fail totals, ownership/path/link audit results, and each failure mapped to its owner to the primary. Do not edit `plan.md`, production code, tests, public docs, or another task file.

## Results

Pending.
