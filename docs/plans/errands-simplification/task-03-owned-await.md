---
id: 03-owned-await
title: Require one internal await ownership identity
status: done
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

The parent accepted the implementation and later test-fixture correction after independent no-findings review, 83 passing top-level tests, zero-issue scoped lint and verification of all 18 handoff hashes. Task03 is complete; final roadmap browser/performance verification and delivery remain separate. The focused pre-edit race baseline passed 14 Errands tests (17 including subtests) and seven MCP handler tests (15 including subtests), with no failures. Three PostgreSQL variants skipped because no DSN was set at baseline: committed revision receipts, flag-off recovery, and atomic batch claim. This is disclosed baseline evidence only; final service and repository verification enabled the disposable PostgreSQL DSN. Baseline command/output and counts are retained at `/tmp/kandev-task03-baseline-await.log` and `/tmp/kandev-task03-baseline-await.counts.json` (handle 5136, exit 0).

The production change is confined to `internal/errands/await.go` and `internal/mcp/handlers/errands.go`: one value `AwaitRequest.Owner` supplies parent and token; successful registration precedes deferred owned abort; duplicate request fields, fallback UUID generation, ownerless service wrappers and the unused MCP interface method are removed. Public schemas/results, optional private invocation metadata, repository `AbortAwait`, compose/reconcile recovery and flag-off abort/recovery remain unchanged. The pre-claim-error oracle now checks a real persisted owned tombstone and delayed-claim fence on SQLite and PostgreSQL. Existing foreign-owner registration failure still proves that a rejected begin cannot abort the original attempt. The handler forwarding assertion now checks the complete authenticated identity.

Private consumers required migration in `orchestrator/errand_isolation_test.go`, `agent_delivery_recovery_errand_test.go`, `errand_admission_decisions_test.go` and `errand_admission_integration_test.go`; their meaningful assertions are preserved. Final orchestrator selection is exactly `TestDeliveryRecoveryPhaseChangePublishesAttentionAndWakesAwait`, `TestRecoveryPublicationFailureDoesNotNotifyOrWakeErrandAwait`, `TestOrdinarySessionRecoveryDoesNotNotifyOrWakeErrandAwait`, `TestRejectedStaleRecoveryDoesNotNotifyOrWakeErrandAwait`, `TestClearedRecoveryNoticePublishesAttentionAndWakesAwait`, `TestNoOpRecoveryClearDoesNotNotifyOrWakeErrandAwait`, `TestErrandParentSettlementReadFailureNeverMovesOrReviewsTask`, `TestErrandParentSettlementReadableAndDisabledControls`, `TestErrandAdmissionAbortBeforeDelayedAwaitClaim` and `TestErrandAdmissionServiceAbortReleasesRealAwait`. It uses the previously approved orchestrator-only `-gcflags=github.com/kandev/kandev/internal/orchestrator=-l` race accommodation. Full selectors and commands are retained in `/tmp/kandev-task03-selected-tests.json` and adjacent phase `.sh` files.

The three existing `TestReleaseClaimDoesNotReleaseReplacementToken`, `TestReleaseClaimChecksRouteAuthority` and `TestReleaseClaimRechecksExactIdentityAtStorageBoundary` controls retain their exact assertions and synchronization on the real legacy compose/reconcile path. Their existing test helper now calls the existing private `releaseClaim` directly; retargeting those tests to `AbortOwnedAwait` would change their no-wake-on-no-change contract. The parent accepted this distinction. They and `TestClaimSnapshotRetryIsBounded` are added to the final service/handler Await selector. No replacement production compatibility wrapper was introduced.

The frozen 17 Go paths are listed in `/tmp/kandev-task03-changed-go-paths.txt`; their hashes are `/tmp/kandev-task03-source.sha256` (manifest SHA256 `ce198ab56ed9943e71b77a8509f00535dc013087da9be04cea8f0924c06d5d57`). No persistence query/schema change or provider operation is part of this phase. Verification claims only the selected service/handler and SQLite/PostgreSQL behaviors; it does not claim a fresh full-store conformance or live-provider guarantee. Final source, lint and release receipts are recorded below.

