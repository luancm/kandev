# Second paired performance cohort

The complete identified candidate includes bounded history, ordinary-session isolation, child-detail release/background guards, the late-fetch loading-metadata guard and the compact parent-state SQL projection. This cohort follows the unchanged [paired protocol](paired-performance-protocol.md); the first matrix and all earlier failures remain intact. Both builds used the identical common helper and fixed workload. The extra child-cache observation runs after the primary heap endpoint and does not mutate application state.

**Result: Task04 is not accepted.** All 84 scenarios passed without retries or skips; every candidate sample satisfies structural bounds. Twenty-eight of thirty numeric limits pass. The source manifests match before and after all 28 runs. No threshold is relaxed, no sample is replaced and no new commit or push is made.

The zero-history cohort contains three ordinary sessions plus two outstanding Errand controls and zero finished children. It is not a cold-start test of a task with absolutely no Errands. A separate three-session, no-Errand task supplies the warm ordinary-navigation control. All six of those warm-switch control limits pass.

## Numeric results

Values below are nearest-rank p95 from seven samples per build/cohort. Memory is MiB; latency is milliseconds. Limits use the frozen formula on the contemporaneous baseline. Seven-run descriptive samples do not establish a formal confidence interval.

| Platform | Finished children | Measure | Baseline | Candidate | Limit | Result |
| --- | ---: | --- | ---: | ---: | ---: | --- |
| desktop | 0 | coldParentInputAcceptedMs | 2349.320 | 2540.976 | 2484.660 | FAIL |
| desktop | 0 | coldReloadInputAcceptedMs | 1591.181 | 1625.108 | 1688.622 | PASS |
| desktop | 0 | postGcHeapAfterRepeatedNavigationBytes | 64.579 | 61.791 | 64.782 | PASS |
| desktop | 0 | warmSwitchBackToHistoryTaskMs | 916.667 | 987.448 | 1072.687 | PASS |
| desktop | 0 | warmSwitchToZeroErrandTaskMs | 1382.395 | 1504.148 | 1705.933 | PASS |
| desktop | 100 | coldParentInputAcceptedMs | 2469.240 | 2336.752 | 2586.772 | PASS |
| desktop | 100 | coldReloadInputAcceptedMs | 1870.429 | 1918.102 | 1971.054 | PASS |
| desktop | 100 | postGcHeapAfterRepeatedNavigationBytes | 69.814 | 70.011 | 70.938 | PASS |
| desktop | 100 | warmSwitchBackToHistoryTaskMs | 1002.665 | 1065.081 | 1116.498 | PASS |
| desktop | 100 | warmSwitchToZeroErrandTaskMs | 1439.320 | 1448.108 | 1775.380 | PASS |
| desktop | 1000 | coldParentInputAcceptedMs | 3421.211 | 3028.706 | 3502.225 | PASS |
| desktop | 1000 | coldReloadInputAcceptedMs | 4858.269 | 2582.196 | 4922.196 | PASS |
| desktop | 1000 | postGcHeapAfterRepeatedNavigationBytes | 146.636 | 70.165 | 194.357 | PASS |
| desktop | 1000 | warmSwitchBackToHistoryTaskMs | 1467.639 | 1232.011 | 1624.005 | PASS |
| desktop | 1000 | warmSwitchToZeroErrandTaskMs | 2573.609 | 1590.708 | 2762.734 | PASS |
| phone | 0 | coldParentInputAcceptedMs | 1975.347 | 1951.579 | 2121.690 | PASS |
| phone | 0 | coldReloadInputAcceptedMs | 1327.451 | 1318.479 | 1417.426 | PASS |
| phone | 0 | postGcHeapAfterRepeatedNavigationBytes | 47.876 | 48.024 | 47.934 | FAIL |
| phone | 0 | warmSwitchBackToHistoryTaskMs | 1346.762 | 1357.572 | 1373.191 | PASS |
| phone | 0 | warmSwitchToZeroErrandTaskMs | 1343.377 | 1359.579 | 1385.629 | PASS |
| phone | 100 | coldParentInputAcceptedMs | 1989.303 | 2003.699 | 2051.537 | PASS |
| phone | 100 | coldReloadInputAcceptedMs | 1420.038 | 1338.282 | 1440.897 | PASS |
| phone | 100 | postGcHeapAfterRepeatedNavigationBytes | 53.121 | 52.337 | 53.499 | PASS |
| phone | 100 | warmSwitchBackToHistoryTaskMs | 1360.660 | 1329.712 | 1411.360 | PASS |
| phone | 100 | warmSwitchToZeroErrandTaskMs | 1366.179 | 1382.171 | 1404.409 | PASS |
| phone | 1000 | coldParentInputAcceptedMs | 3069.160 | 2304.980 | 3196.697 | PASS |
| phone | 1000 | coldReloadInputAcceptedMs | 4026.894 | 1621.239 | 4225.113 | PASS |
| phone | 1000 | postGcHeapAfterRepeatedNavigationBytes | 128.714 | 51.099 | 184.437 | PASS |
| phone | 1000 | warmSwitchBackToHistoryTaskMs | 1288.122 | 1285.178 | 1313.197 | PASS |
| phone | 1000 | warmSwitchToZeroErrandTaskMs | 3153.290 | 1366.195 | 3366.867 | PASS |

## Remaining limits

Desktop zero-finished cold start is 2540.976 ms against a 2484.660 ms limit, an excess of 56.316 ms. Baseline median is 2213.979 ms and candidate median is 2274.506 ms; one candidate round is slower by approximately 327 ms, so the observed tail cannot simply be called measurement noise. In the inspected first pair both builds made 77 HTTP requests, with fewer response bytes in the candidate; this single-pair observation does not identify the remaining CPU cost.

