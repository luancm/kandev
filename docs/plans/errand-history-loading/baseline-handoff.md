# Errand finished-history baseline

This baseline was measured before production changes at source HEAD `6e4c5ca9bce14b952d8cbaa52dc0282af870d835`; Task00 introduced only an isolated backend test fixture, E2E setup freshness wiring, a shared browser measurement helper, and desktop/phone E2E specs.

## Harness and fixtures

The fixture creates one mixed task with three ordinary sessions, one terminal-pending result, one recovery-attention control, and 0, 100, or 1000 finished delivered children, plus a separate ordinary control task with three sessions and no Errand rows; both controls remain fixed as history size changes.

Finished child identities and result payloads are inserted through test-only repository code into the isolated managed E2E database, each child has a fixed payload of five objective sentences and nineteen result sentences, and the fixture launches zero agent processes.

The desktop and phone specs record boot and reload response counts and bytes, WebSocket callers and frame bytes, returned Errand row counts, task-session rows materialized in the browser store, broad CDP renderer duration deltas, usable-parent timing measured by filling and reading back a real composer input, five child open/Back loops where children exist, subscription state, and forced-GC browser heap.

Desktop runs also seed a saved maximized layout aimed at the first finished child for the 100 and 1000 cases; Dockview scopes the saved entry by parent session ID because these fixtures have no session environment, and the test verifies that no finished child panel appears when the parent panel mounts or after the delayed identity response arrives.

Phone runs use the existing full-screen child drawer and have no desktop Dockview maximize control; the phone test verifies child open/Back behavior and the same post-navigation subscription and heap measures.

## Corrected history-navigation protocol (2026-11-13)

The original helper incorrectly required full `errand.list` responses at navigation, reload, task switch and saved-layout restore, treated finished Errands as ordinary session identities, and required `finishedCount + 2` rows to be mounted simultaneously. The corrected helper is baseline/candidate shared and phase-label-only: it records transport and store counts without waiting for a legacy eager response, while asserting the exact three ordinary session IDs (parent plus two siblings) on cold navigation, reload, warm switch and identity restore.

For history, the shared logical workload visits every finished fixture identity in 25-ID ranges, records the range boundaries and IDs, and records rendered finished rows and total panel rows separately at each range. The old baseline's complete response already exposes every range; a candidate traverses further ranges with the visible Load more/Next page control. Before opening the same first five children, the helper verifies/restores the first range using Start again when the candidate's 100-summary cache has evicted it. This measures the same logical history identity set without asserting that all history remains mounted together.

The history transport observation includes received `errand.list` and `errand.history_page` frames and their reported row counts. The history store observation includes total, ordinary and Errand-role rows, but only exact ordinary identities are a compatibility assertion. Saved-maximized restore requires the parent and complete ordinary identities, and asserts no finished-child panels at parent readiness or after ordinary identity resolution; it does not wait for historical identities or require a full session-list HTTP response. `KANDEV_HISTORY_MEASUREMENT_PHASE` is stored only in the sample manifest and never changes helper behavior.

Cold parent-input, reload-input and warm-switch timing boundaries remain before history browsing, so the existing latency samples and all 30 recorded baseline caps are unchanged. The repeated-navigation heap endpoint remains after full logical history traversal, first-range restoration, five child Open/Back loops and forced GC. If the corrected workload requires supplemental heap cohorts, keep those in a separate evidence artifact and leave every original sample and cap in `baseline-results.json` unchanged. The initial phone 1000-history timeout remains a failure with an unknown awaited step; cleanup no longer lets a closed-target CDP detach mask the primary test error, and `release()` is attempted even when detach fails.

The unchanged-production smoke keeps the original heap cohort comparable: its full history was already resident before child navigation, the helper's range checks do not mutate application state or retain additional browser data, and the same five Open/Back loops still precede the same forced-GC endpoint. Candidate browsing must make explicit page requests, but that is a measured candidate behavior against the original full-list baseline. No supplemental heap cohort is necessary for this helper-only correction; the existing caps are neither recalculated nor relaxed.