Independent review accepted the initial source and then the sole later fixture correction in `session_state_test.go`: two stale token references now read `req.Owner.Token`, and the fixture registers the owned attempt before entering `awaitLoop` directly. No assertion or production change followed the original freeze. The initial final command exposed those two compile errors; its unchanged handler package still passed all seven tests (15 including subtests), with no skips or race warnings (`/tmp/kandev-task03-final-await-build-error.log`, overall exit 1). After the fixture correction, the service-only rerun passed 22 tests (36 including subtests), including all four PostgreSQL variants and the three legacy release controls, with no skips or race warnings (`/tmp/kandev-task03-final-service-await.log`, exit 0). The unchanged handler suite was not repeated. Both review manifests and the failed compilation receipt remain retained.

The real repository Await race selector passed 44 tests: 23 SQLite and 21 PostgreSQL top-level cases, 158 entries including subtests, with zero skips or race warnings (`/tmp/kandev-task03-final-repository-await.log`, exit 0). The explicit disposable PostgreSQL 16 DSN used only loopback port 55488 and isolated test schemas. Initial fixture startup inherited the build-lock descriptor, so the waiting test command was terminated before any Go process/test started and PostgreSQL was restarted with `flock --close`; the waiting receipt remains separate. After the both-engine checks, graceful shutdown completed its checkpoint in 53.613 seconds, beyond the first 30-second `pg_ctl` wait. The retained timeout is not claimed as successful shutdown: server-log completion, absent PID/postmaster.pid and absent port-55488 listener establish the final stop (`/tmp/kandev-task03-postgres-stop-confirmed.json`). No live database or provider operation occurred.

The exact ten affected orchestrator cases passed under the race detector, 27 entries including subtests, with zero skips or race warnings (`/tmp/kandev-task03-final-orchestrator-consumers.log`, exit 0). This compiled the affected orchestrator package and all of its test consumers; only its race compilation used the recorded scoped inlining accommodation. Combined scoped verification is 83 top-level tests and 236 entries including subtests, all passing with no final skips or race warnings. The production delta is two files, 28 added and 78 removed lines (net minus 50); the other 381 scoped production hashes match the checkpoint. `/tmp/kandev-task03-production-delta.json` records those paths and counts.

The required exact-17-file `scripts/lint-go-changed` run reached its five-minute context-loading timeout (handle 43626, exit 4), retained at `/tmp/kandev-task03-changed-go-lint.log`. After its processes exited, the identical three packages, comparison base `585824a5d066d22785814c0394b88dd57f6c5ded`, configuration and concurrency passed with only the timeout extended to 15 minutes (handle 92358, exit 0, `0 issues.` in `/tmp/kandev-task03-changed-go-lint-extended.log`). No lint gate or optimization was disabled; `/tmp/kandev-task03-lint-scope.json` and the adjacent command scripts retain the exact scope. All handles are drained in `/tmp/kandev-task03-processes.json`.

Cleanup removed only two superseded Task02-owned orchestrator cache archives and the failed Task03 lint scratch directory: 2,805,653,340 file bytes, 2,805,780,480 allocated bytes, with 11,794,268,160 bytes free immediately afterward. Exact content hashes, source-era symbols, prior ownership receipts and captured compiler output paths establish provenance. There were no active build owners or visible open/mapped references; three inaccessible, preexisting non-build host services are explicitly recorded as an inspection limit. The initial conservative cleanup guard stopped without mutation before those services were identified. Current Task03 caches, module downloads and all logs/manifests/test evidence remain preserved in `/tmp/kandev-task03-cache-cleanup.json` and its linked proof receipts.

Final `gofmt`, work-order Prettier and `git diff --check` passed. The 17 reviewed source hashes and accepted Task01/Task02 source manifests remain unchanged. The complete 18-path manifest, including this Results update, is `/tmp/kandev-task03-final-source.sha256`; final disk/process/lock/PostgreSQL checks are `/tmp/kandev-task03-final-hygiene.json`. The local branch and checkpoint remain `feature/correct-errand-statu-9e4` at `80faa920c31121b1afe84fef14c74f92000827ab`. Shared-checkout writer ownership is released to the parent with no owned process, PostgreSQL instance or build-lock ownership remaining. Parent acceptance and the final roadmap browser/performance work remain outside this handoff.
