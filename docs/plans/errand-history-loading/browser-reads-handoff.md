# Task01 browser-read handoff

## Delivered

Added the three additive WebSocket read actions and compact DTOs. Browser summary returns the live or otherwise-outstanding projection plus task and parent counts; history uses an immutable `created_at DESC, id ASC` keyset cursor scoped to task and parent; inspection verifies one child and returns only its compact row and minimal parent identity. The history SQL reads `limit+1` before scanning or converting rows, trims the lookahead before DTO conversion, and calculates counts in the same read transaction. Browser queries do not call the complete-list service method or perform per-finished-child detail reads.

Workspace-read authorization precedes all three service calls. These read actions remain available for retained inspection when the Errands flag is off; mutation guards and their behavior are unchanged. The full `errand.list`/service/MCP inventory and task-wide cancellation behavior remain intact. Web API methods surface unsupported-action errors directly and do not fall back to the complete list; `listErrands(taskId)` retains its existing signature.

The SQLite schema now has the task/parent/created-at/ID history index. The query-plan test verifies its use and the absence of a temporary order-by b-tree. Differential summary tests compare SQL status, outstanding, permission/recovery attention, unread state, and counts with the complete-state service oracle, including queued, continuation, pending-result, recovery, unread, failed, cancelled, and missing-session rows. History tests cover page bounds, timestamp ties, cursor scope/version/order errors, deletion between pages, foreign scope, and compact projections.

## Verification receipts

All Go commands ran under `/tmp/kandev-errands-repair-build.lock` with `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, and the baseline Go caches. The final race command was:

```bash
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache go test -race -p 1 -parallel 1 ./internal/task/repository/sqlite ./internal/errands ./internal/task/handlers -run 'Errand' -count=1)
```

It passed for all three packages: SQLite 56.597s, errands 2.761s, handlers 1.141s. Four PostgreSQL cases were skipped because `KANDEV_TEST_POSTGRES_DSN` was unset: the history-page, summary, and query-plan repository cases, plus the service-oracle case. The scoped non-race run also passed SQLite, errands, handlers, and DTO tests with `-run 'ErrandBrowser|ErrandRequests|ErrandActionsAreRegistered|ErrandRequestsRejectBadPayloads' -count=1`; it was run before the final query-helper and oracle-assertion extraction.

The focused non-race command was:

```bash
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache go test -p 1 -parallel 1 ./internal/task/repository/sqlite ./internal/errands ./internal/task/handlers ./internal/task/dto -run 'ErrandBrowser|ErrandRequests|ErrandActionsAreRegistered|ErrandRequestsRejectBadPayloads' -count=1)
```

The exact changed-Go lint command was:

```bash
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache bash scripts/lint-go-changed \
  apps/backend/internal/errands/interfaces.go \
  apps/backend/internal/errands/browser_reads.go \
  apps/backend/internal/errands/browser_reads_test.go \
  apps/backend/internal/errands/browser_reads_repository_test.go \
  apps/backend/internal/task/models/errand_browser.go \
  apps/backend/internal/task/repository/interface.go \
  apps/backend/internal/task/repository/sqlite/errand_browser_reads.go \
  apps/backend/internal/task/repository/sqlite/errand_browser_reads_test.go \
  apps/backend/internal/task/repository/sqlite/errands_schema.go \
  apps/backend/internal/task/repository/sqlite/errands_scenarios_test.go \
  apps/backend/internal/task/dto/errand_browser.go \
  apps/backend/internal/task/dto/errand_browser_test.go \
  apps/backend/internal/task/handlers/errand_ws_handlers.go \
  apps/backend/internal/task/handlers/errand_ws_handlers_test.go \
  apps/backend/pkg/websocket/actions.go
