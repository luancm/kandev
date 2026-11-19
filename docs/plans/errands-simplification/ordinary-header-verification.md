# Task04 verification and open performance disposition

Candidate `90aaec3aee7b038cb3d650103d2384a9c4470534` was compared with matching upstream main `585824a5d066d22785814c0394b88dd57f6c5ded` on 2026-11-15. Desktop **3/3**, phone **3/3** and the ordinary comparison **28/28** passed without retries. Independent read-only review recomputed all ten numerical gates and paired deltas, verified all 28 raw sample hashes and confirmed unchanged source/artifact inventories. **Performance acceptance remains open.** Functional passes do not clear numerical warnings.

## Measured cost

Seven observations per arm/platform use the unchanged workload and boundary: `reference p95 + max(3 × MAD, p95 − median)`. With seven observations, nearest-rank p95 is the maximum. Exact values, paired vectors, raw identities, controls and prior-cohort comparisons are preserved in [the verification results](evidence-index.md#ordinary-header-verification).

| Observation               | Difference in medians | Median paired difference | Gate    |
| ------------------------- | --------------------: | -----------------------: | ------- |
| Desktop first switch away | +175.806 ms (+14.61%) |               +69.361 ms | Pass    |
| Desktop first switch back |   +48.449 ms (+5.54%) |               +48.449 ms | Pass    |
| Phone first switch away   |   +46.612 ms (+3.66%) |               +62.894 ms | Warning |
| Phone first switch back   |   +32.245 ms (+2.45%) |               +33.926 ms | Pass    |
| Desktop retained heap     |      +1,033,968 bytes |         +1,039,936 bytes | Pass    |
| Phone retained heap       |        +840,428 bytes |           +845,488 bytes | Warning |

Phone switch-away p95 is **1,554.249 ms**, exceeding the **1,432.753 ms** boundary by **121.496 ms**. One candidate observation exceeds the boundary, but six of seven paired differences are positive. The desktop switch-away cost remains material to the decision even though that variance gate passes. Cold-load and reload gates pass on both surfaces. Overall, seven of eight latency gates and one of two heap gates pass.

The [original desktop return warning](final-verification-handoff.md) at `77b155597c` remains recorded. In this separate cohort, candidate return median fell 55.924 ms while its p95 rose 20.394 ms; the main-derived boundary widened 243.785 ms. This is neither proof that Task04 caused a timing improvement nor clearance of the historical failure. The accepted reduction from twelve ordinary view-model subscriptions/callbacks to one is a separate deterministic result.

Earlier fixed-heap and first-delivery tradeoffs remain context, not blanket approval of new costs. No threshold, fixture, assertion or sample was replaced. Further implementation needs a concrete hypothesis; no further unchanged runs or diagnostic captures are planned. The release-performance decision remains with the parent/user. The already-authorized isolated reliability pilot can proceed while this release gate remains open; it does not grant performance acceptance.

## Functional and build evidence

The desktop command rebuilt once. Phone and comparison reused the same verified artifacts with normal global setup, one worker/shard, Node 24.22.3 and pnpm 9.15.9. Both surfaces materialize three ordinary rows and no child rows on cold load, reload and return. Repeated Open/Back leaves zero child session rows, transcripts and turn caches, while retaining the parent subscription. Desktop and phone screenshots were inspected.

Candidate inventories cover 33,100 source files and 714 artifacts; main covers 32,465 and 715. Complete maps stayed unchanged. Candidate production hook SHA-256 is `0c14eb224a724f30aee4aa80a09d54fe57e798ca1e9c10bf422b82e13a28a687`. The actual resource/recognition tests and source review are in the [source handoff](ordinary-header-subscriptions-handoff.md). The final E2E build is not proof of a production embedded-web pilot build.

All handles (desktop 32647, phone 26013, comparison 65594 and inventory checks) exited zero. No owned process remained; the shared lock was acquired and released. Disk was 9.20 GiB at release and stayed above the 8 GiB measurement floor. No cleanup occurred during measurements.

## Retained evidence

Raw packet: `/tmp/kandev-task04-final.pqprmph0/`. Its original handoff SHA-256 is `6494807dbc7a24a6d53a98391c5e727d7295e6c39a1496383b41b377602f4ade`; its 186-file manifest is `7ce345c8366b988137310675fafb9ce0fcd43599c27bffbd016fe31b1e83a4f4`. The copied results file remains byte-identical, SHA-256 `61259f6650c43b0e7018c71a0f702025a8a4ab03fbe981379a28f66c976c231e`.

Durable private archive: `/home/luancm/kandev-task-evidence/errands-20261115/task04-final-performance-and-assets.tar.gz`, SHA-256 `f66ccd9abcdc5ce2c3dc543c4bc63d2aa885562f5d458e0259ca3a2f24af62c8`. It includes all raw evidence and the exact E2E dist/generated assets that a later production build may replace: 885 files, 138,069,198 source bytes, 39,159,764 compressed bytes. Every archive member and unchanged original was verified; originals remain. The sibling manifest SHA-256 is `f31546401fc7fc620586759e1fb004aeb8dbb20b42ccc491e2ee900ad08b1f1a`.

These are local Chromium/mock-fixture observations, not real-phone or universal provider certification. Bounded browser retention does not establish constant SQL scan work. The real-provider pilot has not run at this checkpoint, and no main runtime, database, configuration or credentials were changed.
