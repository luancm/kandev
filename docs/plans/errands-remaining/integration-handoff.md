# Integration checkpoints

## Current delivery and acceptance, 2026-11-12

All four behavior groups are implemented on `feature/plan-errands-first-d-424`: exact parent notification delivery, recovery attention and safe continuation, immutable historical elapsed time, and durable successful runtime cleanup. Pushed source `452c9d4f0d3bc763ccfc43661ee91f7282425de0` includes upstream main `37f081a976526de1a6967f8714e72e247ec0b386` via merge `e35b7f6e23`, following the earlier 80-commit merge `0067adc210`. Early delivery, upstream refresh, and subsequent verification follow the user's requested order. Earlier entries below are historical checkpoints, including their then-pending states.

Managed browser acceptance passed 21/21 desktop and 19/19 phone cases on source `452c9d4f0d` plus the final test-only context-fixture correction. The fixtures now require successful child sessions to close to `COMPLETED` before seeding retained context. The 60-second polling bound covers the existing 20-second cleanup sweep; the lifecycle regression required a production correction and did not pass merely by extending that timeout. Logs: `/tmp/kandev-final-latest-desktop.log` and `/tmp/kandev-final-latest-phone.log`.

The first affected backend run exposed a gateway test constructing clients without a Hub after upstream dispatch authorization began using it. The fixture now provides the real Hub and asserts authenticated session identity while retaining the foreign-workspace negative control. Its full race suite passed: 473 top-level tests, one opt-in dump probe skipped (`/tmp/kandev-final-gateway-fixture-green.log`). No gateway production behavior changed.

A separate lifecycle failure reproduced three times during test schema initialization (`/tmp/kandev-final-lifecycle-fts-repro.log`). The default SQLite factories interpolated native paths directly into file URIs. A `#00` component truncated the requested temporary path and opened an unrelated old FTS5 database. New writer/reader regressions independently failed for literal hash, percent and query characters, with ordinary spaces passing (`/tmp/kandev-sqlite-native-path-red.log`). Both factories now reuse `EscapeSQLiteURIPath(filepath.ToSlash(normalizedPath))`, matching the existing native backup and inventory-repair contract. There is no migration, new capability or relaxed ownership check. Full database race acceptance and the affected lifecycle rerun passed after the correction; SQL guard and race store conformance also passed. Receipts are in `/tmp/kandev-final-latest-backend-repair.log`.

The browser source checkpoint predates only this filename-factory correction and final test/docs changes. Its normal temporary filenames retain identical URI encoding. Database tests independently cover literal paths and read-only enforcement; lifecycle reruns cover the actual failing initialization. Browser results are not presented as a run of a later hash.

### Final command scope

Heavy commands run sequentially under `/tmp/kandev-errands-repair-build.lock` with `SHELL=/bin/bash`, `GOMAXPROCS=2`, `GOMEMLIMIT=6GiB`, and `GOFLAGS='-trimpath -p=1 -gcflags=github.com/kandev/kandev/internal/orchestrator=-l'`. Only test/E2E builds disable orchestrator inlining; the final production build uses normal optimization. Workspace tooling is pnpm 9.15.9 from upstream's frozen lockfile.

From `apps/backend`, the affected race selector is:

```bash
go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle ./internal/automation ./internal/orchestrator/executor ./internal/task/service -run 'Errand|ParentTerminalIdentity|ColdResume|AgentDeliverySessionState|DeliverySessionState|DeliveryRetention|Terminal|SettledDelivery|NativeResume|RestartRecovery|SilentRestore|SessionRecoveryBlock|RetryLaunch|ContextContinuation|StopExecution|SendNow|Hash|Replay|Compatibility|CurrentTurn' -count=1 -v
```

The successful orchestrator, SQLite, automation, executor and task-service results are retained in `/tmp/kandev-final-latest-backend-remaining.log`; its failed lifecycle result is superseded only by the matching package/selector rerun in the repair log. Full race suites also cover `internal/errands`, `internal/common/wsinput`, `internal/common/wskeepalive`, `internal/agent/loginpty`, `internal/gateway/websocket`, `internal/db` and `internal/persistence/storeconformance`. The first four are in `/tmp/kandev-final-latest-backend.log`, whose failed gateway attempt is superseded by the gateway log above. SQL guard runs `go run -trimpath -p 1 ./cmd/sqlguard ./internal`.

