---
spec: docs/specs/platform/git-remote-roles.md
created: 2026-08-12
status: in_progress
---

# Implementation Plan: Role-Aware Git Remotes

## Objective

Reconcile this branch with current `upstream/main`, then make attached repository, writable action head, tracking upstream, and comparison target explicit roles throughout Kandev. One agentctl resolver maps repository identities to executor-local remotes. A separate backend-owned comparison-context transport carries the selected repository/ref into every runtime before status and operations consume it.

The transport task is a sequential contract gate. No comparison-status or Git-operation work starts until comparison selection, launch metadata, restart recovery, live retargeting, agentctl configuration, and workspace updates share one tested contract.

## Behavioral sources

- [Role-Aware Git Remotes](../../specs/platform/git-remote-roles.md)
- [Workspace Git Status](../../specs/platform/workspace-git-status.md)
- [Task PR Automation Controls](../../specs/ui/ci-pr-automation.md)
- [Remote Contribution Tasks](../../specs/tasks/remote-contribution-tasks.md)
- [External VCS File Links](../../specs/ui/external-vcs-file-links.md)
- [ADR: Role-Based Git Remotes](../../decisions/2026-08-12-role-based-git-remotes.md)

## Required integration checkpoint

Task 00 fetches and merges the then-current `upstream/main`. After the merge, it performs a fresh path and ownership audit against this plan and every sibling task file. The primary conversation must reconcile any upstream path, API, schema, or ownership drift before releasing Task 01 or any later work. A pre-merge audit is evidence only and cannot waive this checkpoint.

## Architecture and ownership

### Remote-role resolver

Task 01 owns the provider-neutral resolver in `apps/backend/internal/agentctl/server/process` and its shared identity/result types. It normalizes credential-free provider, host, full repository path or provider ID, and literal ref; maps identities to executor-local remote names; and distinguishes writable action, tracking, and comparison evidence. Consumers do not parse URLs or assign meaning to `origin`.

### Comparison-context transport

Task 05 owns comparison selection and delivery from task/provider state to agentctl. It extends the existing base-branch path instead of creating a parallel selector: orchestrator launch/resume projection, backend adapters, lifecycle launch/restart/live-update paths, every executor, agentctl configuration and instance creation, process manager and workspace tracker, and the control/API endpoint all carry one per-repository comparison context keyed by deterministic worktree subpath. Selection precedence is a unique linked change whose exact source repository/ref matches the worktree's writable action head, then a validated remote-contribution target when no linked change competes, then attached repository plus selected base. Presence-aware observations distinguish omission from an explicit clear, incomplete refreshes cannot erase a known context, and no raw credential-bearing URL crosses the boundary.

### Consumers

Task 06 consumes the resolver and delivered context for merge-base, cumulative diff, and summary counts. Task 07 consumes the same context for Push, Pull, Rebase, Merge, and provider change creation. Both depend directly on Task 05.

### Provider persistence and external links

Task 04 owns provider identity persistence. It persists canonical base and exact source/head repository identities for GitHub, GitLab, and Azure DevOps, including fresh DDL, additive migration/rebuild paths, projections, scans, upserts, association, lightweight sync, mock fixtures, provider update/removal events, and lossless frontend provider-state ingestion. Task 09 consumes those fields to construct external file links but owns no provider schema or provider store. This separation keeps identity persistence independent of presentation.

### Frontend and mobile parity

Task 08 carries action, tracking, and comparison evidence through the existing WebSocket/store/hooks/view-model paths. Desktop and mobile controls consume the same semantics without compressing desktop UI into mobile. Task 10 owns focused Playwright coverage for desktop/mobile drift, triangular remotes, comparison counts, and provider-side file links.

## Dependency graph

```text
00 integrate upstream and re-audit
 |
01 resolve remote roles
 |
02 exact GitHub discovery
 |
03 watch lifecycle
 |
04 provider identity persistence
 |
05 transport comparison context
 |\
 | +--> 06 comparison status and counts --\
 | +--> 07 Git operations --------------+--> 08 frontend safety --\
 |                                                          |
 +-------------------------------------------------------> 09 external links
                                                              \           /
                                                               10 desktop/mobile E2E
                                                                         |
                                                                    11 documentation
                                                                         |
                                                                    12 verification
```