```

The web API and type tests passed 54 tests in 3 files:

```bash
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 corepack pnpm@9.15.9 exec vitest run lib/api/domains/errand-api.test.ts lib/api/domains/errand-api.browser.test.ts lib/types/errands.test.ts)
```

`flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 corepack pnpm@9.15.9 run typecheck` passed. Exact-file Oxlint and the matching i18n rules both passed:

```bash
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 corepack pnpm@9.15.9 exec oxlint -c oxlint.config.mjs --deny-warnings lib/api/domains/errand-api.ts lib/api/domains/errand-api.test.ts lib/api/domains/errand-api.browser.test.ts lib/types/errands.ts lib/types/errands.test.ts)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 corepack pnpm@9.15.9 exec oxlint -c oxlint.i18n.config.mjs --deny-warnings lib/api/domains/errand-api.ts lib/api/domains/errand-api.test.ts lib/api/domains/errand-api.browser.test.ts lib/types/errands.ts lib/types/errands.test.ts)
```

`bash scripts/lint-go-changed` passed with zero issues for the 15 owned Go files listed in the source-hash table. Final `gofmt -l` and `git diff --check` returned no findings; the targeted trailing-whitespace scan reported none across the owned source, test, and documentation files.

## Limits and follow-up

PostgreSQL execution was unavailable in this environment, so the dialect branch and index plan need CI or a configured PostgreSQL run. Counts intentionally use indexed aggregate scans over matching membership; this work makes no claim that SQL rows scanned are constant with history size. The new API is additive and has no boot/list consumers yet; ordinary isolation, demand-driven UI, direct-link browser behavior, desktop/phone visual checks, and candidate performance acceptance belong to later work orders. Era Sol acceptance review is pending. No commit, push, live runtime, or live database operation was performed.

## SHA-256 source manifest

These hashes identify the final Task01 source and test files reviewed for this handoff.

| File                                                                        | SHA-256                                                            |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `apps/backend/internal/errands/interfaces.go`                               | `189e50eae482327d5ddfa6cbf6e2acd21f0d5174071e120f07235b9faab71af1` |
| `apps/backend/internal/errands/browser_reads.go`                            | `34902b7877477e27e40bbf917cd5c70f9c624dabc31be3fd189d13269372a4f8` |
| `apps/backend/internal/errands/browser_reads_test.go`                       | `236cd6479c149fbd47a6c7acf2c6f59ff39685c61520fe619ef81ee5880a2dae` |
| `apps/backend/internal/errands/browser_reads_repository_test.go`            | `ded3e7457bdaba1d3c87c557fb4ee64a2ecfa9cd44a0ed51ea16657874de3e2a` |
| `apps/backend/internal/task/models/errand_browser.go`                       | `37e7c01a75d542b34fcea3d1a014277a3750aa423b0bffc6a2cdb1a23314b17b` |
| `apps/backend/internal/task/repository/interface.go`                        | `513beec2532493237cdd168fc41dbb6c3bee9f0428d69f7afab53c941b653a72` |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads.go`      | `ec9fe79bc83c8d81e05fd003091e8095aa406f3d33a85ab7d3b70a4db9c18532` |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads_test.go` | `47a8099ab86b3443fbbed00584de94f3ff21c62de18371e67c8480c288c11e5d` |
| `apps/backend/internal/task/repository/sqlite/errands_schema.go`            | `262dc278d2f999acfb511ac88042456afb70f5a11dc92add976a4f1b47f9efd7` |
| `apps/backend/internal/task/repository/sqlite/errands_scenarios_test.go`    | `18aefdb7ba1e099bddc021074257ae003afaf52936b1c9c87bc350a781437861` |
| `apps/backend/internal/task/dto/errand_browser.go`                          | `72d4ad59b123386a23dfa5d1c27ec84acc06ce822de1905ae7cadb5e530cbd7d` |
| `apps/backend/internal/task/dto/errand_browser_test.go`                     | `f89b496b6c481aa4483d88eeb17b89d341958be2efbeb09ca98811b50b4f4d8d` |
| `apps/backend/internal/task/handlers/errand_ws_handlers.go`                 | `3b24f65c90421385106f95c13b89bd6995cec7cf1debcafc8bd9ebaf8509cca7` |
| `apps/backend/internal/task/handlers/errand_ws_handlers_test.go`            | `ddef6373674e01b96aa210450c90f8d0b2d1a0f89e1cded6018907c36788a5e4` |
| `apps/backend/pkg/websocket/actions.go`                                     | `8b8dc3ca52275581c4f729ce822ed93249d8aabe797979d68ff03634bc0c3f76` |
| `apps/web/lib/types/errands.ts`                                             | `53080a4bede45c2d15d6f76a3fab79a2af25c10bf5ada0d5a6819831c51011d5` |
| `apps/web/lib/types/errands.test.ts`                                        | `f2c7acb9c8490e8ce2732564518a12bbcac630bbb275b1f5a9d418bdab2aa0c4` |
| `apps/web/lib/api/domains/errand-api.ts`                                    | `da8cf19204eb889c2bc80eef02ee0ed10bfebca45e8b3cb815f938f64a66a3a8` |
| `apps/web/lib/api/domains/errand-api.browser.test.ts`                       | `e8c6d419ee464b3a9de59eadb2da199fdf1afa9713b9ba9c1e1367b34b5efc09` |

## Era Sol review remediation (2026-11-13)

The initial verification receipts and hashes above are the pre-review Task01 snapshot; the following focused remediation supersedes them for the listed files. Review findings are repaired without changing web/client/handler behavior or Task02. No aggregate constant-scan claim is made.

- **P1, coherent snapshots:** Summary, history, and child-plus-parent inspection now share `beginErrandBrowserReadSnapshot`. SQLite keeps its read-only transaction behavior; PostgreSQL requests `REPEATABLE READ`, following the existing conversation-source read pattern. Deterministic two-connection PostgreSQL tests change an errand from finished to outstanding between the count and membership reads; the summary test includes task counts, parent counts, and membership, and the history test includes total and page membership. Both now observe one committed snapshot.
- **P2, unread rank:** Browser unread SQL now uses the same normalized-microsecond timestamp and byte-ordered ID comparison as canonical session unread ranking. SQLite and PostgreSQL differential cases use equal-microsecond timestamps with opposing ID order and compare against `ListSessionUnreadState`.
- **P2, query plans:** Production and EXPLAIN share `errandBrowserHistoryPageQuery`, so the first-page and continuation plans cover the actual compact projection, session join, recovery/unread predicates, cursor and limit. Each engine is seeded with 1,024 rows across 32 parents and analyzed. Both plans use `idx_task_session_errands_browser_history_normalized_v3`. SQLite has no outer temporary ORDER BY b-tree; nested temporary sorts remain inside recovery subqueries. PostgreSQL's captured outer sort after the join uses exactly `created_at DESC, id COLLATE "C"`; continuation uses the same scoped index for its cursor predicate. The plans and test assertions do not describe correlated recovery work as constant.
- **P2, malformed/zero times:** SQLite ordering, cursor key, selected projection and scan now share a UTC nanosecond ordering key; malformed values normalize to the epoch, matching the existing selector's invalid-time rank, and equal keys break by byte-ordered ID. Valid SQLite zero/year-one values remain distinct and page correctly. PostgreSQL stays on typed timestamp comparison/scanning, where malformed text cannot be stored. The service cursor encoder now accepts a valid Go zero/year-one timestamp instead of rejecting it as a missing cursor.
- **Explicit-tag fixture:** The v0.93.0 SQLite and PostgreSQL SQL fixtures predate `task_session_errands`, so their bytes and recorded checksums stay untouched. Their manifest entries now declare the history index as a post-upgrade expectation, and the manifest validator plus `TestPreviousStableUpgrade` verify that current initialization creates it for both engines and remains replayable.

### RED/GREEN receipts

RED runs below were captured during this remediation before the corresponding production fixes; no RED result was reconstructed after the fix.

| Finding                                             | RED evidence                                                                                                                                                                                                                                                                                                                                                                 | GREEN evidence                                                                                                                                                                                                                                                                                                                                                     |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Snapshot, history count/page                        | `/tmp/kandev-errands-repair-red-snapshot.log`, SHA-256 `b4d653072f7fbf8274785451d6384fe1176ad9400adde4f3c083bfd577c96616`; READ COMMITTED returned total 1 with zero page rows after a concurrent update.                                                                                                                                                                    | `/tmp/kandev-errands-repair-final-race.log`, SHA-256 `eb5fbd260f4f7be93db2d4ff921d2f67a2e820c7cbdffcbc53eec4bbda0f4db3`; `TestPostgresErrandHistoryCountAndMembershipUseOneSnapshot` passes.                                                                                                                                                                       |
| Snapshot, summary task/parent counts and membership | `/tmp/kandev-errands-repair-red-summary-snapshot.log`, SHA-256 `b1930be20ada94e0308ee88e792a8da1889d5a0d5ca8c4158a29c0c2b1904764`; READ COMMITTED kept both counts at zero while membership saw the newly outstanding row. The wrapper also printed zsh's `status` readonly-variable notice after the expected test failure; the test failure is the preserved RED evidence. | `/tmp/kandev-errands-repair-green-summary-snapshot.log`, SHA-256 `4bf35676dab6129d1a323c4f4f085070de8063077b8d25565d14e7c2c940cb8b`; the same interleaving passes with the production snapshot helper and row query.                                                                                                                                               |
| Unread rank and malformed legacy times              | `/tmp/kandev-errands-repair-red-sqlite.log`, SHA-256 `f3d320ddd3262c6d175bf06a872bca04be710baa4b90d252c922e923549999ec`; two failures: malformed-time pagination rejected its generated cursor, and unread differed from the canonical oracle.                                                                                                                               | `/tmp/kandev-errands-repair-green-sqlite-finaltarget.log`, SHA-256 `584eaabb6f577208cd0a07d71e770e32447ecb58b6bc6acf7cdc948469881381`; four SQLite browser-history tests pass. `/tmp/kandev-errands-repair-green-postgres-finaltarget.log`, SHA-256 `96887da7b2fa1325f085e50b06448de9213dfedf2a40464caf08ee89a916d469`; six PostgreSQL browser-history tests pass. |
| Plan/index behavior                                 | `/tmp/kandev-errands-repair-red-index.log`, SHA-256 `770394ed18f745551612806b8a7908d7bc5fd3ab0864115c4fe1f1f01067aec0`; the scoped history index was absent.                                                                                                                                                                                                                 | `/tmp/kandev-errands-repair-green-plans-both.log`, SHA-256 `2fb7c7558a5181d31d1281a4c80266b504f7708b321fec51af8eca6cfda8b9da`; it records actual first-page and continuation plans for both dialects and passes both tests.                                                                                                                                        |
| Valid zero-time cursor                              | `/tmp/kandev-errands-repair-red-service-cursor.log`, SHA-256 `7094c368ddc7b919afb2d763ed2fd398ba08fbac64eb9a0633fa3753ad70aa49`; the service encoder rejected a valid year-one timestamp.                                                                                                                                                                                    | `/tmp/kandev-errands-repair-green-service-cursor.log`, SHA-256 `eb090a610622f3fb021a3e62802852c7f8d5a42055172185adbafe2720bb00de`; zero-time cursor round-trip and neighboring scope validation pass. The final three-package race run also passes the zero-time cursor test.                                                                                      |

### Final remediation checks

The final affected-work-order race command ran 234 top-level tests across the repository, Errands service, and handlers packages, with zero skips: SQLite/PostgreSQL repository `268.113s`, service `3.830s`, and handlers `1.112s`. The command was `cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env KANDEV_TEST_POSTGRES_DSN='postgres://postgres@127.0.0.1:55488/postgres?sslmode=disable' GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache go test -trimpath -race -p 1 -parallel 1 ./internal/task/repository/sqlite ./internal/errands ./internal/task/handlers -run 'Errand' -count=1 -v`; log `/tmp/kandev-errands-repair-final-race.log`, SHA-256 `eb5fbd260f4f7be93db2d4ff921d2f67a2e820c7cbdffcbc53eec4bbda0f4db3`.

The required `go run ./cmd/sqlguard ./internal` completed with exit 0 and no guard findings; log `/tmp/kandev-errands-repair-sqlguard-final.log`, SHA-256 `19eaf43821a7660ec323a87c8457bf74823beb296c39f5e01aa8a683aa50f061`. The required `go test -trimpath -race -p 1 -parallel 1 ./internal/persistence/storeconformance -count=1 -v` passed 11 top-level tests with zero skips, including the SQLite and PostgreSQL v0.93.0 upgrade fixtures; log `/tmp/kandev-errands-repair-storeconformance-race.log`, SHA-256 `263818518088a688a7fff65145f75942d3bbf8362e256e6709b69458271930c7`.

The focused both-dialect plan rerun passed two top-level tests and printed all four production plans (first page and continuation for SQLite and PostgreSQL) to `/tmp/kandev-errands-repair-green-plans-both.log`, SHA-256 `2fb7c7558a5181d31d1281a4c80266b504f7708b321fec51af8eca6cfda8b9da`. Changed-Go lint passed with zero issues for the remediation-owned Go files; log `/tmp/kandev-errands-repair-go-lint-final.log`, SHA-256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`. Final `gofmt -l`, trailing-whitespace scan and `git diff --check` were clean; their empty-output receipts are `/tmp/kandev-errands-repair-gofmt-final.log`, `/tmp/kandev-errands-repair-trailing-whitespace.log`, and `/tmp/kandev-errands-repair-diff-check.log`, each SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

