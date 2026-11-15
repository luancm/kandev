---
id: 03-owned-await
title: Require one internal await ownership identity
status: pending
wave: 3
depends_on:
  - 02-lifecycle-controller
plan: plan.md
requirements:
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-004.5
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.13
system_design:
  - ../../specs/tasks/system-design/errand-await-receipts.md
---

# Task03: require one owned await identity

The sole production constructor of `AwaitRequest`, `handleAwaitErrands`, supplies an authenticated `ErrandAwaitIdentity`. Remove the internal ownerless compatibility path used only by fixtures: require a value owner, derive parent/token from that identity, remove duplicate fields and fallback token generation, and remove service-level `ReleaseAwait`/`AbortAwait` wrappers and the unused `ErrandMCPService.AbortAwait` method after migrating their tests. This makes the existing production authority explicit; it adds no new receipt policy.

Own `apps/backend/internal/errands/await.go`, its request consumers/helpers and direct tests, the request construction/interface in `internal/mcp/handlers/errands.go`, affected handler test doubles, and this Results section. Preserve repository `AbortAwait` used by `errands/compose.go` and persisted legacy recovery. Keep optional private invocation metadata handling in `errand_await_outcomes.go`, the distinction between failed registration and a successfully bound attempt, feature-disabled abort/recovery, public tool schemas and returned result bodies. No schema, store-query, provider parsing or transport-protocol change is planned.

Correct `awaitErrandView`'s nearby comment to describe results claimed by this await versus `DeliveredVia` for another claim; a token-matching result is already claimed when rendered. This comment correction does not change delivery semantics.

## Acceptance

1. All production await calls have one validated ownership value; no ownerless service branch or substitute compatibility wrapper remains. Duplicate parent/token values cannot diverge internally.
2. Existing meaningful fixture assertions survive migration: pre-claim errors commit an owned abort only after successful binding, one token carries only its committed revisions, and invalid/late ownership never releases received or uncertain results. Retain legacy recovery and omitted private metadata compatibility.
3. Focused service/handler tests and both-engine receipt/abort tests pass, with actual matched counts/skips recorded. No claimed success from a gated suite that did not run.

## Verification

Capture the focused baseline before editing; rerun after the final change. From `apps/backend`, sequentially:

```bash
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/errands ./internal/mcp/handlers -run 'Test.*Await' -count=1 -v
flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/task/repository/sqlite -run 'Test.*Await' -count=1 -v
```

Use the existing disposable PostgreSQL fixture with an explicit test DSN for the latter suite, never live data. Retain `TestAwaitErrorBeforeClaimCommitsAbortTombstone`, `TestAwaitReturnsOnlyResultsClaimedByItsToken`, `AwaitResponseCarriesOnlyCommittedRevisionReceipts`, `TestAwaitRequiresAuthenticatedLaunchedParentOwner` and `TestAwaitAbortRetainedOwnerSurvivesDisabledAndNewGeneration`. Run `gofmt`, `git diff --check`, and exact changed-Go lint under the shared lock. Preserve the request/response fixture contract; do not infer that a removed private service method permits deleting SQL recovery tests.

## Results

Pending. Return changed responsibilities/paths, baseline and final receipts, retained-oracle evidence, source hashes, limitations and explicit writer/process release. No commit, push or live operation belongs to this worker.