Task 05 is the sequential transport gate after the provider identities and events it consumes. Tasks 06 and 07 do not begin until it is integrated.

## Implementation waves

Wave 0:

- [x] [Task 00: Integrate upstream and re-audit](task-00-integrate-upstream.md) (`completed`)

Wave 1:

- [x] [Task 01: Resolve remote roles](task-01-resolve-remote-roles.md) (`completed`)

Wave 2:

- [x] [Task 02: Repair exact PR discovery](task-02-repair-exact-pr-discovery.md) (`completed`)

Wave 3:

- [x] [Task 03: Repair watch lifecycle](task-03-repair-watch-lifecycle.md) (`completed`)

Wave 4:

- [x] [Task 04: Persist provider source identities](task-04-persist-provider-source-identities.md) (`completed`)

Wave 5:

- [ ] [Task 05: Transport comparison context](task-05-transport-comparison-context.md) (`in_progress`)

Wave 6:

- [ ] [Task 06: Compute comparison status and counts](task-06-compute-comparison-status.md)

Wave 7:

- [ ] [Task 07: Route Git operations](task-07-route-git-operations.md)

Wave 8:

- [ ] [Task 08: Align frontend remote safety](task-08-align-frontend-remote-safety.md)

Wave 9:

- [ ] [Task 09: Resolve external file links](task-09-resolve-external-file-links.md)

Wave 10:

- [ ] [Task 10: Cover desktop and mobile role parity](task-10-cover-desktop-mobile-role-parity.md)

Wave 11:

- [ ] [Task 11: Document remote roles](task-11-document-remote-roles.md)

Wave 12:

- [ ] [Task 12: Verify the integrated package](task-12-verify-integrated-package.md)

Implementation is authorized by the Lunaldinho handoff session. Wave labels describe dependency and ownership constraints; shared seams remain serialized even when tasks are otherwise independent.

## Exact test map

