# Paired performance checkpoint

All 84 scenarios completed without test failures or retries. Production source and each build’s frontend, backend, fixture and browser identities remained unchanged across its 42 samples. The paired comparison passes 25 of 30 limits; Task04 is not accepted. Full samples, hashes, round deltas and original-cap comparisons are in [paired-performance-results.json](paired-performance-results.json). The original results remain unchanged.

| Platform | Finished | Metric | Baseline p95 | Candidate p95 | Paired limit |
| --- | ---: | --- | ---: | ---: | ---: |
| desktop | 0 | warmSwitchBackToHistoryTaskMs | 885.32 | 1014.37 | 975.13 |
| desktop | 0 | warmSwitchToZeroErrandTaskMs | 1344.29 | 1445.97 | 1442.33 |
| desktop | 1000 | warmSwitchToZeroErrandTaskMs | 2471.30 | 2835.64 | 2626.58 |
| phone | 0 | postGcHeapAfterRepeatedNavigationBytes | 50200120.00 | 50351692.00 | 50230956.00 |
| phone | 100 | coldParentInputAcceptedMs | 2043.41 | 2355.02 | 2091.21 |

All candidate samples met the measured structural bounds: ordinary membership three, no finished rows in that membership, at most 100 retained history summaries, at most 25 rows per history response, no complete-list fallback and no child-rich subscriptions after Back. These assertions did not measure every full conversation cache.

After the matrix, additional regressions exposed two unbounded paths: inspection release retained full child session/transcript data, and unseen background state events materialized child detail/context entries. RED tests reproduce 100 retained rows and 100 context entries. Parent remediation passes focused store/handler/reader tests, typecheck, scoped lint and rebuilt desktop 4/4 plus phone 3/3 checks. Both platforms record zero retained child session rows, transcripts and turn caches after Back in [retention-browser-results.json](retention-browser-results.json). A subsequent handler-only extraction passes 92 tests and scoped lint; the existing matrix identifies the pre-remediation candidate and cannot certify the changed source. No new performance acceptance is claimed.

Raw managed receipts: `/tmp/kandev-history-paired/runs.json` (28 runs); source audit: `/tmp/kandev-history-paired/source-audit.json` (9,936 entries unchanged). Analysis script: `/tmp/kandev-history-paired-analysis.py`. Original control checkout remains at `/tmp/kandev-history-baseline-control`. No commit or push.


Diagnostic task-switch profiling found higher sampled renderer and GC time but no isolated new hot path. A separate React commit diagnostic recorded 54 commits in each build with essentially equal named component activity; it does not support an extra whole-page commit cycle as the explanation. Instrumentation was test-only and removed after the runs. Logs `/tmp/kandev-run.e2e.JMtJTyDj.log` and `/tmp/kandev-run.e2e.WF5yDsKO.log` each pass 1/1; diagnostic JSON remains under `/tmp/kandev-history-cpu-{baseline,candidate}`. Those directories were reused for the render diagnostic and replaced the earlier raw CPU profiles; numerical CPU observations are therefore diagnostic notes, not independently retained profile receipts. Original paired samples and build/source receipts are untouched.
