# Combined recovery, notification and duration acceptance

Scope: read-only combined validation of the staged recovery integration at `HEAD=8ed56d94ef6b` and `MERGE_HEAD=f71b9e09d444`. The recovery merge remains uncommitted; Git ownership stays with the parent. This handoff owns no production or permanent test changes.

## Result

The integrated recovery, parent notification and historical duration checks passed. The fresh managed desktop run passed all three selected browser cases; the phone run reused those artifacts after source stability was established and passed its three selected cases. The shared Errands delivery item retains both attention reason and duration through the strict Send Now metadata codec.

The accepted RED cleanup test was not selected or counted. This does not accept successful-completion cleanup or complete the broader Errands plan. No live database, runtime or external provider was changed. No Git index or ref was changed, and no commit, push or deployment was performed.

## Backend and persistence evidence

The requested combined race command passed all five packages: lifecycle 8.520s, SQLite 27.360s, orchestrator 12.834s, Errands 1.413s and backendapp 2.480s. It reported 128 top-level passing tests, 24 PostgreSQL skipped cases/subcases because `KANDEV_TEST_POSTGRES_DSN` is unset, and one env-gated `TestErrandRecoveryBrowserFixture` skip outside the managed browser run. Every package matched selected tests; there were no `no tests to run` warnings. The separate exact-selector run passed 13 additional top-level tests covering recovery observer publication/wake behavior, error and isolation controls, attention projection/precedence, SQLite projection parity and the delivery wire/strict codec paths.

The required `DeliveryDuration` cases passed for both parent-turn and notification routes, zero/invalid timestamp handling, prepared cancellation metadata and Send Now decoding. The known `TestSendNowErrandCanonicalPersistenceFailurePreventsProviderIO` baseline did not match the requested selector and did not appear in the run; it was not reproduced or waived. No `TestErrandCompletionCleanup` RED case appeared in the run.

The merge conflict resolution is present in [`compose.go`](../../../apps/backend/internal/errands/compose.go#L35): `DeliveryItem` contains `AttentionReason` and `DurationMS`, and its production constructor assigns both at lines 304–310. An overlay smoke test added both fields to the same metadata item, passed it through `DecodeSendNowDelivery`'s strict decoder, then checked both fields after JSON re-encoding. It passed without changing any repository source or test file.

SQL guard passed with no output. Store conformance passed under race in 74.367s. The fresh managed E2E desktop run also built the integrated backend binary, Vite assets and fixture plugin package successfully.

Exact primary race command:

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator ./internal/errands ./internal/backendapp -run 'TestParentTerminalIdentity|Errand.*(Recovery|Continuation|Attention|Owner|Ownership|Admission|Terminal)|RetainedTerminal|Durable.*Identity|QueuedDeliveryPredecessor|DeliveryDuration' -count=1 -v)
```

Exact additional-selector command:

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator ./internal/errands -run '^(TestDeliveryRecoveryPhaseChangePublishesAttentionAndWakesAwait|TestRecoveryPublicationFailureDoesNotNotifyOrWakeErrandAwait|TestOrdinarySessionRecoveryDoesNotNotifyOrWakeErrandAwait|TestRejectedStaleRecoveryDoesNotNotifyOrWakeErrandAwait|TestClearedRecoveryNoticePublishesAttentionAndWakesAwait|TestNoOpRecoveryClearDoesNotNotifyOrWakeErrandAwait|TestRecoveryAttentionProjection|TestRecoveryAttentionReasonInParentBlock|TestRecoveryAttentionPrecedence|TestRecoveryAttentionOutstandingWithoutAwaitingFlag|TestRecoveryAttentionSQLiteProjectionParity|TestDeliverySnapshotItemsUseSnakeCaseWireFields|TestSendNowDeliveryRoundTrip)$' -count=1 -v)
```

The combined-field codec smoke used a temporary Go overlay at `/tmp/kandev-errands-combined-acceptance-20261111/overlay.json`; its exact selected test was `TestDeliveryDurationSendNowCodecPreservesMeasuredZeroAndAbsentPreparedItems` in `./internal/errands`. The temporary test copy asserted `recovery_unresolved` and `duration_ms=134000` together after strict decode and JSON reload.

