---
id: "04-persist-provider-source-identities"
title: "Persist exact provider source and base identities"
status: pending
wave: 4
depends_on: ["03-repair-watch-lifecycle"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 04: Persist exact provider source and base identities

## Acceptance

- GitHub TaskPR, GitLab TaskMR, and Azure DevOps TaskPR expose complete, credential-free source/head repository/ref and canonical base repository/ref identities at their existing provider seams.
- Each provider updates fresh DDL, additive migrations, explicit projection/scan lists, inserts/upserts, association, sync, list responses, and rebuild/copy paths. Azure conversion retains distinct source and target repository data instead of collapsing both into the target repository.
- GitHub's mock controller emits the same complete identity as real association so Playwright fixtures exercise production-shaped data.
- Lightweight or partial backend refreshes preserve a previously stored nonempty identity when the provider response omits that field; explicit validated replacement remains possible.
- GitHub and GitLab association, sync, and removal continue publishing task-change events. Azure gains an equivalent TaskPR update/removal event so Task 05 can refresh running comparison contexts without a restart.
- GitHub and GitLab WebSocket/store setters and Azure refresh-cache merging preserve nonempty source/head identity on a partial update. Their public TypeScript types represent omission distinctly.
- Migration tests build a legacy table, seed distinct source/base identities, run schema setup at least twice against the same database, force the applicable table rebuild/copy path, and prove rows, identities, constraints, and indexes survive.

## TDD sequence

1. Add failing provider-model/conversion tests for distinct source and base repositories, including nested GitLab paths, self-hosted hosts, and Azure source repository distinct from target.
2. Add failing legacy-schema tests that seed and replay setup on the same database, including GitHub's multi-repository table rebuild and equivalent additive migration replay for GitLab/Azure.
3. Add failing association/sync/mock tests for full identity and omission-preserving lightweight refresh.
4. Implement fresh DDL, migrations, rebuild/copy lists, projections, upserts, services, and mock payloads for all three providers.
5. Run `cd apps && pnpm install --frozen-lockfile`, then add failing frontend provider-state tests and implement lossless partial ingestion without adding external-link selection.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*(SourceIdentity|HeadRepository|SchemaReplay|TableRebuild|Mock)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/gitlab -run 'Test.*(SourceIdentity|TargetIdentity|SchemaReplay|TaskMR)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/azuredevops -run 'Test.*(SourceIdentity|TargetIdentity|SchemaReplay|TaskPR|ConvertPullRequest)' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/slices/github/github-slice.test.ts lib/ws/handlers/github.test.ts lib/state/slices/gitlab/gitlab-slice.test.ts lib/ws/handlers/gitlab.test.ts lib/state/slices/azure-devops/azure-devops-slice.test.ts hooks/domains/azure-devops/use-azure-devops-task-pull-requests.test.tsx)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-04-persist-provider-source-identities.md`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_taskpr_schema_drift_test.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/mock_controller_test.go`
- `apps/backend/internal/gitlab/models.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/gitlab/store_test.go`
- `apps/backend/internal/gitlab/store_source_identity_migration_test.go` (new)
- `apps/backend/internal/gitlab/service.go`
- `apps/backend/internal/gitlab/service_task_mr_link.go`
- `apps/backend/internal/gitlab/service_task_mr_link_test.go`
- `apps/backend/internal/gitlab/service_watches.go`
- `apps/backend/internal/azuredevops/client_models.go`
- `apps/backend/internal/azuredevops/client_models_test.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/rest_client_test.go`
- `apps/backend/internal/azuredevops/models.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/azuredevops/store_task_pr.go`
- `apps/backend/internal/azuredevops/store_test.go`
- `apps/backend/internal/azuredevops/store_source_identity_migration_test.go` (new)
- `apps/backend/internal/azuredevops/service_task_pr.go`
- `apps/backend/internal/azuredevops/service_task_pr_test.go`
- `apps/web/lib/types/github.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/lib/ws/handlers/github.ts`
- `apps/web/lib/ws/handlers/github.test.ts`
- `apps/web/lib/types/gitlab.ts`
- `apps/web/lib/state/slices/gitlab/types.ts`
- `apps/web/lib/state/slices/gitlab/gitlab-slice.ts`
- `apps/web/lib/state/slices/gitlab/gitlab-slice.test.ts`
- `apps/web/lib/ws/handlers/gitlab.ts`
- `apps/web/lib/ws/handlers/gitlab.test.ts`
- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/state/slices/azure-devops/types.ts`
- `apps/web/lib/state/slices/azure-devops/azure-devops-slice.ts`
- `apps/web/lib/state/slices/azure-devops/azure-devops-slice.test.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-task-pull-requests.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-task-pull-requests.test.tsx`

## Dependencies

Task 03 completes GitHub watch/store transition work before this task changes the overlapping TaskPR schema and service.

## Parallelism

Sequential because it owns shared provider contracts and update events consumed by Tasks 05, 06, 07, and 09.

## Output contract

Update only this task file's `## Results`. Report each provider's source/base shape, schema replay and rebuild evidence, partial-refresh rules, GitHub mock coverage, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
