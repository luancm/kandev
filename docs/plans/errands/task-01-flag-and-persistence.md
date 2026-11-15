---
id: "01-flag-and-persistence"
title: "Add the errands flag, session role, and errand persistence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.1
  - AC-TASKS-ERRANDS-012.1
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 01: Add the errands flag, session role, and errand persistence

## Summary

Add the `features.errands` release toggle, off in every shipped profile, and
the durable data the rest of the feature builds on: the immutable session
role and parent columns and the `task_session_errands` table. Project the new
fields through the session DTOs. Nothing reads the data yet, so behavior is
unchanged.

## In scope

- `KANDEV_FEATURES_ERRANDS` in root `profiles.yaml` (`prod`, `dev`, `e2e` all
  `"false"`), `FeaturesConfig.Errands` (`mapstructure:"errands"`,
  `json:"errands"`), one registration in `runtimeflags/registry.go` with
  `RestartRequired: true`, and `errands: false` in
  `apps/web/lib/state/slices/features/types.ts`. Follow `/runtime-feature-flags`.
- Idempotent `ADD COLUMN` migrations for `task_sessions.role` and
  `task_sessions.parent_session_id`, then `idx_task_sessions_parent_session_id`,
  in `runMigrations()`; optional inline columns in the `CREATE TABLE`.
