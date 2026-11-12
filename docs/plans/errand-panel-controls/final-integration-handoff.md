# Parent integration and final verification

## Current state

Final local acceptance passed for the combined cleanup and panel-control source. Backend race checks passed 77 tests, scoped Go lint passed with zero issues, and fresh managed browser runs passed 19 desktop and 16 phone cases without failures or skips. Normal-hook commit and push remain the final delivery step.

## Review corrections

Backend review 60c20996 found that category-only diagnostic keys could suppress changed admission evidence. The parent replaced those keys with a bounded SHA-256 fingerprint of the target, expected parent, actual turn identity and admission marker. Raw identity values are not logged. Both SQLite regressions failed with the old implementation, then passed with fingerprinting. The initial broad rerun also exposed an overbroad new test expectation: a different observed submission supersedes a reserved target before admission diagnostics. The corrected test preserves that ownership guard, checking submission changes during legacy adoption and sender/state changes while a reserved target stays pending. No production supersession or stop guard changed. Era Sol 65d34efc reviewed the fingerprint and phone-test corrections without concrete findings; it did not certify runtime checks.

The first browser verifier passed 19 desktop cases and 14 of 16 phone cases. Its two failures were test defects: the ordering helper counted row wrappers as rows, and geometry was sampled during drawer entrance animation. The parent excluded action wrappers from the row locator and waits for existing finite animations before measuring. No frontend production changes were needed for those corrections. Original evidence remains in [combined-browser-handoff.md](combined-browser-handoff.md).

## Completed checks

- Final backend race selector: 77 top-level tests passed, zero failures or skips. Command: `source /tmp/kandev-startup-repair-env.sh; flock -x /tmp/kandev-errands-repair-build.lock go -C apps/backend test -race -trimpath -p 1 -parallel 1 ./internal/errands ./internal/orchestrator -run 'TestErrand.*(Completion|Teardown|Legacy|Sweep|Cancel|Admission)' -count=1 -json`. Receipt: `/tmp/kandev-cleanup-fingerprint-green-final.jsonl`.
- Original fingerprint RED: `/tmp/kandev-cleanup-fingerprint-red.jsonl`. First broad rerun with the invalid test expectation: `/tmp/kandev-cleanup-fingerprint-green.jsonl`; this is failure evidence, not a GREEN receipt.
- Documentation catalog: 453 decisions and 2012 specifications validated. All specification lint passed.
- Earlier frontend component/hook suite: 34 tests passed; implementation and remediation handoffs record typecheck, web lint, locale checks and production build. The parent changed only the two E2E files afterward.

- Scoped Go lint: the first run exceeded its five-minute package-loading limit (`/tmp/kandev-cleanup-final-lint.log`). The unchanged-script retry passed with `0 issues` (`/tmp/kandev-cleanup-final-lint-warm.log`). All seven changed Go files were supplied to `scripts/lint-go-changed`.
- E2E sleep ratchet and Prettier checks passed for both parent-edited browser files.

- Fresh managed desktop: 19/19 passed, zero failures/skips, with rebuilt backend, fixtures and production web assets. Log: `/tmp/kandev-errands-final-parent/logs/kandev-run.e2e.T3vziioe.log`. All 22 frozen source hashes still match. The parent inspected `errands-desktop-panel-stop.png`; both row actions fit the panel.

## Final browser acceptance

Phone acceptance passed 16/16 with zero failures/skips. Both earlier failures passed: finite-animation synchronization establishes settled drawer geometry across 320, 393, 767 and 768px, and the corrected row selector verifies exact ordering through settlement/reload. The parent inspected the new 320px screenshot: running, queued and attention rows, separate Stop targets and the Stop-all footer fit the drawer. Desktop Stop and phone Stop both exercise real cancellation against the integrated cleanup backend, retain transcript IDs and leave sibling status intact.

Both commands used the standard managed build/global setup, host isolation, one shard and one browser worker. No `--no-build`, live database mutation or live runtime restart was used:

```sh
source /tmp/kandev-startup-repair-env.sh
flock -w 300 -x /tmp/kandev-errands-repair-build.lock env KANDEV_RUN_QUIET_DIR=/tmp/kandev-errands-final-parent/logs scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project=chromium tests/session/errands.spec.ts -- --output=/tmp/kandev-errands-final-parent/desktop-results
flock -w 300 -x /tmp/kandev-errands-repair-build.lock env KANDEV_RUN_QUIET_DIR=/tmp/kandev-errands-final-parent/logs scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project=mobile-chrome tests/session/mobile-errands.spec.ts -- --output=/tmp/kandev-errands-final-parent/phone-results
```