Managed browser commands use `pnpm --dir apps/web e2e:run --host --shards 1`, project `chromium` for `errands.spec.ts`, `errand-result-duration.spec.ts`, `repeated-native-resume.spec.ts`, and project `mobile-chrome` for their phone counterparts, with retries disabled and one worker. The fresh combined build and standard global setup were used; subsequent fixture-only desktop rerun and phone reused matching build artifacts with `--no-build`.

From `apps/web`, final web checks run typecheck, the full Errands component test directory plus affected session input/message-handler, PR status and quick-terminal tests, `i18n:check`, `i18n:ratchet`, and `e2e:sleep-ratchet` (`/tmp/kandev-final-latest-web.log`). The final optimized build from repository root is `env GOFLAGS='-trimpath -p=1' go -C apps/backend build -trimpath -p 1 -tags fts5 -o /tmp/kandev-errands-merged-check ./cmd/kandev` (`/tmp/kandev-final-optimized-build.log`).

### Final source verification

All final source checks passed. Counts below are top-level Go tests; nested subcases are additional. PostgreSQL and other gated skips remain explicit, including nested PostgreSQL skips inside passing conformance cases.

| Check | Result | Receipt |
| --- | --- | --- |
| Full Errands, wsinput, wskeepalive and loginpty race suites | 169 passed, four PostgreSQL tests skipped | `/tmp/kandev-final-latest-backend.log`; its gateway failure is superseded below |
| Full gateway race suite | 473 passed, one dump probe skipped | `/tmp/kandev-final-gateway-fixture-green.log` |
| Affected orchestrator, SQLite, automation, executor and task-service race selectors | 1,066 passed, 126 gated tests skipped | `/tmp/kandev-final-latest-backend-remaining.log`; its lifecycle failure is superseded below |
| Full database race suite; matching affected lifecycle selector | 45 and 170 passed respectively, one gated test skipped in each package | `/tmp/kandev-final-latest-backend-repair.log` |
| SQL guard; full race store conformance | Passed; 11 top-level conformance tests passed, PostgreSQL subcases skipped without DSN | Same repair log |
| Frontend | Typecheck; 228 tests in 17 files; translation, new-copy and E2E wait guards passed | `/tmp/kandev-final-latest-web.log` |
| Normal optimized production build with FTS5 | Passed after the native-filename correction | `/tmp/kandev-final-optimized-build.log`; verification driver exited zero |
| Managed desktop and phone browsers | 21/21 desktop, 19/19 phone | Browser receipts above, with their exact source scope |

The unique final Go results total 1,934 top-level passes and 133 top-level gated skips; failed attempts are retained as RED evidence and are not counted as successful acceptance. Earlier 43-case lifecycle/cleanup and 786-case full handler race receipts remain supplemental evidence for their stated source checkpoints. Documentation validation passed: 453 decisions and 2,012 specifications, full spec lint, all 63 public-doc validator tests, and 55 published pages (`/tmp/kandev-final-latest-docs.log`). Public coordination documentation already describes successful runtime close, retained composition, recorded elapsed time and conservative recovery; this final filename/fixture correction requires no additional public copy. The final normal-hook/push receipt is recorded at delivery in the native task plan. No additional production feature work remains in these work orders.

### Evidence and operational limits

Phone screenshots were inspected and preserved as `/tmp/kandev-final-completed-child-phone.png` (SHA-256 `a12ad02caf43e5f5492a747df1842ddb21afc49771e82cb7cc124fa160220318`) and `/tmp/kandev-final-result-duration-phone.png` (`79252ad33fba10b0257ff26e9c56d35dc6270dabe97a347fedea80e74ed70a33`). Long elapsed text, Open and composer fit the phone viewport. A separate global update toast partly covers the first child's header; that artifact is not evidence of a fully unobstructed header. Navigation and continuation have separate executable acceptance.

