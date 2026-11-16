---
id: "09-teardown-intent-storage"
title: "Persist exact teardown obligations"
status: done
wave: 9
depends_on:
  - "04-await-outcome-recovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.2
  - AC-TASKS-ERRANDS-007.4
  - AC-TASKS-ERRANDS-007.6
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 09: Persist exact teardown obligations

## Scope and authority

Repairs review finding(s) 6 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals.

## Low-level implementation and regression plan

**Owned:** `internal/task/models/errand.go`, new `internal/task/models/errand_teardown.go` and test; `internal/task/repository/sqlite/{errands_schema.go,errands.go}` plus existing schema/roundtrip/concurrency tests, `errands_postgres_test.go`, and relevant task store conformance/upgrade fixture. The integrating session must assign one writer for the combined migration after Tasks 03 and 04 release schema ownership.

**Private schema:** Add **one** `teardown_intent TEXT NOT NULL DEFAULT ''` column, encoded JSON, to `task_session_errands`, idempotently through the existing schema/migrator. Empty is no recorded intent, not proof of physical teardown. Add typed `Errand.TeardownIntent *ErrandTeardownIntent` omitted from public DTOs. Closed intent states: `pending`, `completed`, `superseded`. Fields: version=1, server-generated intent ID, state, request time, target task/session/queue-incarnation, execution ID when known, target active-or-latest turn ID (plus explicit no-turn witness), submission ID when known, and optional reserved `(first_turn_id,submission_id)` launch tuple for pre-session cancellation. Store those identity fields as the immutable `Target` value specified in Task 10. Include captured prompt generation where supported for in-process revalidation; do not treat process-local generation as restart authority. Result state and cancellation source retain existing fields.

Why a separate intent: `lifecycle=cancelled` must remain the public idempotent status and cannot be reused as an unfinished-work queue. Keeping the immutable target also prevents a stop retry from resolving a newly active execution and stopping it by mistake. Do not add a new public errand status or a retrying/busy session state.

**Storage rules:** `cancelled + intent(pending) + cancellation notice/discard decision` commit in one version-guarded row update. Intent transitions and completion use the same Version CAS and exact intent ID. A conflict reloads/re-evaluates, never overwrites a new intent/result revision. Validate all identity combinations before writing; a reserved-only target must include both first turn and submission, not a bare ID. No external runtime I/O while a SQL transaction is held. SQLite uses existing immediate writer admission; PostgreSQL uses the existing per-task advisory-lock discipline. Update every insert/select/scan/update, fresh schema, replay migration, roundtrip, required-table/conformance/explicit-upgrade evidence touched by the added column. This is compatible expansion; no blanket backfill of captured execution identity.

**Legacy:** Existing cancelled rows with empty intent are candidates for conservative recovery in Task 10. Do not mass-assign today's current turn as the historical target; require evidence predating cancellation (old session cancellation timestamp/turn/executor binding or exact initial launch authority). If a later user turn exists, preserve it. Unknown evidence remains visible/retryable in logs/recovery block, not successful teardown. Root-cause repairs must not fabricate provenance.