The final corrected smoke used helper SHA-256 `ef7d21f3ea03b0dfadb60c24e40c7cb764cca455e32643ac42fc35054a870f64`, source SHA `6e4c5ca9bce14b952d8cbaa52dc0282af870d835`, one worker/shard, the managed global setup, and the shared build lock. It passed once on desktop 1000-history in 52.4 seconds (`/tmp/kandev-run.e2e.djNNRtNi.log`; raw sample `/tmp/kandev-history-correction-final-desktop-1000/session-errand-history-bas-3f89e-eline-1000-finished-Errands-chromium/errand-history-sample.json`) and once on phone 1000-history in 46.3 seconds (`/tmp/kandev-run.e2e.WZzEGoou.log`; raw sample `/tmp/kandev-history-correction-final-phone-1000/session-mobile-errand-hist-d1562-eline-1000-finished-Errands-mobile-chrome/errand-history-sample.json`). Both schema-v2 samples observed 40 logical ranges and 1000 expected identities, 1000 retained finished rows and 1002 retained panel rows, exact three ordinary IDs, a usable parent composer, retained parent subscription, and zero finished-child subscriptions; the old full-list UI needed zero Load more actions or Start again resets. Desktop saved-layout restore recorded no child panels at parent readiness or after ordinary identities resolved. These are smoke receipts, not additions to the seven-sample numeric cohorts or new caps.

## Environment and repeat commands

The samples ran on Linux `7.0.12-1-pve x64`, Node `v24.22.3`, Chromium `149.0.7827.55`, six reported CPUs, and 16,777,216,000 bytes of reported memory; E2E runs used the standard managed global setup, one worker, one shard, the regular browser cache policy, and `KANDEV_HISTORY_MEASUREMENT_PHASE=baseline` only to label the samples.

The full matrices were run serially under `/tmp/kandev-errands-repair-build.lock` with `GOMAXPROCS=2`, `GOFLAGS=-p=1`, `GOMEMLIMIT=3GiB`, private Go caches, and Corepack-pinned `pnpm@9.15.9`:

```bash
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errand-history-baseline.spec.ts -- --repeat-each=7 --retries=0 --output=/tmp/kandev-history-baseline-desktop
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errand-history-baseline.spec.ts -- --repeat-each=7 --retries=0 --output=/tmp/kandev-history-baseline-phone
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errand-history-baseline.spec.ts -- --grep '1000 finished Errands' --repeat-each=2 --retries=0 --output=/tmp/kandev-history-baseline-phone-1000-rerun
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-errand-history-baseline.spec.ts -- --grep '1000 finished Errands' --repeat-each=5 --retries=0 --output=/tmp/kandev-history-baseline-phone-1000-fixed-build
```

The exact environment manifest and test-source hashes are attached to every raw sample; fixture binary SHA-256 is `df656af7f7084741732f216286566ecc65f11243f9b4c91ae4ebd82e2b878490`, frontend dist SHA-256 is `054703cbaa8dab10b31a79ceb0430b7a3ecbecb2a8bf106fc93d209b26587749`, and manifest SHA-256 is `5712c627b4896bad093aeffa4638e3a92a7c799423d529f3be169744b9df09b9`.

Backend binaries were built separately for the desktop matrix, phone matrix, and phone 1000 follow-up because the Makefile embeds build time; numeric cohorts use seven samples from one exact binary SHA-256 apiece, and all cohorts share the same source SHA, frontend dist, fixture binary, test harness hashes, manifest, browser, and Node version.

## Measured request and materialization results

Values below are per-condition medians across the seven same-build samples; HTTP body totals use `response.body()` decoded bytes, the `/t/{task}` row is the direct page response, the desktop task-session row is the initial `GET /api/v1/tasks/{task}/sessions`, and Errand bytes count the received `errand.list` WebSocket frame.

