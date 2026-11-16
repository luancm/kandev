# Final ordinary-page comparison

The final candidate `ee0f077cb9d8061cf17437021f087326978aed05` includes the same upstream main revision as the control, `585824a5d066d22785814c0394b88dd57f6c5ded`. All 28 scenarios passed: seven alternating main/candidate pairs on desktop and phone, with no retries. Production source, backend binaries, web assets and the common harness were unchanged throughout the cohort. Each sample matches its frozen source/backend identity and the fixed ordinary-only fixture.

## Measurements

| Platform | Metric            | Main median | Candidate median | Median delta       | Candidate p95 | Warning boundary | Above boundary |
| -------- | ----------------- | ----------- | ---------------- | ------------------ | ------------- | ---------------- | -------------- |
| desktop  | Cold navigation   | 2350.1 ms   | 2318.1 ms        | -32.0 ms (-1.36%)  | 2355.3 ms     | 4306.0 ms        | No             |
| desktop  | Reload            | 1672.0 ms   | 1645.2 ms        | -26.8 ms (-1.60%)  | 1711.3 ms     | 1905.2 ms        | No             |
| desktop  | First switch away | 1307.9 ms   | 1299.9 ms        | -8.0 ms (-0.61%)   | 1521.4 ms     | 2009.8 ms        | No             |
| desktop  | First switch back | 924.3 ms    | 973.8 ms         | 49.4 ms (+5.35%)   | 1005.4 ms     | 1166.0 ms        | No             |
| desktop  | Retained JS heap  | 72.731 MiB  | 73.559 MiB       | 0.828 MiB (+1.14%) | 75.553 MiB    | 75.442 MiB       | Yes            |
| phone    | Cold navigation   | 1940.9 ms   | 1950.6 ms        | 9.7 ms (+0.50%)    | 2013.7 ms     | 2048.6 ms        | No             |
| phone    | Reload            | 1372.0 ms   | 1337.3 ms        | -34.7 ms (-2.53%)  | 1384.0 ms     | 1532.8 ms        | No             |
| phone    | First switch away | 1325.0 ms   | 1401.3 ms        | 76.2 ms (+5.75%)   | 1455.5 ms     | 1537.5 ms        | No             |
| phone    | First switch back | 1334.2 ms   | 1353.8 ms        | 19.6 ms (+1.47%)   | 1381.3 ms     | 1571.9 ms        | No             |
| phone    | Retained JS heap  | 52.116 MiB  | 52.919 MiB       | 0.802 MiB (+1.54%) | 52.988 MiB    | 52.237 MiB       | Yes            |

