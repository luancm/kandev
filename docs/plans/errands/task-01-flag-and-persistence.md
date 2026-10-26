---
id: "01-flag-and-persistence"
title: "Add the errands flag, session role, and errand persistence"
status: pending
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

Pending.
