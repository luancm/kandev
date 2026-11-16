---
id: "00-upstream-baseline"
title: "Integrate the canonical upstream baseline"
status: done
wave: 8
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-009
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-005.2
  - AC-TASKS-ERRANDS-009.2
  - AC-TASKS-ERRANDS-012.1
  - AC-TASKS-ERRANDS-012.2
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 00: Integrate the canonical upstream baseline

## Summary

Bring the current task branch up to date with `upstream/main` before changing
Errands behavior. Keep the existing feature history and reviewed design
package; `origin/main` is not the integration base for this fork.

## In scope

- Refresh and record the upstream and feature SHAs. The planning snapshot
  is feature `19df9464a3`, upstream `7f5e4d2115`, merge base `b6263ea4c3`:
  23 upstream-only commits and 51 feature-only commits.
- Integrate upstream on the continuation branch. Read-only merge analysis
  found conflicts only in `src/locales/*/chat.json`, in all eight catalogs.
  Retain the 14 Errands keys and seven upstream Quick Chat creation keys;
  reject duplicate JSON keys and preserve unrelated translations.
- Recheck the clean text merges in MCP, executor start-attempt ownership,
  early launch failure, session preparation, task operations, and chat panel
  composition. Keep the exact eight-tool child catalog and immutable role.
- Install workspace dependencies once. Record a fresh Linux baseline and
  distinguish failures reproduced on the exact upstream SHA from regressions.

## Out of scope

- Rewriting the published feature history, changing fork remotes, force
  pushing, publishing a PR, or fixing unrelated upstream failures.
- Changing any Errands shipped profile default.

## Acceptance

1. Refreshed `upstream/main` is an ancestor of the continuation head, both
   branches' locale additions survive, and reviewed/user edits are preserved.
2. Focused integration tests preserve explicit profiles, workflow isolation,
   launch ownership, terminal settlement recovery, MCP authority, and flag-off
   behavior on the merged base.
3. Results name exact SHAs, commands, failures, and any reproduced baseline
   issue; historical macOS failures are not used as current waivers.

## Verification

Run from the repository root after integrating upstream. Installation is a
one-time prerequisite for subsequent work orders, not a repeated gate.