| Behavior | File | How |
| --- | --- | --- |
| Upstream merge preserves writable-head and tracking snapshots together | `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go`, `apps/backend/internal/agent/runtime/agentctl/git_test.go`, `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'Test.*(HeadRemote|RemoteHead|ActionHead|Tracking)' -count=1` |
| Resolver handles custom names, push URL precedence, nested paths, literal ref case, absent refs, and ambiguity | `apps/backend/internal/agentctl/server/process/workspace_git_remote_test.go` (new) | `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'TestResolveGitRemoteRoles' -count=1` |
| Fresh launch plus single-repository and multi-repository resume retain comparison context before lifecycle metadata | `apps/backend/internal/orchestrator/executor/comparison_context_test.go` (new), `apps/backend/internal/backendapp/adapters_test.go`, `apps/backend/internal/agent/runtime/lifecycle/comparison_context_metadata_test.go` (new) | `cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'Test.*ComparisonContext.*(Launch|Resume|Metadata)' -count=1` |
| Comparison contexts survive full launch and executor metadata construction | `apps/backend/internal/agent/runtime/lifecycle/comparison_context_metadata_test.go` (new), `apps/backend/internal/agent/runtime/lifecycle/container_config_test.go`, `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_test.go`, `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_control_test.go`, `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_remote_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*ComparisonContext.*(Launch|Metadata|Standalone|Docker|SSH|Sprites)' -count=1` |
| Comparison contexts survive agentctl-ready restart and live retarget/association updates without erasure | `apps/backend/internal/agent/runtime/lifecycle/comparison_context_metadata_test.go` (new), `apps/backend/internal/agentctl/server/process/manager_update_comparison_contexts_test.go` (new), `apps/backend/internal/agentctl/server/api/workspace_comparison_contexts_test.go` (new), `apps/backend/internal/task/service/service_branch_update_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/task/service -run 'Test.*ComparisonContext' -count=1` |
| Provider association, sync, and removal events rehydrate every running execution without restart | `apps/backend/internal/orchestrator/event_handlers_comparison_context_test.go` (new), `apps/backend/internal/backendapp/comparison_contexts_test.go` (new) | `cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/backendapp -run 'Test.*ComparisonContext.*(Associate|Sync|Remove|Live)' -count=1` |
| Exact GitHub discovery keys include full head identity, paginate to uniqueness, dedupe, and preserve branch case | `apps/backend/internal/github/graphql_test.go`, `apps/backend/internal/github/service_pr_watch_batched_exact_head_test.go` (new), `apps/backend/internal/github/pat_client_test.go`, `apps/backend/internal/github/gh_client_test.go`, `apps/backend/internal/github/mock_client_test.go` | `cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*(ExactHead|Headless|Pagination|Ambiguous|BranchCase)' -count=1` |
| Searching retries, branch resets, terminal resets, and attached repository anchors remain coherent | `apps/backend/internal/github/store_pr_watch_remote_test.go`, `apps/backend/internal/github/service_pr_watch_anchor_test.go` (new), `apps/backend/internal/orchestrator/event_handlers_github_remote_head_test.go` | `cd apps/backend && go test -race -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(Watch|Searching|Reset|Anchor|RemoteHead)' -count=1` |
| Status and summary use the delivered comparison context rather than `origin` | `apps/backend/internal/agentctl/server/process/workspace_git_status_base_branch_test.go`, `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`, `apps/backend/internal/agentctl/server/process/comparison_base_test.go` (new), `apps/backend/internal/task/statussummary/projector_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/task/statussummary -run 'Test.*(Comparison|BaseBranch|Diff|Summary)' -count=1` |
| Comparison evidence crosses API, runtime, lifecycle, and orchestrator hops | `apps/backend/internal/agentctl/server/api/git_log_merge_base_test.go`, `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go`, `apps/backend/internal/agent/runtime/agentctl/git_test.go`, `apps/backend/internal/agent/runtime/agentctl/git_partial_test.go`, `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go`, `apps/backend/internal/orchestrator/event_handlers_git_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test.*(Comparison|BaseCommit|GitStatus)' -count=1` |
| Push, Pull, Rebase, Merge, and GitHub/GitLab/Azure creation use their assigned roles | `apps/backend/internal/agentctl/server/process/git_test.go`, `apps/backend/internal/agentctl/server/process/git_pr_providers_test.go` | `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'Test.*(Push|Pull|Rebase|Merge|CreatePR|CreateMR|Azure)' -count=1` |
| GitHub source identity persists through mock association and lightweight updates | `apps/backend/internal/github/store_taskpr_schema_drift_test.go`, `apps/backend/internal/github/service_pr_watch_test.go`, `apps/backend/internal/github/mock_controller_test.go` | `cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*(Source|HeadRepository|SchemaDrift|Mock)' -count=1` |
| Schema setup replay and legacy table rebuild use the same database and retain source/base identity | `apps/backend/internal/github/store_taskpr_schema_drift_test.go`, `apps/backend/internal/gitlab/store_source_identity_migration_test.go` (new), `apps/backend/internal/azuredevops/store_source_identity_migration_test.go` (new) | `cd apps/backend && go test -tags fts5 ./internal/github ./internal/gitlab ./internal/azuredevops -run 'Test.*(SchemaReplay|TableRebuild|SourceIdentity)' -count=1` |
| GitLab and Azure source/target identities survive association, refresh, and omission | `apps/backend/internal/gitlab/service_task_mr_link_test.go`, `apps/backend/internal/gitlab/store_test.go`, `apps/backend/internal/azuredevops/service_task_pr_test.go`, `apps/backend/internal/azuredevops/rest_client_test.go`, `apps/backend/internal/azuredevops/store_test.go` | `cd apps/backend && go test -tags fts5 ./internal/gitlab ./internal/azuredevops -run 'Test.*(Source|Target|HeadRepository)' -count=1` |
| Frontend provider association updates preserve nonempty source identity when lightweight updates omit it | `apps/web/lib/state/slices/github/github-slice.test.ts`, `apps/web/lib/ws/handlers/github.test.ts`, `apps/web/lib/state/slices/gitlab/gitlab-slice.test.ts`, `apps/web/lib/ws/handlers/gitlab.test.ts`, `apps/web/lib/state/slices/azure-devops/azure-devops-slice.test.ts`, `apps/web/hooks/domains/azure-devops/use-azure-devops-task-pull-requests.test.tsx` | `cd apps/web && pnpm exec vitest run lib/state/slices/github/github-slice.test.ts lib/ws/handlers/github.test.ts lib/state/slices/gitlab/gitlab-slice.test.ts lib/ws/handlers/gitlab.test.ts lib/state/slices/azure-devops/azure-devops-slice.test.ts hooks/domains/azure-devops/use-azure-devops-task-pull-requests.test.tsx` |
| Frontend hydration, partial update, reconnect, and aggregation keep role evidence separate | `apps/web/lib/ws/handlers/git-status.test.ts`, `apps/web/lib/state/slices/session-runtime/git-status-multi-repo.test.ts`, `apps/web/hooks/domains/session/use-session-git-summary.test.ts`, `apps/web/components/task/changes-panel-timeline-grouping.test.tsx` | `cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts hooks/domains/session/use-session-git-summary.test.ts components/task/changes-panel-timeline-grouping.test.tsx` |
| File links select provider source for head-side content and base for deleted/renamed-old content | `apps/web/lib/utils/external-vcs-file-url.test.ts`, `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx`, `apps/web/hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx`, `apps/web/components/editors/external-vcs-file-link.test.tsx`, `apps/web/components/diff/diff-header-toolbar.test.tsx`, `apps/web/components/review/review-diff-toolbar.test.tsx`, `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx` | `cd apps/web && pnpm exec vitest run lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/editors/external-vcs-file-link.test.tsx components/diff/diff-header-toolbar.test.tsx components/review/review-diff-toolbar.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx` |

