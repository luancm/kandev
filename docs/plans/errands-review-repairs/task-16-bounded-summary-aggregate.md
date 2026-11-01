---
id: "16-bounded-summary-aggregate"
title: "Batch bounded errand summary observations"
status: done
wave: 16
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-010.5
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 16: Batch bounded errand summary observations

## Scope and authority

Repairs review finding(s) 12 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. The repository aggregate is implemented, and the final PostgreSQL acceptance below verifies the shared scenario and 500-ID boundary on PostgreSQL 16.

## Low-level implementation and regression plan

### Exact files/symbols

- New apps/backend/internal/task/repository/sqlite/errands_summary_batch.go: Repository.BatchErrandsSummariesByTaskIDs(ctx, taskIDs []string) (map[string]statussummary.ErrandsSummary, error).
- Existing apps/backend/internal/task/repository/sqlite/errands.go: reuse liveErrandPredicate and outstandingErrandPredicate without changing them; ErrandsSummaryByTask remains compatible.
- New apps/backend/internal/task/repository/sqlite/errands_summary_batch_scenarios_test.go: exerciseBatchErrandsSummaries helper shared by dialect tests.
- New apps/backend/internal/task/repository/sqlite/errands_summary_batch_test.go: SQLite test and env-gated PostgreSQL wrapper, using the repository's existing Postgres fixture setup from errands_postgres_test.go. Use newPostgresErrandRepo(t) for the PostgreSQL fixture. Keep tests below backend size limits.

Do not widen repository.TaskRepository with this optional capability; declare the narrow structural batch-reader interface at the consuming backendapp boundary in Task 17. No new required fake methods.

### Algorithm and state/failure rules

1. Deduplicate nonempty requested IDs; initialize an explicit zero aggregate for EVERY requested ID. Empty input returns an empty map and performs no SQL.
2. Chunk IDs with the existing chunkIDs/buildInPlaceholders helpers. Reserve the parent-state bind parameter: chunk size sqliteMaxHostParams-1, not sqliteMaxHostParams. Rebind through r.ro.Rebind for both SQLite/PostgreSQL.
3. For each chunk run the current aggregate once, grouped by e.task_id:
   SELECT e.task_id,
     COALESCE(SUM(CASE WHEN liveErrandPredicate THEN 1 ELSE 0 END),0),
     COALESCE(MAX(CASE WHEN ps.state = ? AND outstandingErrandPredicate THEN 1 ELSE 0 END),0)
   FROM task_session_errands e
   LEFT JOIN task_sessions s ON s.id=e.id
   LEFT JOIN task_sessions ps ON ps.id=e.parent_session_id
   WHERE e.task_id IN (...)
   GROUP BY e.task_id
   Parent state parameter is WAITING_FOR_INPUT. Waiting means EXISTS an idle parent with an outstanding errand; do not require all children or all parents to wait. Live count includes queued children lacking session rows. A failed/deleted child does not erase a different live child's contribution.
4. Scan only task ID, count and waiting boolean. Return no errand rows, objectives, results, timestamps or transcripts. Unrequested rows cannot enter the result.
5. On query/scan/iteration/close failure, close rows and return nil/error, never a partial success map interpreted as zero. Honor ctx cancellation without sleeps, retries or new goroutines.

Private Go API addition only. No database/table/index/migration, ErrandDTO, HTTP or WS schema change. The single-task projector loader continues using its existing aggregate and semantics.

### Regression names/shapes

- TestBatchErrandsSummariesByTaskIDsSQLite: two tasks, multiple parents, one queued child without session, one running, one failed/missing child, one settled pending result under an idle parent, zero-row task, duplicate/empty IDs and unrelated-task data. Assert exact map equals each existing single-task aggregate and literal expected live/waiting values; release the pending result and assert waiting clears independently of live count.
- TestPostgresBatchErrandsSummariesByTaskIDs: same scenario, real env-gated PostgreSQL, including chunk boundary > sqliteMaxHostParams-1. No fake SQL implementation or dialect-only schema proof.
- TestBatchErrandsSummariesByTaskIDsFailureDoesNotReturnPartialZero: canceled ctx and a closed/failing DB boundary; callers receive an error, not a map usable as a successful empty observation.

This is additive infrastructure. The reviewed-defect behavioral RED is TestTaskStatusSummaryErrandsReadRepair in Task 17; record it before Task 16/Task 17 production work. For the new batch method, a minimal compile scaffold returning no rows lets the permanent aggregate test fail on its expected nonzero map before implementing SQL. Do not claim an undefined-method compile error as behavioral RED.

### Runnable commands (repo root; future implementation)

~~~
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'Test(BatchErrandsSummariesByTaskIDs|PostgresBatchErrandsSummariesByTaskIDs|ErrandsSummaryByTask|PostgresErrandsSummaryByTask)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/errands_summary_batch.go apps/backend/internal/task/repository/sqlite/errands_summary_batch_scenarios_test.go apps/backend/internal/task/repository/sqlite/errands_summary_batch_test.go
(cd apps/backend && go run ./cmd/sqlguard ./internal)
~~~

Postgres test skips without KANDEV_TEST_POSTGRES_DSN are not passes. Use a parent-authorized disposable PostgreSQL instance if available; this design errand must not start one. These queries are read-only, so no new transaction advisory lock is needed.

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