PostgreSQL coverage used the already-present local PostgreSQL 16 disposable cluster under `/tmp/errands-pg-check/data`, bound only to `127.0.0.1:55488`, with isolated test schemas. No image or service was installed or pulled. The cluster was stopped cleanly after checks; its process and `postmaster.pid` are absent and the shared build lock is released. Stop receipt: `/tmp/kandev-errands-repair-pg-stop.log`, SHA-256 `b5588a95fba6b7bc68e0b8477568aa8500d8d4e6efab87741ec6d7beeb0cb4f4`. This does not certify a production database or any external PostgreSQL deployment.

### Current remediation hashes

These hashes supersede the earlier table for files listed here; other entries above remain the unchanged pre-review Task01 snapshot. The handoff's own SHA-256 is reported in the parent result to avoid a self-referential checksum.

| File                                                                                         | SHA-256                                                            |
| -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `apps/backend/internal/errands/browser_reads.go`                                             | `171a197438be22c87019fdad60f1c6b7c28eb73168aa1e889ba22a0f2c362fd2` |
| `apps/backend/internal/errands/browser_reads_test.go`                                        | `ff06ba8b64c0f2a7d21bc17e2a3d19065473e4b744da237cf72fc24a4e8b9f7d` |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads.go`                       | `c5421b01352c58b2fc07d20d13b4cf6fa9f70a0176b32bd7c83860b68eac7ee7` |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads_test.go`                  | `84c2823d69cc35eca732df96f3031c8b9cc6ac00faea674b2a9f6630e3ce873f` |
| `apps/backend/internal/task/repository/sqlite/errands_schema.go`                             | `c33fb5c29c84b3d47e12524241e46a96f1b081313b038c9aa96ec436757b39f5` |
| `apps/backend/internal/task/repository/sqlite/errands_scenarios_test.go`                     | `7bf01339f2c82815376a6cec856e6ec27fbf802d410e48f77ec91d2f4eab5304` |
| `apps/backend/internal/persistence/storeconformance/upgrade_test.go`                         | `5e23128180e95075a641b5745a46fa353cf28d5474d1c5b6f341bf366ab281ec` |
| `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json` | `425c322287f92e1afc71a41b08c34100ee4a337fde60f09d536797e4008fba2a` |
| `docs/plans/errand-history-loading/task-01-browser-reads.md`                                 | `0416d3d954f827740c911595ad3d4056c970e2cd3692857bb59dd1207ccabea0` |