## E2E test map

| Scenario | File | What it proves |
| --- | --- | --- |
| Desktop custom comparison remote with unrelated `origin` | `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts` | Sidebar and Changes use the canonical comparison merge-base and exclude already-landed fork commits. |
| Mobile custom comparison remote with unrelated `origin` | `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` | Mobile reports the same counts and action availability as desktop. |
| Provider head moves after local status | `apps/web/e2e/tests/git/git-changes-panel.spec.ts` and `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` (available after Task 00 upstream integration) | Push remains tied to the writable action-head snapshot, Pull remains tied to tracking, and refreshed drift renders consistently on desktop and mobile. |
| Triangular push/tracking/comparison remotes | `apps/web/e2e/tests/git/git-changes-panel.spec.ts` and `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` (available after Task 00 upstream integration) | Desktop and mobile never enable or route an action from the wrong role. |
| Fork source and deleted canonical-base links | `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts` and `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts` | Added/modified content opens the provider source repository while deleted/renamed-old content opens the canonical base on both form factors. |

## Documentation

Task 11 updates `docs/public/git-operations.md`, `docs/public/sessions-and-review.md`, and scoped agent guidance only after behavior and E2E semantics stabilize. It documents arbitrary remote names, role-specific operation routing, comparison-count behavior, ambiguity/fail-closed behavior, and remote-contribution materialization without assigning global meaning to `origin`.

## Ownership rules

- The primary conversation owns `plan.md` and dependency/status integration.
- Each implementation worker owns only the production/test files listed in its task and its own task file. It updates only its own `## Results` and reports evidence to the primary.
- Workers do not edit `plan.md` or another task file. Cross-owner failures return to the primary for routing.
- Task 12 is verification-only. It does not repair code, tests, docs, or task records.

## Risks

- `upstream/main` may move or rename APIs. Task 00's post-merge audit is a release gate for all later work.
- Several comparison-context paths currently transport only base branch names. Extending rather than duplicating that path avoids conflicting selection authorities.
- Multiple remotes may normalize to the same repository while exposing different refs. Ambiguity must fail closed instead of selecting by name or iteration order.
- An absent destination ref differs from an observation failure. First Push is safe only when destination identity is known and absence is proven.
- Provider light refreshes can omit source repository fields. Nonempty stored identity must not be erased by omission.
- SQLite schema rebuilds can appear correct on a fresh database while losing data during repeated setup. Same-database replay and rebuild coverage is mandatory.
- Partial WebSocket updates can erase nested evidence unless omission and reconnect behavior are explicit.
- Desktop and mobile can drift even with a shared store when controls derive availability separately. E2E covers both surfaces after provider head movement.

## Out of scope

- Inferring canonical fork parents without a linked change, validated remote-contribution binding, or explicit comparison selection.
- Adding fork badges, `isFork` persistence, remote-name-specific UI, or a remote-role settings page.
- One-click reconciliation for provider/local history divergence.
- Redesigning branch selection, desktop Changes layout, or mobile navigation.

## Verification results

Pending. Task 12 reports the integrated audit to the primary after all owners have completed and recorded their focused evidence.
