# Main versus Errands: ordinary-page measurements

Historical comparison checkpoint, before the final upstream-integrated cohort: **56/56 scenarios passed**, with no retries or skipped samples and identical source/build manifests before and after the measured cohort. At this checkpoint the comparison did **not** establish zero overhead or close Task04; it revealed fixed costs that the earlier eager-Errands baseline could not measure. [Final measurements and first-delivery disposition](summary-followup-handoff.md) record the subsequent acceptance without replacing the observations below.

## What was compared

- Matched upstream main: `37f081a976526de1a6967f8714e72e247ec0b386`, already merged into the feature branch. This is the primary control for the complete branch delta.
- Latest upstream main: `e9c14a3e74ee99cacaeda531893a57e644e7593d`, 16 commits later. Differences include unrelated upstream changes; they cannot all be attributed to Errands.
- Current candidate, HEAD `6e4c5ca9bce14b952d8cbaa52dc0282af870d835` plus the frozen uncommitted implementation, with Errands disabled and enabled. These use identical binaries and assets. The full source manifest identifies the actual candidate.

Each arm ran seven desktop and seven native-phone Chromium scenarios. Two tasks each contained three ordinary waiting sessions with ten fixed messages per session, no Errands and no executing providers. Each scenario measured cold navigation, reload, first task switch, return, and retained JS heap after five more switch pairs. HTTP cache was disabled, CPU throttling was 1, and input timing ended only once the intended composer accepted a draft. All ordinary identities and expected transcript content were checked. See the [protocol](main-comparison-protocol.md) and [machine-readable results](../errands-simplification/evidence-index.md#main-comparison-results).

## Results

All values below are **median / p95**. With seven observations, nearest-rank p95 is the maximum, not a reliable estimate of rare production tail latency. Times are milliseconds; heap is MiB of browser JavaScript after forced GC.

| Surface / arm           |        Cold |      Reload | First task switch |      Return | Retained JS heap |
| ----------------------- | ----------: | ----------: | ----------------: | ----------: | ---------------: |
| desktop / matched-main  | 2318 / 2447 | 1636 / 1795 |       1282 / 1877 |  916 / 1027 |    72.53 / 74.51 |
| desktop / latest-main   | 2301 / 2369 | 1685 / 1726 |       1226 / 1417 |  866 / 1000 |    72.35 / 74.82 |
| desktop / candidate-off | 2317 / 2352 | 1666 / 1754 |       1286 / 1549 |  941 / 1107 |    73.27 / 75.93 |
| desktop / candidate-on  | 2318 / 2472 | 1627 / 1775 |       1463 / 1575 |  950 / 1344 |    73.64 / 75.41 |
| phone / matched-main    | 1957 / 2064 | 1330 / 1382 |       1293 / 1406 | 1331 / 1431 |    52.19 / 52.22 |
| phone / latest-main     | 1935 / 2011 | 1347 / 1428 |       1313 / 1374 | 1331 / 1361 |    52.09 / 52.11 |
| phone / candidate-off   | 1879 / 1970 | 1329 / 1492 |       1345 / 1454 | 1384 / 1436 |    52.92 / 52.99 |
| phone / candidate-on    | 1906 / 1976 | 1333 / 1383 |       1374 / 1427 | 1351 / 1377 |    53.02 / 53.07 |

## Interpretation

Against matched main, the enabled candidate has essentially unchanged median desktop cold navigation (−0.4 ms) and lower median phone cold navigation (−51.2 ms). This local sample is not evidence that Errands speeds up cold loading. Median reload is also close (desktop −9.6 ms, phone +3.4 ms).

There is a first-task-switch signal: desktop median rises by **181.2 ms / 14.1%**, and phone by **80.7 ms / 6.2%**. Paired-round differences are variable, so these are observed differences rather than an isolated causal diagnosis. Desktop return p95 is **1344.4 ms**, above the matched-main variance boundary **1186.4 ms**; the latest-main and candidate-off comparisons also flag that tail. A targeted navigation profile is the next useful diagnosis.

Retained heap increases by a median **1,157,232 bytes / 1.10 MiB / 1.52% on desktop**, and **864,000 bytes / 0.82 MiB / 1.58% on phone** against matched main. The phone increase appears in every paired round (786,964–916,096 bytes). Most of that phone fixed cost remains with Errands disabled: **761,592 bytes / 0.73 MiB / 1.39%** above main. Enabling it adds another **102,408 bytes** to the phone median compared with the disabled candidate. Desktop heap has greater variability. These values are fixed overhead for this ordinary workload, not proof of a leak or of unbounded growth. The separate history matrix owns growth acceptance.

Latest-main comparisons support the same broad concern but include unrelated changes. Enabled median heap is 1.78% higher on both surfaces; enabled first-switch median is 19.3% higher on desktop and 4.6% higher on phone. Consult the JSON for exact values, all seven raw metric values, ranges, MAD, paired-round deltas and every boundary calculation.

### Diagnostic traffic

Every enabled scenario made exactly **three `errand.browser_summary` requests** over the entire cold/reload/repeated-navigation sequence. No Errand history, inspection or complete-list requests occurred. Main and the disabled candidate made zero Errand requests. Candidate cold resource body bytes increased by about **104 KiB on desktop** and **103 KiB on phone** versus matched main (about 0.56%). This is the browser-reported sum for resource entries; it excludes the HTML navigation document and is not total wire traffic.

### Variance boundaries

The predeclared comparison formula flags the following candidate p95 values. These are exploratory warnings, not independent statistical significance tests or approved new allowances. No original Task04 gate is relaxed.

| Surface | Reference → candidate        | Metric            | Candidate p95 | Reference boundary |
| ------- | ---------------------------- | ----------------- | ------------: | -----------------: |
| desktop | matched-main → candidate-on  | switchBackMs      |       1344.37 |            1186.36 |
| desktop | latest-main → candidate-on   | switchBackMs      |       1344.37 |            1133.69 |
| desktop | candidate-off → candidate-on | coldMs            |       2472.16 |            2428.15 |
| desktop | candidate-off → candidate-on | switchBackMs      |       1344.37 |            1289.13 |
| phone   | matched-main → candidate-off | reloadMs          |       1491.55 |            1481.67 |
| phone   | matched-main → candidate-off | retainedHeapBytes |   55568048.00 |        54782416.00 |
| phone   | matched-main → candidate-on  | retainedHeapBytes |   55645488.00 |        54782416.00 |
| phone   | latest-main → candidate-off  | switchAwayMs      |       1454.18 |            1434.31 |
| phone   | latest-main → candidate-off  | switchBackMs      |       1435.92 |            1416.57 |
| phone   | latest-main → candidate-off  | retainedHeapBytes |   55568048.00 |        54682104.00 |
| phone   | latest-main → candidate-on   | retainedHeapBytes |   55645488.00 |        54682104.00 |

## Evidence and reproducibility

- Measured runs and complete raw samples: `/tmp/kandev-main-comparison/`; per-run log paths, commands, flags, browser versions, source heads, binary hashes and sample hashes are retained in the results JSON. The durable JSON retains measurement values and resource aggregates; full resource entries remain in the raw samples.
- Runner: `/tmp/kandev-main-comparison-run.py`; analyzer: `/tmp/kandev-main-comparison-analyze.py`; analysis output: `/tmp/kandev-main-comparison-analysis.log`.
- Source and artifact manifests: `/tmp/kandev-main-comparison/source-before.json` and `source-after.json`, byte-identical; SHA-256 `176f617859c8854c6513eb02762c01b2dc5001d5066ea1b2a9a5c3816152c6de`. Both main checkouts have only the three common test-harness files added, with tracked production source clean.
- Common helper SHA-256: `b1c944168c79a18cd96408700f8381b74bd9be988b2ce9818ea2d66370d46a99`. Standard global setup and managed builds were used. No production source was transplanted into main.
- Final preflight: typecheck and exact-file Oxlint exited 0; `/tmp/kandev-main-final-preflight-0.log` and `-1.log`. All eight final arm/surface smoke checks passed; the earlier phone-fixture mismatch and correction are recorded in the protocol.
- The measured run took about 21.8 minutes under the shared build lock, with one worker and rotating arm order. Disk stayed near 11.9 GiB free. All scenario/fixture processes ended; no live user runtime/database was used. After measurements, 6.01 GiB of private Go-cache entries older than two hours were removed under the shared lock, leaving 17.93 GiB free; the audit is `/tmp/kandev-main-comparison-cache-cleanup.json`. Raw evidence and built artifacts were preserved.

Final formatting, E2E sleep lint/ratchet, documentation catalog validation (453 decisions, 2014 specifications), specification lint and whitespace checks passed.

## Limits and remaining work

This measures saved-conversation loading in local Chromium, including Pixel phone emulation. It does not certify real phone hardware, Safari, the user’s Mac, remote latency, active provider processes, very large ordinary transcripts, backend startup/recovery latency, backend/native RSS, or disk use. The fixture uses seeded waiting sessions; existing ordinary-session functional suites remain necessary. Latest main’s unrelated source differences and finite-sample timing variability remain explicit limitations.

Task04 remained **in progress at this historical checkpoint**; its subsequent completion is recorded in the final disposition linked above. Preserve the two existing strict history-matrix failures, add the main-comparison navigation-tail and fixed-heap findings to the performance work, and diagnose before claiming no regression. The stronger existing result remains that bounded history substantially reduces large-history loading and retained heap; that does not cancel the fixed overhead found here. No commit, push or upstream merge was performed for this comparison.