- `initErrandsSchema` for `task_session_errands` with the columns and indexes
  in the [design persistence section](../../specs/tasks/system-design/errands.md#persistence),
  including the `mcp_server_ids` snapshot and the `turn_context` and
  `notification` delivery routes.
- `models.TaskSession.Role`, `ParentSessionID`, `SessionRoleErrand`,
  `models.Errand`; every session insert path writes the columns; no update
  path changes them.
- Repository methods for errand rows: insert, get by ID, list by task, list
  by parent, list queued, count live by task, version-guarded update, delete
  by session ID inside the session delete transaction. SQLite and PostgreSQL.
- `TaskSession.ToAPI`, `dto.TaskSessionDTO`, and `dto.TaskSessionSummaryDTO`
  project `role` and `parent_session_id`; `dto.ErrandDTO` exists.
- `task_session_errands` in the `task` descriptor `RequiredTables`, a
  `storeconformance` scenario, and the explicit-tag upgrade fixture.

## Out of scope

- Any reader of the role (task 02), any errand creation (task 04), and
  frontend consumption beyond the flag key (task 03).

## Acceptance

1. A fresh database and a same-database replay both produce the columns,
   index, and table on SQLite, and on PostgreSQL when
   `KANDEV_TEST_POSTGRES_DSN` is set; an errand row round-trips with all
   fields and a version-guarded update rejects a stale version.
2. A session inserted with role `errand` and a parent keeps both values
   through every read and full-row write, and session DTOs project them.
3. The flag reads `false` from every shipped profile, the registry, config,
   profile, and frontend contract tests pass, and toggling requires restart.

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`. Set `KANDEV_TEST_POSTGRES_DSN`
to include PostgreSQL coverage in the repository and conformance commands.

```bash
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test -trimpath ./internal/task/models ./internal/task/dto -run 'Errand|SessionRole' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'Errand|SessionRole' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/persistence/requiredstores ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `profiles.yaml`, `apps/backend/internal/common/config/config.go`,
  `apps/backend/internal/runtimeflags/registry.go` and its tests.
- `apps/backend/internal/task/repository/sqlite/base_schema.go`,
  `base_migrations.go`, `session.go`, a new `errands.go` and tests.
- `apps/backend/internal/task/models/models.go`, a new `models/errand.go`.
- `apps/backend/internal/task/dto/dto.go`.
- `apps/backend/internal/persistence/requiredstores/catalog.go`,
  `apps/backend/internal/persistence/storeconformance/` scenarios and fixture.
- `apps/web/lib/state/slices/features/types.ts`.

## Dependencies

None.

## Risks

- Table-rebuild migrations elsewhere in `base_migrations.go` must mirror the
  new `task_sessions` columns in their replacement `CREATE TABLE` and copy
  lists, or a later rebuild drops them.
- Every session insert and bulk-write path must set the columns; a missed
  path silently creates ordinary sessions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/errands.md) and
  [system design](../../specs/tasks/system-design/errands.md#persistence).
- `apps/backend/AGENTS.md` schema, PostgreSQL parity, and persistence rules.
- `/runtime-feature-flags` and `/tdd`.

## Results

Completed on 2026-10-26 in the primary session, test-first. No delegation.

- Flag: `features.errands` / `KANDEV_FEATURES_ERRANDS`, `FeaturesConfig.Errands`,
  `"false"` in `prod`, `dev`, and `e2e`, `RestartRequired: true`, experimental,
  medium risk, and `errands: false` in the frontend defaults.
- Sessions: `task_sessions.role` and `parent_session_id` (`TEXT NOT NULL
  DEFAULT ''`) and `idx_task_sessions_parent_session_id`, added by
  `migrateTaskSessionErrandColumns` right after the `task_sessions.name`
  migration, so the two gated table rebuilds always run first. No inline
  `CREATE TABLE` columns. `models.TaskSession.Role` (`models.SessionRole`) and
  `ParentSessionID` are written by `createTaskSession`, read by every session
  scanner and the summary observation, and never written by an update.
- Errands: `initErrandsSchema` creates `task_session_errands` with the design
  columns, `idx_task_session_errands_task_lifecycle`, and
  `idx_task_session_errands_parent`. `models.Errand`, its closed state types,
  `Validate`, `ErrErrandNotFound`, `ErrErrandVersionConflict`, and
  `ErrInvalidErrand` are in `models/errand.go`.
- Repository: `CreateErrand`, `GetErrand`, `ListErrandsByTask`,
  `ListErrandsByParent`, `ListQueuedErrands`, `CountLiveErrandsByTask`,
  `UpdateErrand` (compare-and-set on `Version`; immutable start fields are
  never written), and `DeleteErrand` for rows without a session. The session
  delete transaction removes the errand row; the task foreign key removes the
  rest. Every method is a single statement, so no advisory lock is needed yet.
- Projection: `TaskSession.ToAPI`, `TaskSessionDTO`, and
  `TaskSessionSummaryDTO` project `role` and `parent_session_id` with
  `omitempty`. `dto.ErrandDTO` and `dto.FromErrand(errand, status)` cut the
  preview to 280 characters.
- Stores: `task_session_errands` is in the `task` descriptor, `errandAction`
  covers CRUD, boolean, timestamp, and duplicate and stale-version conflicts,
  and the v0.93.0 manifest lists the table as known missing.

Verification passed:

- `go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1`
- `go test -trimpath ./internal/task/models ./internal/task/dto -run 'Errand|SessionRole' -count=1`
- `go test -trimpath ./internal/task/repository/sqlite -run 'Errand|SessionRole' -count=1`,
  also under `-race`; the PostgreSQL variants skipped because
  `KANDEV_TEST_POSTGRES_DSN` was not available.
- `go test -race -trimpath ./internal/persistence/requiredstores ./internal/persistence/storeconformance -count=1`
  (SQLite only for the same reason). A mutation that dropped the version
  guard failed the conformance conflict scenario.
- `go run ./cmd/sqlguard ./internal`
- `pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts` (9 tests)
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues with
  `GOTOOLCHAIN=go1.26.3`. The local golangci-lint cannot read Go 1.27 export
  data; CI uses Go 1.26.3.
- `gofmt -l` on changed Go files, `pnpm run typecheck` (8 GiB heap), and
  `git diff --check`.

Broader runs of `./internal/task/...`, `./internal/office/service`, and
`./internal/orchestrator/...` show only failures that reproduce identically on
the base commit on this macOS host (workspace path and terminal tests).
