---
id: "01-browser-reads"
title: "Add bounded browser read contracts"
status: done
wave: 2
depends_on:
  - "00-baseline"
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.2
  - AC-TASKS-ERRAND-HISTORY-001.3
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.6
  - AC-TASKS-ERRAND-HISTORY-001.8
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Add bounded browser read contracts

## Scope, implementation acceptance and exclusions

Own additive query methods in `apps/backend/internal/task/repository/interface.go`, `repository/sqlite/errands*.go`, browser read service methods alongside `internal/errands/list.go`, handlers alongside `internal/task/handlers/errand_ws_handlers.go`, DTO/action declarations, `apps/web/lib/types/errands.ts` and typed `lib/api/domains/errand-api.ts` methods/tests. Preserve complete internal/legacy/MCP lists and mutations. Implement the exact scope/limit/keyset/count/auth/flag-off contracts in the design; SQL LIMIT precedes scanning/enrichment, compact columns exclude result bodies. Query-plan/index and differential lifecycle/recovery/unread classification tests cover both supported database implementations; do not change provider abstractions.

Acceptance: (1) bounded 25+1 SQL read and authoritative counts match complete-state oracle across pending/recovery/continuation states, (2) scope/auth/cursor/unsupported-action/feature-off tests pass, (3) complete lists and task-wide cancellation still include unloaded children. Exclude ordinary boot wiring and UI switching; no simultaneous shared-contract writer.

## Verification

Baseline review disposition: browser-store counts do not establish database materialization or enrichment work. Add focused repository/service evidence that LIMIT/lookahead applies before materialization/enrichment, compact projections exclude bodies, and listing finished history performs no per-finished-child detail calls. Indexed aggregate scans may scale with history; do not claim constant rows scanned or substitute returned-row counts for this evidence.

Run from repository root after implementation authorization; commands are future checks, not design evidence. Heavy checks require the shared lock, GOMAXPROCS=2 and no overlapping browser runs. Task00 must record the exact chosen isolated seeding/measurement invocation before its gate can pass. New task04 E2E files are proposed, not present yet. Also run changed-Go lint with explicit changed Go paths and exact-file web lint for files edited by this work order, following scoped AGENTS.md.

```bash
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 go test -race -p 1 ./internal/task/repository/sqlite ./internal/errands ./internal/task/handlers -run 'Errand' -count=1)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm exec vitest run lib/api/domains/errand-api.test.ts lib/types/errands.test.ts)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm run typecheck)
```

## Results

Parent accepted Task01 on 2026-11-13. Era Sol reviews accepted the snapshot/unread/production-plan/upgrade corrections and offset normalization; parent closed the final single-digit-hour finding after inspecting the validated padding, shared-key use, v3 predecessor replacement and parser-differential cases. Source hash `e8b3a593d49eeea2e2ca5b008c9941324193e5737fd0678be148187efe935b66` and final SQLite/both-engine GREEN log hashes match the handoff. Earlier 234-test race and 11-test conformance evidence remains prior evidence; final focused timestamp, plan and upgrade checks passed without PostgreSQL skips. The handoff records exact revisions and limits. This accepts additive browser contracts only; ordinary loading isolation, history UI and candidate performance remain later work. Historical in-progress statements below describe earlier checkpoints.

Implementation completed on 2026-11-13 for the additive browser-read contracts. `errand.browser_summary`, `errand.history_page`, and `errand.inspect` are available through the WebSocket handlers, DTOs, Errands service, and optional repository interface. Summary totals and rows come from scoped SQL; history counts and `LIMIT 26` compact rows share a read transaction; direct inspection reads one child and minimal parent identity. Existing complete list, MCP, cancellation, and mutation paths remain intact. No ordinary boot/list wiring or UI consumers changed.

The TDD red phase first exposed the absent browser API method (1 failing and 8 passing API tests) and absent repository browser-read methods. The green API and type suites pass 54 tests across three files. The focused SQLite/service/handler race run passes after the final query-helper refactor; changed-Go lint reports zero issues. Exact commands, test evidence, residual limits, and SHA-256 hashes for the 19 owned source/test files are recorded in [browser-reads-handoff.md](browser-reads-handoff.md).

The original Task01 query-plan check used a simplified ID-only query, and its PostgreSQL cases were skipped because `KANDEV_TEST_POSTGRES_DSN` was unset; the Era Sol remediation replaces that evidence with full production first-page and continuation plans on both engines, as recorded in the handoff. Parent acceptance review remains pending; this result does not claim candidate browser-performance acceptance or implement later work orders.

Era Sol's 2026-11-13 review remediation is recorded in the [updated Task01 handoff](browser-reads-handoff.md). It adds dialect-appropriate repeatable snapshots for summary/history/inspection, canonical unread ordering, production-query plan checks for both engines, deterministic legacy-time normalization and a post-upgrade history-index assertion for the v0.93.0 fixtures. The final remediation race run used an isolated local PostgreSQL cluster and had zero skips; the earlier no-DSN statement above remains the historical receipt for the original implementation run.

The 2026-11-13 SQLite timestamp follow-up normalizes zone offsets against the whole second before restoring the stored fraction, preserves parser-supported zone-less space timestamps, fractional forms, boundary offsets and year-boundary rollover, and keeps invalid values at the deterministic epoch key. Ordering, cursor seeks, projected timestamps and the normalized history index share that key. The unshipped predecessor index is dropped at initialization and the current key uses a versioned index name. Focused pagination regressions pass on bundled SQLite 3.51.1 and SQLite 3.45.1; history, production plan, index replacement and stable-fixture upgrade checks pass for SQLite and PostgreSQL 16 with no skips. Task01 remains in progress pending parent acceptance; exact RED/GREEN receipts and limits are in the handoff.

The remaining single-digit-hour parser mismatch is fixed in the [updated Task01 handoff](browser-reads-handoff.md): SQLite pads only a validated one-digit hour before using the shared parser-differential key, and the index is now v3 with both unshipped predecessor names removed at initialization. The expanded pagination/projection/cursor matrix, both-engine index replacement and v0.93.0 upgrade checks, SQL guard, and scoped lint pass; Task01 remains in progress pending parent acceptance.