## SQLite timestamp normalization follow-up (2026-11-13)

The remaining Era Sol findings are fixed. SQLite now passes only the timestamp's whole second to `datetime()`, applies a validated numeric zone through inverse hour/minute modifiers, and then restores the stored fraction; this avoids shifting fractional values twice on SQLite versions that round fractional inputs. Legacy zone-less `YYYY-MM-DD HH:MM:SS` values and their supported fractional forms use UTC, matching `parseTaskActivityTime`. The same expression backs history ordering, cursor seek, selected timestamp projection and the expression index. Dot and comma fractions from one through nine digits, parser-supported extra precision, both offset signs including the parser-accepted hour/minute boundaries, space-offset values, year one and an offset crossing into year zero, invalid dates/fractions/offsets and trailing garbage are covered in a parser-differential pagination/cursor test; invalid times retain the epoch rank.

That follow-up assigned expression index version `idx_task_session_errands_browser_history_normalized_v2` and dropped its unshipped predecessor index name before creating the current index. This was a pre-release index correction, not a new stable-version migration claim. The unchanged v0.93.0 SQLite and PostgreSQL fixture bytes continue to verify that current initialization creates the expected index and remains replayable.

### Timestamp RED/GREEN receipts

| Finding                                                                                          | RED evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | GREEN evidence                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fraction plus offset can cross a second/day boundary                                             | `/tmp/kandev-errand-history-offset-old-sqlite-red.log`, SHA-256 `0ed4858c3e32ffa40cf75f2b53c7b78e59ba37132980b6077ec35243d1b81338`; the failing repository regression used a temporary modfile with go-sqlite3 v1.14.22 / SQLite 3.45.1 and observed `23:00:01.999999999` instead of `23:00:00.999999999`.                                                                                                                                                                             | `/tmp/kandev-errand-history-timestamp-final-sqlite345.log` and `/tmp/kandev-errand-history-timestamp-final-default.log` pass the rollover regression and parser-differential matrix on SQLite 3.45.1 and pinned SQLite 3.51.1; their hashes are recorded in the final receipts below. |
| The Go parser accepts boundary-hour/minute offsets that SQLite's timestamp suffix parser rejects | `/tmp/kandev-errand-history-offset-boundary-red.log`, SHA-256 `02451c9752c926108a31625c7a883951a4977774e595d9d57322fbed6490a99d`; the repository ordering differed from the accepted Go-parser offset.                                                                                                                                                                                                                                                                                 | Both final SQLite matrix logs below pass the parser-differential ordering, projection and cursor checks for boundary offsets on SQLite 3.45.1 and 3.51.1.                                                                                                                             |
| Zone-less legacy values, parser-supported fractions and predecessor index                        | `/tmp/kandev-errand-history-timestamp-red-focused.log`, SHA-256 `d29c923172cd7e8fd65ac094d73a61c017773f6d5c08b3001dfe9c984cf72a84`; the SQL key ranked a zone-less timestamp at epoch and retained the predecessor index. `/tmp/kandev-errand-history-fraction-forms-red.log`, SHA-256 `ac303bde2b2bb6978d540049d8e47cfb2cf78c288c75df0391e4a2d621adbc08`, also shows parser-supported precision beyond nine digits disagreeing with the SQL key before the final coverage refinement. | The final SQLite matrix logs below pass table-driven key, ordering, projection and cursor checks; the full browser run passes index replacement and first-page/continuation plans for both dialects.                                                                                  |
| Malformed suffix after a zone-less fractional value                                              | `/tmp/kandev-errand-history-garbage-red.log`, SHA-256 `fa9820892c4e39221ff344c53f1740f197b1c923d98e8951beaba475d305a95f`; the legacy branch incorrectly accepted trailing zone garbage.                                                                                                                                                                                                                                                                                                | The final SQLite matrix logs below include `garbage-after-space-fraction` and pass it as epoch-ranked malformed input.                                                                                                                                                                |

