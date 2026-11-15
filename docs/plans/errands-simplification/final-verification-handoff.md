# Verification at 77b155597c: original performance finding

This receipt preserves the comparison before [Task04's subscription optimization](ordinary-header-subscriptions-handoff.md). Its measurements and limits remain unchanged. Task04 source review and resource checks have passed; fresh browser/performance acceptance is still pending. The warning below is not cleared by the source change alone.

Candidate `77b155597c1dab4b16016bc5e5da17ac6a16c632` passed **desktop 3/3, phone 3/3 and all 28/28 ordinary comparison scenarios**, with no retries. Source and artifacts stayed unchanged. **Performance acceptance remains open:** desktop return-navigation p95 is **1039.246185 ms**, above the unchanged **958.561579 ms** warning boundary by **80.684606 ms**. The other seven latency boundaries passed. Parent owns the bounded investigation and disposition; no rerun, source change or budget relaxation was made, and the real-provider pilot waits.

The control is `/tmp/kandev-main-latest-control` at `585824a5d066d22785814c0394b88dd57f6c5ded`, the same upstream revision incorporated by the candidate. All 9,419 recorded source entries, 693 web files, backend and common harness matched the accepted historical main manifest, so its build was reused. The candidate was rebuilt once through the managed host runner; phone reused it only after complete source/build-input/artifact equality. Both kept normal global setup. The existing [protocol](../errand-history-loading/main-comparison-protocol.md) and workload were unchanged.

## Measurements and finding

Each cell below is **median / p95 [minimum, maximum]**; heap is MiB, other values milliseconds. The warning boundary remains `reference p95 + max(3 × MAD, p95 − median)`. Seven observations make nearest-rank p95 the maximum, not a confidence interval. Exact values, original command receipts, all per-case/full log paths and hashes, raw samples, source/build identities and resource diagnostics are retained in [the results JSON](final-verification-results.json).

| Platform | Metric                 | Main                                 | Candidate                            | Difference in medians | Median paired delta | Main warning boundary | Result  |
| -------- | ---------------------- | ------------------------------------ | ------------------------------------ | --------------------- | ------------------- | --------------------- | ------- |
| desktop  | Cold navigation (ms)   | 2249.53 / 2286.94 [2227.85, 2286.94] | 2234.56 / 2297.22 [2181.55, 2297.22] | -14.97 (-0.67%)       | -11.26              | 2337.14               | Within  |
| desktop  | Reload (ms)            | 1574.94 / 1695.05 [1528.21, 1695.05] | 1590.88 / 1640.33 [1561.08, 1640.33] | +15.94 (+1.01%)       | +25.74              | 1815.15               | Within  |
| desktop  | First switch away (ms) | 1170.74 / 1373.02 [1129.55, 1373.02] | 1232.35 / 1377.09 [1156.14, 1377.09] | +61.60 (+5.26%)       | +63.87              | 1575.29               | Within  |
| desktop  | First switch back (ms) | 822.24 / 890.40 [814.31, 890.40]     | 979.45 / 1039.25 [864.24, 1039.25]   | +157.21 (+19.12%)     | +113.54             | 958.56                | Warning |
| desktop  | Retained JS heap (MiB) | 72.417 / 75.251 [72.327, 75.251]     | 73.482 / 76.231 [73.412, 76.231]     | +1.065 (+1.47%)       | +0.997              | 78.086                | Within  |
| phone    | Cold navigation (ms)   | 1906.20 / 1971.46 [1826.96, 1971.46] | 1846.83 / 1872.41 [1799.66, 1872.41] | -59.37 (-3.11%)       | -50.18              | 2088.39               | Within  |
| phone    | Reload (ms)            | 1325.70 / 1399.06 [1254.22, 1399.06] | 1269.15 / 1303.32 [1241.64, 1303.32] | -56.56 (-4.27%)       | -56.56              | 1472.76               | Within  |
| phone    | First switch away (ms) | 1319.31 / 1361.53 [1255.15, 1361.53] | 1349.91 / 1418.45 [1297.32, 1418.45] | +30.60 (+2.32%)       | +42.18              | 1429.30               | Within  |
| phone    | First switch back (ms) | 1307.58 / 1339.04 [1292.42, 1339.04] | 1357.91 / 1371.29 [1338.22, 1371.29] | +50.33 (+3.85%)       | +39.66              | 1384.51               | Within  |
| phone    | Retained JS heap (MiB) | 52.093 / 52.205 [52.083, 52.205]     | 52.955 / 53.022 [52.924, 53.022]     | +0.862 (+1.66%)       | +0.835              | 52.317                | Warning |

Desktop return navigation is slower in **all seven paired rounds**: median paired delta **+113.539438 ms**, range **+49.934614 to +184.369177 ms**. The difference in cohort medians is **+157.209688 ms (+19.119707%)**. This is a concrete warning in the present matched-main experiment. It does not establish a causal regression from Task01, Task02 or Task03 individually. The prior final cohort's eight latency results remain historical observations; they do not clear this new finding.

Phone heap still exceeds its boundary: candidate p95 **55,597,276 bytes**, boundary **54,858,056 bytes**. The median fixed-heap differences are **+1,117,004 bytes (1.065 MiB) desktop** and **+904,304 bytes (0.862 MiB) phone**. Desktop's heap boundary passes in this cohort, with greater reference variability. Preserve the previously discussed approximately one-MiB fixed-cost disposition; it is not a new universal allowance.

Paired candidate-minus-main deltas, in round order (milliseconds, or MiB for heap):

| Platform / metric           | Rounds 1–7                                                 |
| --------------------------- | ---------------------------------------------------------- |
| desktop / Cold navigation   | -84.26, +4.57, +1.65, -67.98, -11.26, +68.27, -14.50       |
| desktop / Reload            | +25.74, +32.87, +20.13, -121.12, +10.71, +36.36, +65.39    |
| desktop / First switch away | -9.67, +190.31, +28.52, -175.07, +110.65, +92.83, +63.87   |
| desktop / First switch back | +113.54, +119.46, +82.85, +166.43, +94.83, +49.93, +184.37 |
| desktop / Retained JS heap  | +0.980, +1.032, +0.240, +0.997, +3.006, +1.120, -1.684     |
| phone / Cold navigation     | -158.10, -27.30, -86.15, -39.33, -99.05, -15.57, -50.18    |
| phone / Reload              | -59.77, -16.02, +2.19, +46.29, -157.41, -56.56, -57.75     |
| phone / First switch away   | +56.92, +61.86, +19.04, +61.21, -2.47, +42.18, +8.01       |
| phone / First switch back   | +29.66, +24.29, +64.84, +39.66, +39.20, +45.80, +67.73     |
| phone / Retained JS heap    | +0.835, +0.833, +0.834, +0.871, +0.751, +0.850, +0.910     |

## Functional and visual controls

Desktop passed in **36.2 s** and phone in **34.1 s**. Both cover demand-only bounded history, explicit error/revision loading with live controls, and repeated child Open/Back. Each retention sample reports exactly `sessionRows=0`, `transcripts=0`, `turnCaches=0` after Back. The desktop summary/history and phone drawer screenshots were inspected: the expected attention/actions, history paging and phone footer remain visible without a concrete visual finding.

- Desktop full log: `/tmp/kandev-quality-final.EiqaN4sF/logs/kandev-run.e2e.ZWhCnQTd.log`; screenshots/sample under `desktop-controls/`.
- Phone full log: `/tmp/kandev-quality-final.EiqaN4sF/logs/kandev-run.e2e.GcDNWYmu.log`; screenshots/sample under `phone-controls/`.
- Every enabled ordinary workload made exactly **three `errand.browser_summary` requests**, with no history, inspection or complete-list request; main made none. Browser version was **149.0.7827.55** in all cases, desktop 1280×720 and native mobile-chrome 393×727.

## Source, build and evidence identities

| Identity                                    | SHA-256                                                            |
| ------------------------------------------- | ------------------------------------------------------------------ |
| Candidate backend                           | `f9653e1341b77cd6d316e9a7635583c0f0a899ffe00bce816b04dfaea94fe06e` |
| Candidate native agentctl                   | `f22b5e0ec8a473d7c854af0cf3083a80a309578c4dd2a1f0250a421f2804d9a4` |
| Candidate dist/index.html                   | `6ee9efe71ad66bba25a90dcfa844ff79216331339f0bdb2e5f2f4f40e8b41b34` |
| Main backend                                | `657efc7f4fa1ce0000c8efb30bc79fa2cec9b6d072f19b6a30a41ded56c92e5e` |
| Comparison source/artifact manifest         | `527fa976e6a165195f60250d2c3c2538795cc2a52a8799308f1298f9ce680518` |
| Complete source/build-input/artifact freeze | `aa038db6e79f19a5c5d3983cf5334622535bb14d3df102fa5b2370d99c7717c2` |
| Common comparison helper                    | `b1c944168c79a18cd96408700f8381b74bd9be988b2ce9818ea2d66370d46a99` |
| Desktop comparison spec                     | `6b0266749cd770c5fce9e55f88ad12ed1ae5e2d714ea8a78a2e1e5066f702f48` |
| Phone comparison spec                       | `c3f7ef66060fa9afbac3448fde5804cfb2ee8127d5cae0024fd03d74f991a750` |

The complete freeze checks cover **33,095 candidate source entries / 714 artifacts** and **32,465 main entries / 715 artifacts**, including module/build inputs and generated files beyond the comparison runner's narrower manifest. Post-desktop, pre-phone, pre-comparison and post-comparison snapshots are byte-identical. They were checked before authoring only this handoff and its results JSON. Candidate native helpers and all web-file hashes, changed production-source hashes, script hashes and canonical full-source digests are in the JSON.

Private raw evidence remains at `/tmp/kandev-quality-final.EiqaN4sF/`: `preflight.json`, `complete-*.json`, `resources.jsonl`, `release-check.json`, `main-comparison/{runs.json,source-before.json,source-after.json}`, all 28 scenario directories, full logs and `main-comparison-analysis.log`. This repository JSON includes full raw comparison samples. Parent archival verified all 172 files in `/home/luancm/kandev-task-evidence/errands-20261115/quality-final-performance.tar.gz`, SHA256 `a04dec68f05a3de77216638cad0ca4da6bc0bb68bef0b5accd0d75dbbb71f7db`; originals remain. Later diagnostics are preserved separately in `navigation-diagnostics.tar.gz`, SHA256 `a58665c837c6ca1a411c60e6eb8233c76bf3c72c67daae700f7f9bfa2023a8c9`, with all 88 members checked. No numerical result or protocol was rewritten.

## Resources, release and limits

The timed cohort took **639.66 s**, serialized under `/tmp/kandev-errands-repair-build.lock`, one worker/shard, with Node **24.22.3**, pinned pnpm **9.15.9**, private Go caches, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB` and `GOFLAGS=-p=1`. The Go memory setting is a soft target: the largest observed fixture-compiler RSS was about **8.65 GiB**, with host headroom monitored. No cache cleanup occurred. Minimum free space at a timed scenario start was **10,294,267,904 bytes (9.59 GiB)**; the release check recorded **10,281,721,856 bytes (9.58 GiB)**, above the 8 GiB stop floor.

All owned process handles are terminal: desktop **96028**, source snapshot **83955**, phone **77182**, comparison **69729**, all exit 0. No candidate/control browser, mock-agent, backend or agentctl process remains; the only named native runtime processes are the pre-existing main instance in `/home/luancm/projects/kandev`. The build lock is free. Writer/build ownership is released to the parent with this handoff; only the two authorized new evidence documents were authored. No main-instance/runtime-data change, production-web build or pilot launch occurred.

This ordinary local Chromium sample uses two tasks, three waiting sessions each, ten small messages per session, no Errands/provider executions, disabled HTTP cache, CPU rate 1, composer-acceptance timing and five additional task-switch pairs before GC. It does not certify physical phones, Safari, remote latency, large transcripts, backend startup/recovery, real-provider release, native/backend RSS or disk growth. The earlier history matrix stays **84/84 functional, 28/30 strict numeric limits**, retaining the **56.316 ms** desktop zero-finished cold miss and **94,448-byte** phone heap miss. The approximately one-MiB historical fixed-cost disposition and seven-sample limits remain explicit. The new return-navigation warning stays open for parent investigation and decision.