Desktop log: `/tmp/kandev-errands-final-parent/logs/kandev-run.e2e.T3vziioe.log`. Phone log: `/tmp/kandev-errands-final-parent/logs/kandev-run.e2e.QJGScGzQ.log`. Screenshots and their hashes remain under `/tmp/kandev-errands-final-parent`; all 22 frozen source paths match after both runs. The manifest below covers the exact accepted source. Build artifacts include the backend application, isolated delivery fixtures and production web assets.

## Limits and follow-up

Provider behavior used isolated local mock fixtures; this is not live Mac or PostgreSQL validation. No persistence schema changed. The verification Errand's missing result delivery remains a [deferred incident](../errands-remaining/verification-result-delivery-incident.md), not a resolved cause. Separately owned performance planning is excluded from this delivery. Both managed runs exited and released their lock; retained logs/screenshots are evidence, and private caches can be removed after commit hooks finish.

## Accepted source hashes

```text
bd6cc151131dc7c8ee54c680b89bcd0073dbbf7e0e05ec6f5e3d1b65c85556d1  apps/backend/internal/errands/service.go
a07869e0068b7e537da5668774d3385a1e61f90e9e814606e2a4195df99a67b0  apps/backend/internal/errands/teardown.go
00a2db0a2b3669b9560faecf738348398429bbdb98dfe94a1ce3c2d0037f4e48  apps/backend/internal/errands/teardown_compatibility_test.go
dc83d4ee451b9807b3cec844b3e8ec1f03bb6f1d2caba1a6499c2523b3c0d0bf  apps/backend/internal/orchestrator/errand_completion_cleanup.go
0cf865f1f51d0c62b8851149fea31e881831e51ec719b4f0d0c470cc3d7b2c0b  apps/backend/internal/orchestrator/errand_teardown.go
d801e591c66a3cb3db3ed6ff6fd9c9eddab538be33f9f2119de3a777fd3bfd01  apps/backend/internal/orchestrator/errand_teardown_integration_test.go
875d7e0dc9c8095c77e6b8baf9b70638d20c72d0e7d65fab89e84d6413e6be9c  apps/backend/internal/orchestrator/errand_terminal_compatibility_test.go
8e80e1855d9834972c399482974f73491c30252a0f373d0c92466b07a671760c  apps/web/components/task/chat/errands/errands-chip.test.tsx
000385d2d045389bb0c439665dc8129c41ad38a40fc54fbde3303837f84fb496  apps/web/components/task/chat/errands/errands-chip.tsx
5bbb53bdf547dbc79f895530eec90fc8260e46f5f7fdfff263ae277a59acb647  apps/web/components/task/chat/errands/errands-panel-row.test.tsx
77cb9b03861581a83785daf54972404b6da31a806cad23f97c9875bf43cc71a3  apps/web/components/task/chat/errands/errands-panel-row.tsx
23ef87c0ccc14c027240ec2d3c8cf66db462926dc3c94d75ee974fd2063fe065  apps/web/e2e/helpers/errands.ts
a63297dbf8ffcdeccb585d979f7e39ecb4c7eeb76d3017f32c8c8c08c5d4857e  apps/web/e2e/tests/session/errands.spec.ts
e312193f138ec05c0791ba1d7e21eef967d4b1e4dd565e1fb2ff5687498d6ca5  apps/web/e2e/tests/session/mobile-errands.spec.ts
a7b046f023c7cb8e37e0c08923839ae4b23dd0ed5046fdc9cab3328f787acdae  apps/web/src/locales/en/task.json
61b84174f916966cd1730aaefff9ec32bd19551139d72da4fc955cc24b705e1f  apps/web/src/locales/ja/task.json
eb2e6bf967deb8e351535b35e19a8d9a88c180a8326d12db14f912b773ce1e45  apps/web/src/locales/ko/task.json
b1d56f5260a9ea3bd05b4f16ddd43392dd8687a8da499bdeb7b9d171bd31f96a  apps/web/src/locales/pseudo/task.json
496e0233218e7aabee29f2aff5d98b36771f6f98a6142de545fdcbaf4068cd44  apps/web/src/locales/pt-pt/task.json
b549d145f85c4396343b0596ebc68015ad3419080b05744fb6445790de2b515b  apps/web/src/locales/zh-cn/task.json
5107685ba7c0a0e575877d450db8ae2a92d8a99572517052fe19b645b51db636  apps/web/src/locales/zh-hk/task.json
62ca4bbaca39466c36bda0a1fdd6fc7870ea922ba236ba4bfbe7ec95daaa4d5d  apps/web/src/locales/zh-tw/task.json
```