Parallel-safe with Task04C frontend/localization. Task16 owns only the aggregate repository files and tests listed above; frontend owns web files. All heavy validation must acquire `/tmp/kandev-errands-repair-build.lock` and follow `/tmp/errands-verification-resource-policy.md`. The integrating session authorizes the already established disposable PostgreSQL fixture for real dialect validation; no live database is used.

## Results

The behavioral RED is recorded at `/tmp/errands-task16-red.log`. `GOMAXPROCS=2 go test -trimpath -p 1 ./internal/task/repository/sqlite -run '^TestBatchErrandsSummariesByTaskIDsSQLite$' -count=1` failed on the expected aggregate map: the zero-only scaffold returned `{Live:0, Waiting:false}` where the seeded active task expected `{Live:2, Waiting:true}`. Earlier fixture-only failures were corrected before this RED and are not acceptance evidence.

The primary Task 17 consumer RED is also recorded with a temporary Go overlay, without editing its unowned service/test files: `GOMAXPROCS=2 go test -trimpath -p 1 -overlay=/tmp/errands-task16-primary-red/overlay.json ./internal/task/service -run '^TestTaskStatusSummaryErrandsReadRepair$' -count=1` failed both real-SQLite cases. `missing_summary` returned `Errands:nil` instead of `{Live:1, Waiting:true}`; `stale_summary` retained `{Live:7, Waiting:false}` instead of `{Live:1, Waiting:true}`. The failure is at the returned-summary assertion; the persisted-summary assertion follows it. Log: `/tmp/errands-task16-primary-red.log`. This consumer RED was captured after Task 16's repository implementation but before Task 17 work, since its reader setter/test seam is outside this packet.

`GOMAXPROCS=2 go test -trimpath -p 1 ./internal/task/repository/sqlite -run '^TestBatchErrandsSummariesByTaskIDs' -count=1` passed the three focused SQLite tests for the mixed-parent aggregate, nil/error on cancellation/closed database and later-chunk failure, and exact query counts/deduplication. Log: `/tmp/errands-task16-sqlite-focused.log`.

Historical SQLite-only run, superseded by the final PostgreSQL acceptance below: `GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/task/repository/sqlite -run 'Test(BatchErrandsSummariesByTaskIDs|PostgresBatchErrandsSummariesByTaskIDs|ErrandsSummaryByTask|PostgresErrandsSummaryByTask)' -v -count=1` passed 4 SQLite tests, including the existing single-task summary regression; both PostgreSQL tests skipped because `KANDEV_TEST_POSTGRES_DSN` was unset. Log: `/tmp/errands-task16-focused-race.log`. The skip was not a PostgreSQL pass.

`go run ./cmd/sqlguard ./internal` from `apps/backend` exited 0; log `/tmp/errands-task16-sqlguard.log` is empty. `gofmt -l` reported no files.

`bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/errands_summary_batch.go apps/backend/internal/task/repository/sqlite/errands_summary_batch_scenarios_test.go apps/backend/internal/task/repository/sqlite/errands_summary_batch_test.go` passed with `0 issues.` Log: `/tmp/errands-task16-lint.log`. Its first attempt exited 3 while another worker held the golangci-lint lock; this successful retry ran after that process ended.

Historical checkpoint: at the time of the earlier SQLite implementation pass, no PostgreSQL instance was available and actual execution remained pending. The acceptance below resolves that gate.

The repository method is implemented and ready for Task 17 to consume through its planned narrow structural reader interface. No shared interface, schema, service, or consumer file was changed.

Final PostgreSQL acceptance: started the authorized disposable PostgreSQL 16.15 fixture at `127.0.0.1:55487` from `/tmp/errands-pg-check`; `KANDEV_TEST_POSTGRES_DSN` was explicitly set to `postgres://postgres@127.0.0.1:55487/postgres?sslmode=disable`, so neither PostgreSQL test skipped. Ran `GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=8GiB GOCACHE=/home/luancm/.cache/go-build GOMODCACHE=/home/luancm/go/pkg/mod go test -trimpath -race -p 1 -parallel 1 ./internal/task/repository/sqlite -run 'Test(BatchErrandsSummariesByTaskIDs|PostgresBatchErrandsSummariesByTaskIDs|ErrandsSummaryByTask|PostgresErrandsSummaryByTask)' -count=1 -v -timeout=300s` from `apps/backend`; all 6 selected tests passed, including both real PostgreSQL summary tests, the shared mixed-parent/queued-child/pending-result/duplicate/empty/unrequested scenario, and 500 unique IDs across the 499-ID chunk size. The SQLite grouped-query-count test observed 2 statements for 500 IDs and 1 for a deduplicated single chunk; cancellation, closed-reader, and later-chunk failure cases returned errors without partial maps. Receipt: `/tmp/errands-task16-completion-pg-race.log`.

The existing Task16 SQLguard and changed-file lint receipts remain available (`/tmp/errands-task16-sqlguard.log`, `/tmp/errands-task16-lint.log`; lint reported `0 issues`); no Go code file was edited during this acceptance-only pass. PostgreSQL fixture cleanup is registered by `OpenIsolatedPostgres`; the test run produced no PostgreSQL test failures or skips. PostgreSQL stop succeeded with `/tmp/errands-task16-completion-pg-stop.log`, and `/tmp/errands-pg-check/data/postmaster.pid` is absent. A post-run read-only check found one `kandev_test_*` schema in the persistent disposable database; its run could not be attributed from available pre-run evidence, so it was left untouched. No source, service, live-instance, shared manifest, commit, or push changes were made.