```bash
git fetch upstream main
git merge-base --is-ancestor upstream/main HEAD
git rev-parse HEAD upstream/main
test "$(scripts/resolve-go-lint-base)" = refs/remotes/upstream/main
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers ./internal/mcp/server -run 'Errand|Priority|HandleUpdateTask_.*Launch|ExternalMCPViewerCannotChangeDeferredLaunchPrompt' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/orchestrator/executor -run 'Errand|ExistingWorkspaceStart|CancelledResumeStartup|SessionStartingRace|InitialAdmissionReceives|QuickChat|EarlyLaunchFailure|LaunchFailure|BootstrapFailure|CodexRateLimitRetryEpisode|CompletionFollowUp|CancelledTurn|PromptTask' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/backendapp ./internal/errands -run 'NativeResumePendingTerminal|RuntimeTurnChangeSummaryProcessing|Reconcile|Restart' -count=1)
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts lib/ws/handlers/agent-session.errand-adoption.test.ts components/task/dockview-session-tabs.hook.test.tsx)
(cd apps/web && pnpm exec vitest run lib/state/slices/errands lib/api/domains/errand-api.test.ts lib/ws/handlers/errands.test.ts hooks/domains/session/use-task-errands.test.ts hooks/domains/session/use-errand-actions.test.ts components/task/chat/errands components/task/chat/messages/kandev/errand-renderers.test.tsx components/task/mobile/mobile-sessions-section.errands.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Also parse each conflicted catalog with a duplicate-key-rejecting JSON reader;
ordinary JSON parsing alone accepts duplicate keys silently.

## Files likely touched

- `apps/web/src/locales/{en,ja,ko,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/chat.json`.
- Auto-merged `apps/backend/internal/mcp/{handlers,server}/`,
  `internal/orchestrator/{executor,session_launch.go,task_operations.go}`,
  task model/DTO/service files, and `apps/web/components/task/task-chat-panel.tsx`.
- This work order's Results and the parent plan's baseline record.

## Dependencies

None. This is the mandatory first continuation work order.

## Risks

- Upstream moves; repeat ancestry and affected-contract checks when it does.
- A clean textual merge does not prove launch/admission semantics.
- Do not reset, stash away, or discard the user's reviewed design package to
  make integration convenient.

## Parallelism

`sequential`

## Inputs

- [Continuation checkpoint](plan.md#continuation-checkpoint-2026-10-27).
- Existing work orders 01, 02, 04, and 06; scoped backend/web guidance.

## Results

### Completed integration (2026-10-27)

The user authorized the local upstream merge and continuation. The primary completed merge commit `69c190e0785bb21d0b750297125bcd9ef5ec07db` with parents `19df9464a303462fab5c46b4fa80f1d0d1ca9333` and `f150e6accd66c9cecdadb56d481180ebc9f75c7f`. `MERGE_HEAD` is gone and `git merge-base --is-ancestor upstream/main HEAD` passes. The committed 807 paths exactly match the inspected integration inventory. All 13 uncommitted design documents were byte-verified preserved before completion updates; none entered the merge commit. All three acceptance conditions are satisfied, and source ownership remains released.

Both normal commit hooks are active and passed without bypass. Initial Go lint package-loading timeout and subsequent compiler termination were resolved by warming Go export data for the task-service test dependencies (1,158 packages, compilation only), followed by a successful normal commit. Harness, architecture, gofmt, Go lint, Prettier, web lint, i18n, public-copy and commitlint hooks passed; documentation/specification and E2E-sleep hooks reported their actual skipped results where applicable. Additional required harness/specification validators and their tests passed. Receipt and failed-attempt evidence are in `/tmp/kandev-errands-merge-xfr6fjug/hook-receipt.json` and its sibling logs. No source workaround, hook bypass, push or PR was used. The worktree intentionally retains the design package and completion records as uncommitted changes.

### Prepared merge before authorization (historical, 2026-10-27)

At preparation, status was `in_progress`. Reversible integration was prepared, but the
HEAD-ancestry criterion requires a separately authorized merge commit. No
commit, push, PR, workflow completion signal, profile-default change, or live
instance mutation was performed.

- Branch: `feature/plan-errands-first-d-424`.
- Unchanged HEAD: `19df9464a303462fab5c46b4fa80f1d0d1ca9333`.
- Fetched `upstream/main` and retained `MERGE_HEAD`:
  `f150e6accd66c9cecdadb56d481180ebc9f75c7f`.
- Merge base: `b6263ea4c3165c86ec60e29e798475ad552ad6ce`;
  51 feature-only and 30 upstream-only commits before integration.
- `git merge --no-commit --no-ff upstream/main` initially returned 1 with
  exactly the eight expected `chat.json` conflicts. All were resolved and
  explicitly staged. There are zero unmerged paths; 807 integration paths
  are staged for inspection. The prepared merge is retained uncommitted.

All 13 original design documents were copied and hashed before integration.
Eleven other documents remain byte-identical. This order's status/results
are the specialist's only design edits; the primary's later manifest note
is preserved. No design documents were staged or overwritten. Full initial
and prepared path inventories, original copies, command receipts and logs
are in `/tmp/kandev-errands-baseline-m5146ci_`.

Each resolved catalog has 221 unique keys: all 14 Errands additions, seven
Quick Chat creation additions, and six other upstream recovery additions
survive with every upstream translation value intact. The scratch
`catalogs.py` performs three-way key/value reconciliation, rejects duplicate
JSON keys with `object_pairs_hook`, and tests the duplicate rejection
against an identical-value duplicate. All eight catalogs passed.

Semantic inspection covered the early fixed-catalog branch and final
eight-tool filter in MCP, immutable role/parent stamping and non-primary
session preparation, start-attempt ownership callback propagation, guarded
early launch failure, explicit-profile/workflow-detached launch paths,
authenticated pending-terminal proof and settlement fences, and upstream
`detailActive` forwarding in the chat panel. No manual production-logic or
test changes were needed. Only locale conflict resolutions were authored;
desktop/phone composition is unchanged. Browser proof remains work order 09.

### Verification evidence

Environment: Linux x86_64, Go 1.26.3, Node v24.16.0, pnpm 9.15.9. Existing
`GOFLAGS=-p=2` was retained. Frozen workspace installation ran once and
installed 934 packages without changing the lockfile. Each log below has a
sibling JSON receipt containing the exact command, working directory and
exit code; paths are relative to the scratch directory above.

| Exact verification command | Outcome | Log |
| --- | --- | --- |
| `git fetch upstream main` | Passed; refreshed from the planning snapshot to `f150e6accd` | `fetch.log` |
| `git merge-base --is-ancestor upstream/main HEAD` | Exit 1; pending the excluded merge commit | `ancestry.log` |
| `git rev-parse HEAD upstream/main` | Passed; exact SHAs recorded above | `refs.log` |
| `test "$(scripts/resolve-go-lint-base)" = refs/remotes/upstream/main` | Passed | `lint-base.log` |
| `(cd apps && pnpm install --frozen-lockfile)` | Passed; one installation | `install.log` |
| `(cd apps/backend && go test -trimpath ./internal/mcp/handlers ./internal/mcp/server -run 'Errand\|Priority\|HandleUpdateTask_.*Launch\|ExternalMCPViewerCannotChangeDeferredLaunchPrompt' -count=1)` | Both packages passed | `backend-mcp.log` |
| `(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/orchestrator/executor -run 'Errand\|ExistingWorkspaceStart\|CancelledResumeStartup\|SessionStartingRace\|InitialAdmissionReceives\|QuickChat\|EarlyLaunchFailure\|LaunchFailure\|BootstrapFailure\|CodexRateLimitRetryEpisode\|CompletionFollowUp\|CancelledTurn\|PromptTask' -count=1)` | Both packages passed on retry; initial process interrupted with exit 143 before results | `backend-orchestrator.log`, `backend-orchestrator-retry.log` |
| `(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/backendapp ./internal/errands -run 'NativeResumePendingTerminal\|RuntimeTurnChangeSummaryProcessing\|Reconcile\|Restart' -count=1)` | All four packages passed | `backend-recovery.log` |
| `(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)` | Three config tests failed with inherited port aliases; reproduced on exact upstream; all three packages passed with port aliases unset | `backend-flags.log`, `upstream-config.log`, `backend-flags-clean.log` |
| `(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts lib/ws/handlers/agent-session.errand-adoption.test.ts components/task/dockview-session-tabs.hook.test.tsx)` | 3 files, 30 tests passed | `frontend-contract.log` |
| `(cd apps/web && pnpm exec vitest run lib/state/slices/errands lib/api/domains/errand-api.test.ts lib/ws/handlers/errands.test.ts hooks/domains/session/use-task-errands.test.ts hooks/domains/session/use-errand-actions.test.ts components/task/chat/errands components/task/chat/messages/kandev/errand-renderers.test.tsx components/task/mobile/mobile-sessions-section.errands.test.tsx)` | 14 files, 139 tests passed | `frontend-errands.log` |
| `(cd apps/web && pnpm run i18n:check)` | Passed; all six real locales complete and pseudo in sync | `i18n-check.log` |
| `(cd apps/web && pnpm run i18n:ratchet)` | Passed; 220 added and 521 modified files clean; 642 guard entries intact | `i18n-ratchet.log` |
| `git diff --check` | Passed, including final results | `diff-check.log`, `final-diff-check.log` |
| `git diff --cached --check` | Passed | `staged-diff-check.log` |
| `python3 /tmp/kandev-errands-baseline-m5146ci_/catalogs.py` | All eight catalogs passed; 221 unique keys each | `catalog-check.log` |

The three configuration failures are `TestHomeConfigDiscovery`,
`TestHomeConfigDiscoveryOrderUsesOneFirstExistingFile`, and
`TestHomeConfigDefaultDotDirectory` (`source_test.go:149`, `:318`, `:448`).
Each sees inherited port 10101 instead of its YAML fixture port. The tests
clear `KANDEV_SERVER_PORT` but the compatible `KANDEV_BACKEND_PORT=10101`
remains. The identical command reproduced all three failures in a Git
archive of exact upstream `f150e6accd`; the test source is byte-identical.
The merged command then passed with only `KANDEV_SERVER_PORT`,
`KANDEV_BACKEND_PORT`, and `KANDEV_PORT` unset for that subprocess. No source
was changed to waive these failures. See `config-baseline-comparison.json`
and `environment-adjustments.json`.

The initial orchestrator process returned exit 143 without package results
or a runner completion receipt. Its cause is unestablished. The retry uses
the identical test command with `GOGC=50` to reduce compiler memory pressure
and passed both packages. The observed cgroup reported memory-pressure
events but no OOM kill. The recovery command also uses `GOGC=50`.

### Remaining acceptance and handoff at preparation (historical)

- HEAD ancestry is pending; do not mark acceptance 1 or this order done
  until a separately authorized merge commit establishes it.
- Focused MCP, launch/admission, terminal recovery, runtime flags/profiles,
  frontend and i18n checks passed, with the reproduced configuration-shell
  baseline and clean-port rerun described above. No historical macOS waiver
  was used.
- `KANDEV_TEST_POSTGRES_DSN` is unavailable. PostgreSQL and real-provider
  evidence are not claimed; provider/browser/pilot work remains downstream.
- All launched test/install/archive processes have completed. Final
  whitespace/preservation evidence and a stopped-writer handoff release
  source ownership to the primary; no successor is started here. The
  uncommitted merge remains available for primary inspection.
- Specialist task: `c6c7b095-8652-4480-8a06-6f45ccb6a3f3`;
  session: `101d7bb6-ed13-4e5e-ae6e-63c036986516`.