### Timestamp verification and limits

The final focused history/browser repository run passed SQLite and PostgreSQL cases with zero skips in `/tmp/kandev-errand-history-browser-final-both.log`, SHA-256 `b43480aeb73df7e76677dafdfd24321c6a09b8c80bdb95536ca915ace307921a`; it includes first-page and continuation production query plans on both engines. The v0.93.0 upgrade/manifest run passed both dialects in `/tmp/kandev-errand-history-upgrade-final-both.log`, SHA-256 `b9f122afc3aaaeb7d08bb847d41231592019196714158785ebd16edb1e3a59f9`. The final single-engine timestamp matrix receipts are `/tmp/kandev-errand-history-timestamp-final-sqlite345.log` (SQLite 3.45.1) and `/tmp/kandev-errand-history-timestamp-final-default.log` (pinned SQLite 3.51.1); their hashes are recorded below.

The final SQL guard reported no findings and scoped changed-Go lint reported zero issues. `gofmt -l`, `git diff --check`, the owned-file trailing-whitespace scan and manifest JSON validation all passed. The SQLite timestamp correction did not rerun the earlier three-package race suite because service, handler, PostgreSQL production query, web and browser behavior were unchanged.

The earlier Task01 three-package race run remains prior evidence and was not repeated for this SQL-only correction; PostgreSQL production query code, service, handlers, web code and browser flows were unchanged. The disposable PostgreSQL 16 cluster was started for the two-dialect checks and stopped afterward. Final stop output is `/tmp/kandev-errand-history-pg-final-stop.log`, and final `pg_ctl status` reports no server in `/tmp/kandev-errand-history-pg-final-status.log`; their hashes are recorded below. No main database, runtime, commit, push, Task02 code or user data was used.