Phone zero-finished retained heap is 50,357,064 bytes against a 50,262,616-byte limit, an excess of 94,448 bytes (about 92.2 KiB). Its p95 is 155,612 bytes (about 0.31%) above the contemporaneous baseline. The earlier optional 1% fixed-memory-tolerance question has no answer, so this remains a failed strict gate.

Both remaining limits concern low-history fixed cost. All 100/1000-history limits pass, as do all ordinary no-Errand warm-navigation controls. This supports resolving the history-growth concern but does not erase the two remaining acceptance failures. General asset optimization and normal-session lazy-loading conversion remain outside this package.

## Evidence and limits

- Raw samples and runner receipts: `/tmp/kandev-history-paired-v2`; commands, durations and log paths are in `runs.json`.
- Durable complete metrics, round deltas, original-limit comparisons and per-sample hashes: [paired-performance-v2-results.json](paired-performance-v2-results.json).
- Backend correction and checks: [parent-state-performance-handoff.md](parent-state-performance-handoff.md).
- Candidate structural observations: ordinary membership is exactly three sessions with no child rows; history pages return at most 25 rows and cache at most 100 summaries; no complete-list fallback; no finished-child rich subscriptions; zero full child session/transcript/turn caches after Back.
- Each run uses isolated local fixtures and SQLite for browser behavior. PostgreSQL semantics were checked separately; these are not Mac or live-provider performance measurements. SQL aggregates may still scan history. Small shared hydration bookkeeping maps have not been exhaustively audited; do not generalize the measured child-cache observations to every allocation in the application.

Receipt SHA-256 values:

- `/tmp/kandev-history-paired-v2-results.json`: `a2e441af213d53dafd588c5788a40ee0460a2b977859fcf3abb21755068cb93d`
- `/tmp/kandev-history-paired-v2/runs.json`: `8a62a396da52345fc38caae1272ea7cae34b458a18f1648d0635033e8a0a061b`
- `/tmp/kandev-history-paired-v2/source-before.json`: `3b3382d6ff33fcc6a500271a325409d1f461818d7fadd725204ce747ddd8c6b2`
- `/tmp/kandev-history-paired-v2/source-after.json`: `3b3382d6ff33fcc6a500271a325409d1f461818d7fadd725204ce747ddd8c6b2`

## Post-cohort cold-start diagnosis

Three cold-start CPU captures per build completed without failures: baseline managed log `/tmp/kandev-run.e2e.PvFSRoZ9.log`, candidate `/tmp/kandev-run.e2e.OOVN8iPD.log`. The samples show generic rendering, program time and garbage collection, with no isolated Errands-specific hot function. The largest named application frame is the same resize-observer callback in both bundles. Summary: `/tmp/kandev-history-cold-profile-summary.log`. Profiling adds overhead, so these timings do not replace the accepted measurement protocol or any cohort sample.

A separate single cold-start render-count diagnostic per build passed (logs `/tmp/kandev-run.e2e.ItiShmuk.log` and `/tmp/kandev-run.e2e.sCSEMQxC.log`). Both recorded 117 React commits. Fiber work flags may persist and are not reliable render-duration measurements; the counts rule out an obvious extra-commit loop in this exercise, not every additional render cost. No new production change followed.

Temporary instrumentation was restored byte-for-byte in both helpers. All owned browser/backend processes exited and the shared build lock is free. A post-cohort cleanup removed 5 GiB of owned Go-cache data older than two hours, preserving raw samples, profiles and build artifacts; about 18 GiB free remains. The first-delivery gate is still open for the two recorded fixed-cost failures. There is no new commit or push.

Diagnostic artifact hashes:

- `/tmp/kandev-history-cold-profile-baseline/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium/cold-start.cpuprofile`: `b52d41b5be723d18caad3f839b6a528d2ce83a7b4bd3af5940050cd3aaacf351`
- `/tmp/kandev-history-cold-profile-baseline/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium-repeat1/cold-start.cpuprofile`: `acacb5db8213b6b73a04a5bb22dcdb44b00e7e3fffcbc666ab3841b7fcabea54`
- `/tmp/kandev-history-cold-profile-baseline/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium-repeat2/cold-start.cpuprofile`: `2f416a9d2ce7128dfdecfa7fbcf761bbb37debe997087e36c2b17d84872fbc77`
- `/tmp/kandev-history-cold-profile-candidate/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium/cold-start.cpuprofile`: `eb7b4092d936a3c601aff37ee0549abce4babe5ad6647965d1df4ae04f412b8c`
- `/tmp/kandev-history-cold-profile-candidate/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium-repeat1/cold-start.cpuprofile`: `6b60e332a18885a3d8325d4abd3aa3391e7c7af5f74a45fc948ea71d86b277a1`
- `/tmp/kandev-history-cold-profile-candidate/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium-repeat2/cold-start.cpuprofile`: `13bd4c1ea9e3d92da1a9663444053a15a891da95994884ebd681da1c113404f3`
- `/tmp/kandev-history-cold-render-baseline/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium/cold-render-diagnostic.json`: `1ff29748006f7fd00c2d8270e14de7e1898322ce15c59611426d0a371ff95484`
- `/tmp/kandev-history-cold-render-candidate/session-errand-history-bas-a124f-baseline-0-finished-Errands-chromium/cold-render-diagnostic.json`: `bf70d03c32567a805595357eb963704596a44695e17360005df65ac875692b1d`