| Platform | Finished | HTTP responses | HTTP decoded body bytes | `/t/{task}` body bytes | Task-session list body bytes | `errand.list` bytes / rows | Mixed task store rows |
| -------- | -------: | -------------: | ----------------------: | ---------------------: | ---------------------------: | -------------------------: | --------------------: |
| Desktop  |        0 |             77 |                 146,505 |                 36,572 |                       11,830 |                  1,531 / 2 |                     5 |
| Desktop  |      100 |             77 |                 506,244 |                137,480 |                      270,651 |              119,231 / 102 |                   105 |
| Desktop  |     1000 |             73 |               2,008,140 |              1,045,585 |                      867,933 |           1,178,531 / 1002 |                  1005 |
| Phone    |        0 |             60 |                 141,240 |                 36,569 |                       11,830 |                  1,531 / 2 |                     5 |
| Phone    |      100 |             60 |                 500,970 |                137,477 |                      270,651 |              119,231 / 102 |                   105 |
| Phone    |     1000 |             57 |               1,138,459 |              1,045,578 |         0, no separate route |           1,178,531 / 1002 |                  1005 |

The ordinary control task materialized exactly three ordinary rows after every warm switch for all six conditions; the mixed task retained three ordinary rows while its store grew from five total rows at 0 finished children to 105 at 100 and 1005 at 1000, including its two fixed outstanding controls.

The initial parent visit received all finished children in one `errand.list` response; opening the history surface produced no additional `errand.list` response, and the measured UI rendered the full available completed list without a next-page request in this baseline.

On desktop the saved-layout reload returned 105 session identities at 100 finished children and 1005 at 1000; parent panel readiness preceded the natural identity response in all fourteen samples, with median post-parent identity-store lag of 394 ms at 100 and 1418 ms at 1000, and no finished child panel mounted before or after identity arrival.

For each 100 and 1000 condition on desktop and phone, the harness opened and backed out of five finished children, observed five child panel mounts and five removals, retained zero finished-child rich subscriptions, and retained the parent rich subscription; the zero-finished condition correctly has no child to open.

## Frozen candidate regression limits

For each metric and condition, use median, nearest-rank p95 at index `ceil(0.95*n)-1`, and MAD (median absolute deviation); freeze tolerance as `max(3 * MAD, p95 - median)` and candidate p95 cap as `baseline p95 + tolerance`.

Every numeric cohort contains seven successful same-binary samples, so nearest-rank p95 is the maximum observed value; phone cold-ready p95 includes a 4.1 second 0-history outlier and a 4.6 second 1000-history outlier, which widen those baseline-derived caps and must remain visible when interpreting them.

The tuples below are `median / p95 / tolerance / frozen upper limit`; time values are milliseconds, while the repeated-navigation heap measure is bytes after forced GC.

| Platform | Finished |           Cold parent input ready |          Cold reload input ready |     Warm switch to ordinary task |      Warm switch back to history |                             Repeated-navigation heap |
| -------- | -------: | --------------------------------: | -------------------------------: | -------------------------------: | -------------------------------: | ---------------------------------------------------: |
| Desktop  |        0 | 2297.9 / 3833.3 / 1535.4 / 5368.7 | 1553.4 / 2152.4 / 599.0 / 2751.4 |  1435.6 / 1491.4 / 67.9 / 1559.3 |  991.6 / 1133.9 / 295.7 / 1429.6 |       63,805,964 / 64,075,652 / 269,688 / 64,345,340 |
| Desktop  |      100 |  2483.8 / 2701.0 / 217.2 / 2918.1 |  1845.7 / 1931.8 / 87.8 / 2019.6 | 1392.9 / 1555.6 / 162.7 / 1718.3 |   934.6 / 999.5 / 144.0 / 1143.5 |       71,675,204 / 72,628,720 / 953,516 / 73,582,236 |
| Desktop  |     1000 |  3573.0 / 3814.7 / 257.9 / 4072.6 | 3453.4 / 3621.4 / 207.2 / 3828.6 | 1674.1 / 2236.2 / 562.2 / 2798.4 |  1389.6 / 1486.5 / 96.9 / 1583.3 | 104,489,400 / 123,663,876 / 19,174,476 / 142,838,352 |
| Phone    |        0 | 1885.4 / 4105.5 / 2220.1 / 6325.6 | 1283.4 / 1421.2 / 228.5 / 1649.7 | 1337.4 / 1493.5 / 156.1 / 1649.6 | 1375.8 / 1425.5 / 146.8 / 1572.3 |        49,715,212 / 49,746,736 / 31,524 / 49,778,260 |
| Phone    |      100 |  2028.4 / 2156.2 / 127.8 / 2284.0 | 1498.5 / 1614.1 / 179.0 / 1793.1 | 1336.7 / 1391.6 / 122.6 / 1514.2 |  1367.4 / 1426.0 / 58.6 / 1484.7 |       54,678,156 / 54,785,124 / 210,780 / 54,995,904 |
| Phone    |     1000 | 3079.8 / 4627.1 / 1547.3 / 6174.4 | 2951.9 / 3379.0 / 427.0 / 3806.0 | 1828.6 / 2034.1 / 296.9 / 2331.0 | 1288.8 / 1410.7 / 121.9 / 1532.7 |   99,537,812 / 100,941,768 / 4,211,868 / 105,153,636 |