The timestamp-specific RED/GREEN evidence and cleanup receipts have these SHA-256 values:

| Receipt                                                    | SHA-256                                                            |
| ---------------------------------------------------------- | ------------------------------------------------------------------ |
| `/tmp/kandev-errand-history-timestamp-final-sqlite345.log` | `f13a12d42285bb6b44ecf8f56e9c95ca64d991dbf508beaa4fae952f7edd8839` |
| `/tmp/kandev-errand-history-timestamp-final-default.log`   | `702203388413225f8af9d64fd5a11610affa77b8cb844b4b98c8776a899aaf31` |
| `/tmp/kandev-errand-history-yearone-default.log`           | `0aea4a6e6bbb3e0dcaed3bdd3a1b3a36acf750bb0a0f0aa275c5b832e00f96fb` |
| `/tmp/kandev-errand-history-yearone-sqlite345.log`         | `b23f5c10a2ed921f181254fcdfec8e64b2e597c5bb4b29bbe398ba4e89796ebd` |
| `/tmp/kandev-errand-history-sqlguard-final.log`            | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-errand-history-go-lint-final.log`             | `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47` |
| `/tmp/kandev-errand-history-gofmt-final.log`               | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-errand-history-diff-check-final.log`          | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-errand-history-trailing-whitespace-final.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-errand-history-pg-final-stop.log`             | `e7b24f63c690806243625ca8cb53f81f1a71a030c6c34a3820e76021c6810564` |
| `/tmp/kandev-errand-history-pg-final-status.log`           | `e138ccb54fdb08283c6eb21159c53207bbbbf07077246e2804e18d22efe6e250` |

## Single-digit-hour timestamp follow-up (2026-11-13)

The remaining P2 finding was that `parseTaskActivityTime` accepts a one-digit hour such as `2024-01-05 1:00:00.123`, while SQLite's shared normalizer required fixed-width `HH` positions and therefore assigned the epoch fallback. The normalizer now pads a one-digit hour only when the value has a valid date shape, a supported `T` or space separator, a decimal hour digit, and the following colon. All later time, fraction, and zone validation remains in the shared expression, so malformed clock text still receives the epoch key.

The parser-differential pagination test now covers zone-less whole seconds and dot/comma fractions, `T`-separated UTC and numeric-offset forms, space-separated numeric-offset forms with dot/comma fractions, and invalid clock text and range. The added valid forms are explicitly checked with `parseTaskActivityTime`; the existing offset-boundary, year-one, and garbage-suffix cases remain in the same matrix. SQL key, selected timestamp, page ordering, and cursor continuation therefore stay compared against that parser.

Because the expression index stores the normalized key, its name is now `idx_task_session_errands_browser_history_normalized_v3`. Initialization drops both unshipped predecessor names, the original unnormalized index and v2, before creating v3. The v0.93.0 SQLite and PostgreSQL upgrade manifest now expects v3, and its unchanged fixture bytes still pass replayable upgrades. This remains a pre-release index correction, not a new stable-version migration claim.

### RED/GREEN and verification receipts

The RED run failed at the parser-vs-SQL ordering assertion: row 3 expected the parsed `single-digit-hour-space-offset` row but got `zone-less-comma-fraction`, because SQLite had ranked the single-digit value as the epoch. Its log is `/tmp/kandev-errand-history-single-digit-hour-red-assertion.log`, SHA-256 `c6a4871e75a2119c90c89cb742602bdb2aca02eb6c47853362d7ddbfaa1637d3`.

The focused SQLite pagination, projection, cursor, index-replacement, and first-page/continuation plan run passed five tests using the bundled SQLite engine. Its log is `/tmp/kandev-errand-history-single-digit-hour-green-sqlite.log`, SHA-256 `6460a6d38d750c93a88889fabbbf66beac724adcd3f4ada5667f1a080ab769e7`; the production plans use the v3 index.

The both-engine index and upgrade run passed PostgreSQL index replacement and first-page/continuation plan tests plus `TestUpgradeFixtureManifest` and `TestPreviousStableUpgrade` for both SQLite and PostgreSQL, with no skips. Its log is `/tmp/kandev-errand-history-single-digit-hour-green-both-engines.log`, SHA-256 `ff662aee969876ab885879aec3e80bfebafd29c3d7b5c8f9f1febc2617cbac64`.

`go run -trimpath -p 1 ./cmd/sqlguard ./internal` passed without findings; log `/tmp/kandev-errand-history-single-digit-hour-sqlguard.log`, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. Scoped changed-Go lint passed with zero issues; log `/tmp/kandev-errand-history-single-digit-hour-go-lint.log`, SHA-256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`. `gofmt -l`, JSON manifest parsing, and `git diff --check` passed.

