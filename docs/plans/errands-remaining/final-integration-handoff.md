# Errands delivery integration

The verified production source is `ee0f077cb9d8061cf17437021f087326978aed05`, integrating upstream main `585824a5d066d22785814c0394b88dd57f6c5ded`. The working branch is `feature/correct-errand-statu-9e4`; delivery targets `origin/feature/plan-errands-first-d-424`. Checkpoint `dcc3de55a2` preserves history loading, exact late-tool replay and the shared-summary publication correction. Merge `4f39f99b7b` integrated the first eighteen upstream commits; `ee0f077cb9` adds the two subsequent native-recovery and SSH-cleanup fixes. Both merge commits passed normal hooks with bypass disabled.

## Integration decisions

- Use upstream's shared `AgentDeliveryPromptTurnBinding` model while retaining the optional native-session identity needed by exact interrupted-Errand proof. Existing serialized bindings retain their meaning.
- Preserve upstream's typed recovery refusals. Exact retained interrupted-Errand evidence can authorize its initialized native successor across generations; unsupported successors remain refused. Ordinary recovery does not gain Errand authority.
- Preserve upstream's already-committed MCP attachment and connection-scoped startup handling. Conflicting prompt/submission ownership is rejected before entering the durable writer and does not masquerade as a storage failure or cancel a successor.
- Retain the automatically merged startup, settlement, ordinary-session and web-store behavior, including upstream's removal of the Azure DevOps slice. The final two upstream commits merged without conflicts; their recovery, journal and SSH boundaries passed the focused integration checks.

## Verification

The final upstream delta passed **605 top-level Go tests across eight packages**, with three expected opt-in browser-fixture skips and no failures: transport 114, lifecycle 129, orchestrator 291, repository 59 and persistence conformance 12. Repository and conformance checks exercised SQLite and disposable PostgreSQL 16; SQL guard passed. The driver used `-race -trimpath -p 1 -parallel 1 -count=1`, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB` and the shared build lock. PostgreSQL stopped cleanly afterward. Commands and counts are retained in `/tmp/kandev-latest-delta-verify.sh` and `/tmp/kandev-latest-delta-receipts.json`.

The preceding merge passed 570 top-level Go tests and 459 Vitest tests in 31 files, plus typecheck and localization checks. The final two upstream commits change no frontend source, so those frontend checks were retained rather than repeated. All 58 previously affected Go packages passed lint; all eight packages affected by the final upstream delta then passed with zero issues. Exact commands and outcomes are in `/tmp/kandev-final-integration-lint-groups.json` and `/tmp/kandev-latest-delta-lint-groups.json`. The earlier aggregate lint timeout and its corrected duplicated literal remain recorded in `/tmp/kandev-final-integration-lint.log`.

Fresh managed builds passed main's ordinary smoke **1/1**, candidate desktop **3/3**, and candidate phone **3/3**, without retries. Candidate cases cover ordinary navigation, demand-only history and release of full child caches after repeated Open/Back. The final rebuilt fixture also passed **20 focused admission/replay cases**, including one exact parent result after the late-tool event. These binary checks supplement the race runs; they are not an additional race claim. The final matching-main performance cohort is recorded separately in the [comparison handoff](../errand-history-loading/summary-followup-handoff.md).

Normal pre-commit and commit-message hooks were active. The environment supplied a nonexistent macOS hooks path, so each commit explicitly used the existing repository hooks at `/home/luancm/projects/kandev/.git/hooks` without changing shared configuration. The checkpoint's first hook attempt exposed a test-helper label to the copy ratchet; the fixture now uses its supplied session identity, its regression passed, and the retry passed all applicable hooks. Both failed and successful receipts remain `/tmp/kandev-delivery-checkpoint-commit.log` and `/tmp/kandev-delivery-checkpoint-commit-retry.log`. Merge hooks correctly skip unchanged/non-conflicting categories; the separate affected-package checks above provide the broader evidence.

## Delivery boundary

Implementation and integration are committed. The final performance cohort passed 28/28 with unchanged source/artifacts. All latency measures stay within their warning boundaries; the fixed heap warnings and earlier strict-limit exceptions are explicitly dispositioned in the comparison handoff. The final documentation commit precedes the authorized push; the remote SHA receipt belongs in the current Kandev task plan. No deployment, live database mutation, historical-session cancellation or automatic live repair is claimed. Unknown historical records remain held when exact evidence is absent.

The [bounded-use roadmap](bounded-use-roadmap.md) separates this delivery from a later real-provider pilot. The [history plan](../errand-history-loading/plan.md) and [exact-tool replay evidence](../errand-tool-terminal-replay/implementation-handoff.md) retain the detailed contracts and earlier regression evidence. Local mock-provider browsers, emulated phone layouts and SQLite/PostgreSQL tests do not certify every provider, executor, Mac or physical phone.

## Final integration receipts

| Receipt                                     | SHA-256                                                            |
| ------------------------------------------- | ------------------------------------------------------------------ |
| `/tmp/kandev-latest-delta-transport.log`    | `65205ee4cc3ffc5b435f5da17c1121171985195f1d423cac239c40a7d674b642` |
| `/tmp/kandev-latest-delta-lifecycle.log`    | `6b74b47da43704aa8ba92fc05d147703ef9d40e343ce1b2e7bb030b56b46f5c0` |
| `/tmp/kandev-latest-delta-orchestrator.log` | `826445db196a3db8ba10f1f982b7a8c386f12953ccd9c6fb4982b7fbee8a61f0` |
| `/tmp/kandev-latest-delta-repository.log`   | `11abb5a608f22864e9943ecf2e7b45ddb739ab3d2ef90188898dec2ea3925c44` |
| `/tmp/kandev-latest-delta-conformance.log`  | `f95f87a63c9c886e67a38870db2edb87280d338fabe560a73ea67accea354cce` |
| `/tmp/kandev-latest-delta-sqlguard.log`     | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-latest-merge-commit.log`       | `0d5d675c5ee1121ee74c928b22d39451dd31f8aba5677e6f5cb3f17f62724bfb` |
| `/tmp/kandev-latest-main-build.log`         | `1ce81302a36dbaf30b9273b24a52f704c8ac9a5fad87c58a4de4fe31563aa960` |
| `/tmp/kandev-latest-candidate-build.log`    | `a92b747ec5814cdb3170149e15034a590e85898d189b23320650ab7130e6b42d` |
| `/tmp/kandev-latest-admission-binary.log`   | `b8bb0000bc4b2cf240b86be7ae9e45e16af881a89914c6a6ef4167137b9f3b4c` |
| `/tmp/kandev-latest-candidate-phone.log`    | `8a3bcb6144478de344e44ed0bce8410b2abaaaf131edf6c91e4363a4993bc2fe` |

The raw browser logs are `/tmp/kandev-run.e2e.kvY7mTEt.log` (main), `/tmp/kandev-run.e2e.zaadeMSx.log` (candidate desktop) and `/tmp/kandev-run.e2e.uqXZqqQ8.log` (candidate phone). Build and test processes exited before the timed cohort. Private-cache cleanup ran under the shared lock only when no Go process was active; the final prebuild cleanup recovered 8.86 GiB and left 18 GiB free. Its exact entries and byte count are recorded in `/tmp/kandev-latest-prebuild-cache-cleanup.json`. Raw evidence was preserved and no cache cleanup runs during the timed comparison.