The complete per-sample values and exact floating-point limits are in [baseline-results.json](baseline-results.json); an otherwise equivalent candidate must use the same fixture, browser, build discipline, and seven-run sample count, compare each platform/history condition independently, and keep its p95 at or below the corresponding frozen cap.

## Run receipts and evidence

The desktop full matrix passed 21/21 tests; the phone full matrix passed 20, failed one 1000-history run, skipped zero, and used zero retries; two phone 1000 follow-ups and five further same-build phone 1000 runs passed, yielding seven successful samples for the phone 1000 numeric cohort.

The phone failure reached the 240-second Playwright timeout; its `finally` cleanup then reported a closed-page CDP detach and masked the awaited operation, while the backend log showed state-read deadlines and an empty queued message against a fixture task without an executor record; the screenshot shows the parent composer usable, the recovery-attention chip visible, and no drawer open, so the original awaited step is not known.

Logs are `/tmp/kandev-run.e2e.V1kiWhSB.log`, `/tmp/kandev-run.e2e.lKswJYSy.log`, `/tmp/kandev-run.e2e.5TSN1Fjw.log`, and `/tmp/kandev-run.e2e.Hr4HT34l.log`; raw sample files and their full environment manifests remain under `/tmp/kandev-history-baseline-desktop`, `/tmp/kandev-history-baseline-phone`, `/tmp/kandev-history-baseline-phone-1000-rerun`, and `/tmp/kandev-history-baseline-phone-1000-fixed-build`.

The failing-run context and screenshot are at `/tmp/kandev-history-baseline-phone/session-mobile-errand-hist-d1562-eline-1000-finished-Errands-mobile-chrome-repeat6/error-context.md` and `/tmp/kandev-history-baseline-phone/session-mobile-errand-hist-d1562-eline-1000-finished-Errands-mobile-chrome-repeat6/test-failed-1.png`; the corresponding backend log is `/tmp/kandev-e2e-6-ZxdjcP`.

Focused checks passed: `gofmt`, exact-file Oxlint, `lint:e2e-sleeps`, `lint:i18n`, web typecheck, and `python3 scripts/list-docs.py validate` (453 decisions and 2014 specifications); direct `golangci-lint run ./internal/orchestrator --new-from-rev=37f081a976526de1a6967f8714e72e247ec0b386 --timeout=10m --concurrency=2` completed with zero issues after the changed-Go wrapper reached its built-in five-minute timeout.

## Measurement limits

The fixture boundary and browser store expose rows returned and materialized, not SQLite rows scanned, joined, or enriched; do not label returned rows as SQL work.

CDP `ScriptDuration` and `TaskDuration` are broad renderer deltas over the navigation or interaction interval, not isolated ordinary-session reconciliation time; the baseline has no production seam to attribute that work without instrumentation changes.

The desktop saved-layout test records natural session-identity timing, but the fixture does not introduce an artificial identity delay; a controlled delayed-identity comparison needs a separate authorized fixture boundary.

HTTP decoded body totals use Playwright response buffers; request-size encoded-body totals and headers are also retained, but a cached mock-agent logo response is reported as negative 378 bytes by Playwright and skews the encoded aggregate by that amount, so compare the target `/t/{task}` and task-session routes or decoded totals.

No production code, production instrumentation, public API, live database, or running main instance was changed or used for fixture seeding; all E2E data lives in managed isolated temporary databases, and no browser test launched an agent process.