PostgreSQL cases require `KANDEV_TEST_POSTGRES_DSN` and are explicitly skipped when it is absent. Standalone browser fixtures and dump probes are gated; managed browser runs provide their actual exercised coverage. Browser providers are isolated local fixtures and mock ACP, not universal external-provider certification. Missing/conflicting historical evidence remains held; no live database repair, deletion, deployment or restart was performed. The unrelated untracked `loading-performance-investigation.md` is preserved and excluded. Build/lint caches are cleaned only after owned processes release them; evidence logs and screenshots remain available.


## Terminal identity

Commit `a96adeec38f40a0e0a0621c8ee046847cf9dc731` contains the reviewed 12 Go files over `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. Era Sol accepted both production semantics and direct automatic-notification regression coverage. Focused race checks passed nine top-level tests plus four notification/control tests. The earlier broad selector passed 84 tests with 22 PostgreSQL skips; SQL guard, store conformance and build passed. See the linked implementation and notification handoffs for exact evidence and limitations.

Extended affected-package lint passed with zero issues (`/tmp/kandev-terminal-final-extended-lint.log`). The first normal hook attempt reached zero issues but exceeded its five-minute limit and created no commit. The warm-cache retry passed normally (`/tmp/kandev-terminal-checkpoint-commit-retry.log`).

Hook receipt: pre-commit active; commit-msg active; harness-lint passed; architecture-lint passed; gofmt passed; go-lint passed; public-copy guard passed; commitlint passed. Documentation/specification and frontend-only hooks skipped where no matching staged files existed. Bypass false; commit result pass. The worktree deliberately retains the separately reviewed duration packet and unfinished cleanup regression/docs.

Duration Go files were temporarily preserved in a path-scoped stash during the terminal-only commit, restored with SHA-256 equality for all four files, and the owned stash removed. No source or evidence was discarded.

## Remaining integration

Commit accepted duration separately, merge reviewed recovery checkpoint `f71b9e09d4`, validate combined seams, then implement the already-planned successful-completion cleanup. Upstream main was fetched at `b2376186ed`; recheck its current tip at final merge. Finish affected validation and verify the remote parent-branch SHA after push. No live runtime/database mutation or deployment has been performed.

## Duration and recovery merge

Duration checkpoint `8ed56d94ef6b9a2d99d9b415f4ed1f638141626b` passed normal pre-commit and commit-msg hooks without bypass, including Go/web lint, formatting, i18n, E2E sleep guard, docs/spec/catalog and public-copy checks. Log: `/tmp/kandev-duration-checkpoint-commit.log`. Both parent checkpoints were pushed to `origin/feature/plan-errands-first-d-424` and `git ls-remote` verified that exact SHA. The unfinished cleanup packet was temporarily stashed for the clean checkpoint push, restored with SHA-256 equality, and the owned stash removed.

Recovery merge from `f71b9e09d4` is prepared, with one source conflict resolved in `apps/backend/internal/errands/compose.go`: preserve both `AttentionReason` and `DurationMS` in the immutable item and its producer. Other changes merged automatically; no unresolved paths remain. The merge is not committed pending combined validation and focused integration review. The intentionally RED cleanup test remains outside the index and is not part of the accepted merge.

Combined acceptance passed and Era Sol `523eba58` found no new integration findings. Parent verified all nine receipt SHA-256 values from [combined acceptance](combined-acceptance-handoff.md) and all seven reviewed source Git blobs before committing. Results: 128 primary and 13 extra top-level backend race passes; 24 PostgreSQL cases/subcases skipped without a DSN and one standalone environment-gated browser fixture skipped; SQL guard and race store conformance passed; web typecheck and 162 Vitest tests passed; fresh managed desktop 3/3 and source-stable phone 3/3 passed. The strict codec overlay checks both optional metadata fields together. These checks exclude the deliberately unimplemented cleanup RED cases and do not certify external providers or live incident repair.

Recovery integration committed as `67b6b9ba9d105d16aacac7fa3c1c297783e129d8` over parent `8ed56d94ef` and merged parent `f71b9e09d4`. Normal merge hooks were active and passed: harness, architecture, gofmt, changed-package Go lint, public-copy and commitlint; other hooks skipped where the merge-conflict file scope had no matches. Bypass false. Receipt: `/tmp/kandev-recovery-integration-merge-commit.log`. Push to the parent feature branch succeeded and remote SHA matched. The unfinished cleanup packet remains uncommitted. A separately authored untracked `loading-performance-investigation.md` appeared during integration; it was preserved and excluded from the commit/push. No owned stash remains.

## Direct primary cleanup checkpoint, 2026-11-12

The primary completed cleanup checkpoint `4becbc47a1f38d869f5e8b90bce44cc019a9bd6b`, parent `67b6b9ba9d105d16aacac7fa3c1c297783e129d8`. Both commit hooks were active and bypass was false. Harness, architecture, gofmt, scoped Go lint, Prettier, web lint, i18n, E2E sleep guard, public copy guard and commitlint passed; documentation-catalog and specification hooks skipped their unselected paths, with standalone catalog/spec checks passed. The unrelated loading-performance note stayed untracked. This is a local checkpoint; final merged browser/race acceptance and delivery remain pending.

Fetched upstream main is `dab0d02e4571c147adf8b9c193db595dbafdd190`. Its 17 conflicts are resolved and the merged backend builds. Final managed acceptance uses one shared build lock, two Go workers and a 3 GiB soft Go memory target. Inlining is disabled only for the large orchestrator test package; a separate optimized production build validates normal optimization. No live instance or database was changed. Final acceptance and push receipts will replace this pending state below.

## Final upstream integration acceptance, 2026-11-12

The upstream merge preserves exact parent terminal repair, immutable historical elapsed time, recovery attention/refusal and explicit continuation, and durable successful runtime cleanup. Integration also corrected cancellation re-entry during terminal convergence, session recovery publication ordering, Send Now claim-write failure fallthrough, and retention of unbound opaque ordinary-session payloads without granting turn authority. Runtime admission fixtures use the existing executor actor helper; prompt/signature fixtures match upstream contracts. No production safety gate was removed.

Backend race acceptance in `/tmp/kandev-upstream-backend-acceptance.log` passed the full Errands, models, status-summary, system-prompt and MCP-profile suites, then the affected orchestrator, SQLite, executor, task-service, agentctl API and lifecycle selectors. The verbose selector recorded 875 top-level passes and 98 top-level skips; the earlier five package suites were not verbose and are not included in that count. Skips include PostgreSQL without a DSN and environment-gated browser/storage cases. SQL guard and race store conformance passed. Scoped lint passed with zero issues across 93 changed Go packages (`/tmp/kandev-upstream-lint.log`); its first cold package-loading attempt timed out, then the extended retry passed.

Web acceptance passed typecheck, 418 tests, i18n checks and the E2E sleep guard (`/tmp/kandev-upstream-web-acceptance.log`). The first real browser run exposed completed-child follow-up rejection introduced by upstream's general terminal-session policy. Frontend and WebSocket admission now permit a completed Errand explicitly, with exact-settlement refusal still authoritative; ordinary completed, failed and cancelled sessions remain blocked. Frontend RED/GREEN evidence is `/tmp/kandev-completed-errand-submit-{red,green}.log` (119 affected tests pass). Backend admission RED evidence is `/tmp/kandev-completed-errand-handler-red.log`. Final backend admission, browser, optimized-build and delivery receipts follow after completion; none is inferred from the earlier browser results.

No live runtime/database repair or deployment was performed. The unrelated untracked `loading-performance-investigation.md` is excluded. Heavy builds use the shared lock and bounded resources; cache cleanup removes only unused scratch or older cache entries between builds.

## User-requested delivery checkpoint

The user requested committing and pushing the implemented work before final verification, then merging the newer upstream main and fixing any integration failures. This checkpoint completes the existing `dab0d02e4571c147adf8b9c193db595dbafdd190` merge and preserves the current source; it does not claim final browser acceptance. The newer fetched upstream snapshot is `9b2b8e98e2b7a510560c65ac572d412fa3394eb0`.

The full handler race suite passed 786 top-level tests with no skips (`/tmp/kandev-completed-errand-handlers-final.log`), using `SHELL=/bin/bash` to avoid this host's zsh auto-tmux startup in the unrelated authentication-terminal test. Frontend typecheck and 175 tests passed (`/tmp/kandev-final-web-integration.log`). Browser admission now preserves the child target, but the follow-up exposed a separate missing cold-resume permission. The latest implementation grants that permission only for a task-bound explicit manual request after exact Errand continuation validation; passive/foreign-task/ordinary/failed/cancelled/unsettled controls are included. Its race run is `/tmp/kandev-completed-errand-cold-resume2.log`; collect its terminal result and rerun browser acceptance after the checkpoint. Earlier failed browser runs remain RED evidence, not final acceptance.

Final upstream integration, full desktop/phone Errand checks, affected native-recovery/storage checks, optimized build and final delivery records remain required. The unrelated loading-performance investigation stays untracked and excluded.

## Checkpoint delivery and latest upstream

Checkpoint `0067adc210e28ee242f507a211c4a7ca6b96d508` was pushed to `origin/feature/plan-errands-first-d-424` and its remote SHA was verified. Its parents are cleanup `4becbc47a1` and upstream `dab0d02e45`. Both normal hooks were active and bypass was false. The first hook attempt reported zero Go lint issues but exceeded five minutes; the warm-cache retry passed all applicable hooks, including commitlint (`/tmp/kandev-errands-before-refresh-commit-retry.log`). The latest completed-authority race selector passed 15 top-level tests (`/tmp/kandev-completed-errand-cold-resume2.log`). Browser follow-up acceptance remains pending.

The subsequent upstream fetch returned `37f081a976526de1a6967f8714e72e247ec0b386`: five additional commits covering repeated native Resume, recovery-block migration and quarantine triggers, terminal keepalive/render lifetime and PR conflict badges. The merge applied cleanly, preserving the Errand fixture freshness entry and both migration sets. Per the user's requested order, this updated branch is checkpointed before final verification. Acceptance of the combined source, any necessary fixes and final delivery records remain open. No live runtime/database changes were made.

## Post-refresh lifecycle corrections

Real desktop follow-up delivered the expected provider response, but cleanup remained waiting across three 20-second sweeps. The saved backend log `/tmp/kandev-followup-close-lifecycle-backend.log` reports `latest errand turn has only lifecycle authority`. Teardown selected the newest history row after completion, so a later synthetic resume turn shadowed genuine settled work. It now uses the repository's existing canonical conversational-turn query, which excludes lifecycle bookkeeping and ranks live work first, following ADR-2026-08-14-current-turn-clarification-ownership. No schema or public API was added.

A second regression proved explicit continuation stopped before executor admission when its old runtime row had been removed. Existing exact-authorized completed-prompt options are now computed before that guard; only that permission permits continuing without the row. Retained-row and passive-view controls remain unchanged. The lower executor still owns authenticated admission, restoration policy and stale-execution checks.

RED: `/tmp/kandev-completion-lifecycle-and-absent-row-red.log` reproduced both ready/process-exit lifecycle-shadow failures and the removed-row continuation failure; retained-row and passive controls passed. GREEN: `/tmp/kandev-completion-lifecycle-and-absent-row-green.log` passed 43 top-level cleanup, cancellation, continuation and cold-resume race tests. The browser helper excludes synthetic lifecycle-only rows while still requiring exactly one completed application turn, no prompt errors, preserved history and completed cleanup. Its bounded closure wait covers the existing sweep cadence; increasing that wait alone did not fix the reproduced defect.

Per the user's early-delivery instruction, these corrections are checkpointed before the remaining full browser/storage acceptance. Resume the managed acceptance driver with a fresh build; final integration is not yet accepted. Between build phases, 8.04 GiB of cache entries unused for more than two hours were removed with no compiler/linter active, restoring about 23 GiB of headroom.