Both-engine checks used the existing disposable PostgreSQL 16 cluster at `/tmp/errands-pg-check/data`, bound only to `127.0.0.1:55488`; it was stopped after the test run and the shared build lock was released. This check does not certify any production PostgreSQL deployment. Task01 remains in progress pending parent acceptance; Task02 remains untouched and blocked.

### Current follow-up hashes

These hashes identify the source, tests, schema code, fixture metadata and work order updated in the latest follow-up; they do not include this handoff so it does not checksum itself.

| File                                                                                         | SHA-256                                                            |
| -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads.go`                       | `e8b3a593d49eeea2e2ca5b008c9941324193e5737fd0678be148187efe935b66` |
| `apps/backend/internal/task/repository/sqlite/errand_browser_reads_test.go`                  | `6f5397901c89c1d984e0a9423f073d3a7e21cd0a94d8cbb02deb5ce376b9948a` |
| `apps/backend/internal/task/repository/sqlite/errands_schema.go`                             | `da641a4ff8070dc5b32bdfb919e7d6816fb3ef3844f970654614a411f37afc26` |
| `apps/backend/internal/persistence/storeconformance/upgrade_test.go`                         | `e1076d646bcc71930b0111b4578daddc835b0772be14c277d274004177d5afa5` |
| `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json` | `80e17c4efc3e9b719f2b11a2c93aa6d9f08fcd2c0b4c7041b67bf55a0f291edd` |
| `docs/plans/errand-history-loading/task-01-browser-reads.md`                                 | `8e7084bee83c6be95e74f4de0087636fbd1ca071a4d40ae0e3e198052ce3ab17` |

No commit, push, live runtime, or live database operation was performed. Task01 remains in progress pending parent acceptance; Task02 remains untouched and blocked.