The warning formula remains `reference p95 + max(3 × MAD, p95 − median)`. With seven samples, nearest-rank p95 is the maximum; it is not a confidence interval. Full observations, ranges, per-round paired deltas, requests, renderer times, resource totals and raw-sample hashes are retained in [the results JSON](../errands-simplification/evidence-index.md#summary-followup-results).

## First-delivery disposition

Accept the implemented package for the first delivery under the user's direction to prioritize dependable bounded work and assess proportionate fixed cost. All eight final latency measures stay within the unchanged main-reference warning boundaries. The final desktop first-switch difference in medians is −8.0 ms; phone is +76.2 ms. This seven-sample experiment does not prove zero latency regression, and the phone difference remains a pilot observation.

Both fixed-heap warnings remain explicit. Differences in medians are +0.828 MiB desktop and +0.802 MiB phone; median paired-round differences are +1.171 MiB and +0.805 MiB. This is consistent with the approximately one-MiB fixed-cost tradeoff discussed with the user. No universal memory allowance is introduced, and no original threshold or sample is changed.

The earlier history-growth matrix remains 84/84 functional scenarios and 28/30 strict numeric limits. Its desktop zero-finished cold p95 misses by 56.316 ms and phone heap misses by 94,448 bytes (92.2 KiB) are accepted as disclosed first-delivery exceptions. Every 100/1000-history limit and separate ordinary warm-navigation control passed; deterministic row, request, cache and subscription bounds passed. Inspection releases full child caches after Back. These observations support the bounded-loading goal while preserving the small fixed-cost findings. Indexed SQL aggregation may still scale with history; constant database work is not claimed.

The two strict misses remain failed numerical observations in the earlier report. The first-delivery decision is an explicit engineering disposition using the user's agreed priorities, the bounded-history evidence and the final main comparison. The next gate is the small real-provider reliability pilot in the [bounded-use roadmap](../errands-remaining/bounded-use-roadmap.md), after the running instance includes the repair.

## Requests and resource bodies

| Platform / arm         | Errand requests across one workload | Cold resource body bytes (range) |
| ---------------------- | ----------------------------------- | -------------------------------- |
| desktop / matched-main | None                                | 18,843,387–18,843,417            |
| desktop / candidate-on | `errand.browser_summary` 3–3        | 18,950,281–18,950,304            |
| phone / matched-main   | None                                | 18,725,199–18,725,242            |
| phone / candidate-on   | `errand.browser_summary` 3–3        | 18,830,646–18,832,212            |

Resource body totals are browser Resource Timing entries and exclude the HTML document. Heap is post-GC JavaScript heap, not browser-process RSS, backend memory, provider cost or disk usage.

## Evidence and limits

Source/artifact manifest SHA-256: `d9475314ef0476a4a7985ddd8aa74b02819acfd10bb9ac6738934828e47c0cf7`. The unchanged before/after manifests and command receipts are retained under `/tmp/kandev-summary-followup/`. The runner is `/tmp/kandev-summary-final-run.py`; each recorded command uses the standard isolated managed runner, `--no-build`, one worker/shard and zero retries after rebuilt smoke checks.

This is an ordinary-only local workload: two tasks, three waiting conversations per task and ten small agent messages per conversation, without Errands or provider executions. It does not establish behavior for large transcripts, remote latency, every provider/executor, backend startup recovery or physical phones. The phone cohort uses the native mobile-chrome test configuration. Main and candidate include the same upstream revision, but the feature-branch difference is broader than the shared-summary correction; this comparison does not attribute every difference to that single fix.

The earlier [four-arm comparison](main-comparison-handoff.md) and [history-growth comparison](paired-performance-v2-handoff.md) remain unchanged. Their original warnings and two strict-limit misses are retained. This follow-up measures the final ordinary candidate and does not replace the zero/100/1000-history workload.

## Historical shared-summary publication repair (2026-11-14)

This subsection preserves the pre-integration repair checkpoint from `dcc3de55a2`; its then-pending measured follow-up is resolved above.

Multiple mounted consumers already share one browser-summary request, but each consumer published loading and the resolved summary into Zustand. The new regression observes the real store subscription: one attempt emitted two loading updates and two successful updates. Error publication was already idempotent for the same error object.

The store now begins each attempt once and accepts its result/error only while that attempt is loading. Existing connection, revision, identity and replacement-attempt guards remain intact. The change removes duplicate store notifications; it does not remove the summary request or alter history freshness. Retry and shared-consumer lifetime controls remain covered.

RED: two failed, one passed, 28 unselected. GREEN: 51 tests across four files pass; typecheck, exact-file Oxlint and formatting pass. This shared state-only change has no rendered layout or touch changes; the mobile-parity state-only exception applies. The existing desktop/phone ordinary benchmark is being repeated separately to measure, rather than assume, any timing effect. Earlier results remain unchanged.

### Source and receipts

| Path                                                      | SHA-256                                                            |
| --------------------------------------------------------- | ------------------------------------------------------------------ |
| `apps/web/lib/state/slices/errands/errands-slice.ts`      | `e21ad83a27ae8d076c372472020b31269e0cf2318a2fe3fb6ff85fbf10780629` |
| `apps/web/lib/state/slices/errands/errands-slice.test.ts` | `04b202fd0d5aec9b8f86c377315b5e829a11c7c205aab0e551de2e44ec15abcf` |
| `/tmp/kandev-summary-publication-red.log`                 | `d8e3ab1c7610a0a65476b065c3ebecaa84402a48fc42fa5be229ba26830a5870` |
| `/tmp/kandev-summary-publication-green.log`               | `fe9201f70207420407b7bea61f09164ee4526c7d78b2e87a81015c4861096e97` |
| `/tmp/kandev-summary-publication-typecheck.log`           | `f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708` |
| `/tmp/kandev-summary-publication-lint.log`                | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-summary-publication-format.log`              | `17aa973d3f004560237d9a95171210b0671deff23d61628eecf7322ff5938f20` |

### Rebuilt acceptance before upstream integration

Managed desktop acceptance passed 3/3: ordinary task navigation, demand-only bounded history and repeated child-cache release (`/tmp/kandev-run.e2e.SKvGAJuS.log`). Phone acceptance on the same artifacts passed 2/2: bounded drawer/history and repeated child-cache release (`/tmp/kandev-run.e2e.y4xHRFB9.log`). The measured follow-up is deferred until the upstream-integrated branch and matching upstream control are built; no pre-merge timed cohort was collected. These functional checks do not silently close earlier numerical warnings.