**Regression:** `TestErrandTeardownIntentRoundTripsAndSurvivesReopen`, `TestErrandCancellationAndStopIntentCommitTogether`, `TestErrandTeardownIntentCASDoesNotOverwriteSuccessor`, `TestErrandTeardownIntentMigrationReplays`, and matching `TestPostgresErrandTeardownIntent...` wrappers. Reopen the real file DB after commit to prove durable state, not just an in-memory struct. Malformed JSON, partial identity, explicit empty legacy value and wrong intent-ID CAS need coverage.

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/models -run 'Errand' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite -run 'Test(Errand.*(Teardown|Schema|RoundTrip|Concurrent)|PostgresErrand.*(Teardown|Schema|RoundTrip|Concurrent))' -count=1)
(cd apps/backend && go test -trimpath ./internal/persistence/storeconformance -run 'Errand|Task' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/models/errand.go apps/backend/internal/task/models/errand_teardown.go apps/backend/internal/task/repository/sqlite/errands.go apps/backend/internal/task/repository/sqlite/errands_schema.go
git diff --check
```

**ACs:** `007.1`, `007.2`, `007.4`, `007.6`, `012.3`. PostgreSQL tests skipped without `KANDEV_TEST_POSTGRES_DSN` are unproved, never a pass. No need to add a new RequiredTables entry for a column on the existing table; extend the owning conformance scenario/upgrade check.

## Dependencies and ownership

[Task 04](task-04-await-outcome-recovery.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

**Done.** Added private, versioned teardown-intent data to `Errand` and one idempotent `teardown_intent TEXT NOT NULL DEFAULT ''` column. Empty remains no evidence and legacy rows are not backfilled. Insert/select/scan/update paths validate and round-trip the JSON; the field is omitted from JSON claim snapshots and public DTOs. Cancellation, pending intent, and result decision share the existing task-locked, version-CAS row write. `TransitionErrandTeardownIntent` fences pending-to-terminal transitions by exact intent ID and row version and changes no successor result fields.

**Task 10 storage handoff.** Attach an intent by loading the errand, setting `Lifecycle=cancelled`, its existing `CancelSource`, and a validated pending `TeardownIntent`, then calling `UpdateErrand`; it must include a server-generated ID and request time. Attachment is allowed only when no intent exists. Generic updates cannot change a recorded intent's ID, version, request time, target, or state. Complete or supersede with `TransitionErrandTeardownIntent(ctx, errandID, expectedIntentID, expectedVersion, state)`. The transition returns the updated row; stale versions, wrong IDs, and non-pending intents return `ErrErrandVersionConflict`, so callers must re-read and re-evaluate. Session targets require matching task/session IDs, nonblank session incarnation, and exactly a turn ID or explicit no-turn witness; reserved-launch targets require the exact immutable first-turn/submission pair and no session-only fields. Prompt generation requires a known execution and remains process-local evidence.

**TDD evidence.** Model validation REDs demonstrated acceptance of a partial session target and whitespace-only turn, execution, submission, and reserved tuple IDs (`/tmp/errands-task09-model-red.log`, `/tmp/errands-task09-blank-id-red.log`). File close/reopen RED lost the intent, and a mutated exact-transition RED rejected a valid completion (`/tmp/errands-task09-roundtrip-red.log`, `/tmp/errands-task09-cas-red.log`).

**Final test, sqlguard, and lint commands all used `/tmp/kandev-errands-repair-build.lock`.** `cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/task/models -run '^TestErrand' -count=1 -v` passed 6 top-level tests (`/tmp/errands-task09-model-final.log`). With `KANDEV_TEST_POSTGRES_DSN=postgres://postgres@127.0.0.1:55487/postgres?sslmode=disable`, the focused SQLite repository selector `go test -race -trimpath -p 1 -parallel 1 ./internal/task/repository/sqlite -run 'Test(ErrandTeardown|ErrandCancellationAndStopIntent|ErrandSchemaReplays|ErrandRowRoundTrips|ErrandConcurrentUpdate|PostgresErrandTeardown|PostgresErrandCancellationAndStopIntent|PostgresErrandSchemaReplays|PostgresErrandRowRoundTrips|PostgresErrandConcurrentUpdate)' -count=1 -v` passed 8 SQLite and 8 PostgreSQL top-level tests with zero skips (`/tmp/errands-task09-postgres-repository.log`). The corrected `go test -race -trimpath -p 1 -parallel 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3|pgx)/^task$' -count=1 -v` selector passed 14 leaf scenarios across `sqlite3/task` and `pgx/task` (`/tmp/errands-task09-postgres-conformance.log`).

`go test -race -trimpath -p 1 -parallel 1 ./internal/persistence/storeconformance -run '^TestPreviousStableUpgrade$/(sqlite3|pgx)/v0.93.0$' -count=1 -v` passed the v0.93.0 upgrade subtest on both dialects and verified the expanded column (`/tmp/errands-task09-postgres-upgrade.log`). `go run -trimpath -p 1 ./cmd/sqlguard ./internal` passed (`/tmp/errands-task09-sqlguard.log`); scoped `bash scripts/lint-go-changed` over the eleven owned Go files passed with zero findings (`/tmp/errands-task09-lint.log`). Owned-file `gofmt` and `git diff --check` passed. The isolated PostgreSQL schema inventory matched before and after; the server started for this run was stopped. Baseline hashes show zero changes outside Task09-owned files. An earlier no-DSN probe skipped 8 PostgreSQL wrappers and an initial repository regex matched zero tests; neither is counted as acceptance evidence. The final corrected run above exercised all PostgreSQL wrappers without skips.
