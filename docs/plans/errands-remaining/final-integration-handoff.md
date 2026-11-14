# Errands delivery integration

The primary session is completing the accepted history-loading and result-delivery fixes on `feature/correct-errand-statu-9e4`, for delivery to `origin/feature/plan-errands-first-d-424`. Checkpoint `dcc3de55a2ee2ec9314ef81fd32a4f7feb9768a6` preserves the implementation and earlier evidence. Upstream main `037c37619613dd50c86a639662e3f246ecbb76fa` is the integration and final comparison reference.

## Merge decisions

- Use upstream's shared `AgentDeliveryPromptTurnBinding` model while retaining the optional native-session identity needed by the exact interrupted-Errand proof. Existing serialized bindings keep their meaning.
- Retain upstream's typed recovery refusals. An exact retained interrupted-Errand assessment may still authorize its initialized native successor across generations; an unsupported successor remains refused. Ordinary recovery does not gain Errand authority.
- Preserve upstream's already-committed MCP attachment and connection-scoped startup handling. Conflicting prompt/submission ownership is still rejected before entering the durable writer and does not masquerade as a storage failure or cancel a successor.
- Keep the automatically merged startup, terminal-settlement, ordinary-session and web-store changes. The removed upstream Azure DevOps slice is not restored by the Errands store wiring.

## Verification and delivery state

The checkpoint's normal pre-commit and commit-message hooks passed with bypass disabled. The environment supplied a nonexistent macOS hooks directory; the commit explicitly used the existing repository hooks at `/home/luancm/projects/kandev/.git/hooks`, without changing shared configuration. The first attempt exposed a test-helper label to the copy ratchet; the fixture now uses its supplied session identity, its regression passed, and all hooks passed on retry. Receipts: `/tmp/kandev-delivery-checkpoint-commit.log`, `/tmp/kandev-checkpoint-test-helper.log`, and `/tmp/kandev-delivery-checkpoint-commit-retry.log`.

Affected merge verification passed 570 top-level Go tests across ten packages, with three opt-in browser fixtures skipped by the unit invocation. The twelve persistence-conformance cases include SQLite/PostgreSQL v0.93.0 upgrades; SQL guard passed. Frontend verification passed 459 tests in 31 files, typecheck, and localization checks. Checks used the shared build lock and disposable PostgreSQL 16, which was stopped cleanly afterward. All 58 changed Go packages passed lint in six sequential groups against upstream `037c376196`. The initial all-package pass reached its fifteen-minute limit and reported one duplicated parent-source literal; a value-preserving named constant resolved it. The failed receipt remains `/tmp/kandev-final-integration-lint.log`; successful group commands, logs and exit codes are recorded in `/tmp/kandev-final-integration-lint-groups.json`. The final rebuilt fixture will also rerun the focused admission cases after that constant extraction. The final ordinary performance comparison will use matching main and merged-candidate builds, preserving the earlier measurements and warnings. The merge is not yet committed or pushed, and no deployment or live-data repair is claimed.

The [bounded-use roadmap](bounded-use-roadmap.md) separates the current delivery from the subsequent real-provider pilot. The [history plan](../errand-history-loading/plan.md) and [exact-tool replay evidence](../errand-tool-terminal-replay/implementation-handoff.md) retain the detailed implementation contracts and earlier checks.

## Integration receipts

Go checks used `-race -trimpath -p 1 -parallel 1 -count=1`, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, and the disposable loopback PostgreSQL DSN. The sequential driver is `/tmp/kandev-final-integration-verify.sh`; it records each package group separately and stops the disposable server on exit. The web driver is `/tmp/kandev-final-web-checks.sh`.

| Check | Receipt SHA-256 |
| --- | --- |
| `/tmp/kandev-final-integration-agentctl.log` | `a93a5fee5515e26e27865319f9d5cd72724bbbe9ab11cb4bb45e2929cea73b72` |
| `/tmp/kandev-final-integration-runtime.log` | `6a58171561ee19e19051613a5d755798b493677ee7b3d22ef4e8ddfc761e9bbd` |
| `/tmp/kandev-final-integration-reads.log` | `3087b227a80da38c8702678924619af019236df1a319cfa3ef0ce2745f93cd35` |
| `/tmp/kandev-final-integration-conformance.log` | `a01675f7c690ff5c413bd3993e129bd12c0d882063f61b9774bb59c226e76181` |
| `/tmp/kandev-final-integration-sqlguard.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-final-integration-web.log` | `47e233549b2cfb4564eb6c527e62b6e0fdcb49aebadb04b9a04af3c990de7eab` |
| `/tmp/kandev-final-integration-typecheck.log` | `f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708` |
| `/tmp/kandev-final-integration-i18n.log` | `fffd1b3d52b7e59a7e815a67784a67ed5cdf8f21deddc46a78520febdab69ecf` |

After tests, 3.84 GiB of private Go-cache entries older than two hours and not read in the previous thirty minutes were removed under the shared lock, with no Go process active. Raw evidence was retained. The cleanup audit is `/tmp/kandev-merge-cache-cleanup.json`.