Persistence commands, both run under the same lock and resource limits:

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp go run -trimpath -p 1 ./cmd/sqlguard ./internal)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp go test -race -trimpath -p 1 -parallel 1 ./internal/persistence/storeconformance -count=1)
```

The compact initial Go invocation using `-p1 -parallel1` returned `no Go files` before test discovery. The explicit `-p 1 -parallel 1` command above is the corrected run and passed; the initial invocation is not test evidence.

## Web and browser evidence

`pnpm run typecheck` passed. The focused recovery and duration Vitest run passed 162 tests across nine files: the seven accepted recovery files plus `errand-parse.test.ts` and `errand-delivery-message.test.tsx`.

The final standard managed Chromium run kept repository global setup enabled, used one shard/worker, rebuilt the backend and Vite assets, and passed 3/3: desktop real recovery refusal, exact-settled explicit continuation, and finished-result duration. It inherited `MAKEFLAGS='GOFLAGS=-v\ -p=1'`; a `make -n build-kandev` check showed the backend command as `go build -trimpath -v -p=1`. This bounded rerun supersedes the earlier fresh managed pass, whose backend Makefile build had not received the explicit `-p=1` limit. The standard managed `mobile-chrome` run also kept global setup enabled and passed 3/3: phone recovery refusal, exact-settled explicit continuation, and finished-result duration. The phone run used `--no-build` only after the final fresh desktop build/run and matching source manifest.

Desktop command:

```bash
(cd apps/web && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp 'MAKEFLAGS=GOFLAGS=-v\ -p=1' corepack pnpm@9.15.9 e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts e2e/tests/session/errand-result-duration.spec.ts -- --grep 'desktop real Errand recovery refusal|desktop explicit Errand continuation|finished result duration' --retries=0)
```

Phone command:

```bash
(cd apps/web && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOTMPDIR=/tmp/kandev-errands-combined-acceptance-20261111/gotmp 'MAKEFLAGS=GOFLAGS=-v\ -p=1' corepack pnpm@9.15.9 e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts e2e/tests/session/mobile-errand-result-duration.spec.ts -- --grep 'phone real Errand recovery refusal|phone explicit Errand continuation|finished result duration' --retries=0)
```

The final source manifest covered 22,190 files under `apps/backend`, `apps/web` and `apps/packages`, excluding dependency/build/cache/output directories, `apps/backend/bin`, and generated `apps/web/generated/release-notes.json`. Its content SHA-256 was `ca07569cd501f045bcf1a0fbeb6f0365b6bd9772ac633feb801da9a805bcff7c` before and after the fresh desktop run and before and after the phone run; each pair of full manifest files compares byte-for-byte equal. The four manifest files are `/tmp/kandev-errands-combined-desktop-bounded-before-20261111.sha256`, `/tmp/kandev-errands-combined-desktop-bounded-after-20261111.sha256`, `/tmp/kandev-errands-combined-phone-bounded-before-20261111.sha256` and `/tmp/kandev-errands-combined-phone-bounded-after-20261111.sha256`.

Browser cases run through the isolated Kandev E2E backend, WebSocket path and local mock/ACP provider fixture. They prove the integrated backend and UI flow with controlled provider behavior; they do not claim a third-party provider integration or live production recovery outcome. PostgreSQL-backed runtime cases were skipped because the DSN was unset.

## Receipts and cleanup

All explicit Go tests and persistence checks used `/tmp/kandev-errands-repair-build.lock`, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `GOTMPDIR` in the owned scratch directory, `-trimpath`, `-p 1` and `-parallel 1`. The final managed backend build passed the same lock and environment limits and inherited `MAKEFLAGS='GOFLAGS=-v\ -p=1'`; Playwright ran one shard/worker. Logs and their SHA-256 values:

| Evidence | Log | SHA-256 |
| --- | --- | --- |
| Combined backend race selectors | `/tmp/kandev-errands-combined-race-main-20261111.log` | `7bf2dd40b8f1f546abdbb3fc46a8b99ed05c05303311cdecdb4cf7472df1a206` |
| Additional exact selectors | `/tmp/kandev-errands-combined-race-extras-20261111.log` | `877d917f18260b9f54720bb98305d611df5e05fea32dfee92a0d04f038e7a557` |
| Combined strict-codec overlay smoke | `/tmp/kandev-errands-combined-codec-smoke-20261111.log` | `b5aed525e724466fd1a14267e3ab0d7ca6e503e38b465b6b7ea5f65a13fc7924` |
| Web typecheck | `/tmp/kandev-errands-combined-web-typecheck-20261111.log` | `20a2df4ce2388711bcf679aeb7f0e564594556e42e3d1d2c6fa45a48923e727f` |
| Focused web Vitest | `/tmp/kandev-errands-combined-web-vitest-20261111.log` | `b753ab10587d5e23ecc350f2b089d618d7f7d3a987494caff9679d6e7704c012` |
| SQL guard | `/tmp/kandev-errands-combined-sqlguard-20261111.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Store conformance | `/tmp/kandev-errands-combined-storeconformance-20261111.log` | `080d043b531bdc737d2637c4875cffd032a8934a78d82b5c2372c8cd53417993` |
| Final bounded managed desktop E2E | `/tmp/kandev-errands-combined-desktop-bounded-20261111.log` | `3726eab49b0dbdb727c3273a78dc6c77116377fe92670af816c6768c38a4d34f` |
| Final bounded managed phone E2E | `/tmp/kandev-errands-combined-phone-bounded-20261111.log` | `c50866b866520803bb54bfb98b0d16910899817b50064b66fe7e25b8732f6059` |

The test/build processes terminated and the shared lock was released. Disk had 17 GiB available after the browser runs. Owned temporary Go build and codec-overlay scratch were removed; logs and source manifests remain in `/tmp` as evidence. No repository source/test file was changed by this packet.
