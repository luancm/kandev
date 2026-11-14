---
id: "04-acceptance"
title: "Prove ordinary compatibility and measured bounded history"
status: in_progress
wave: 5
depends_on:
  - "03-demand-ui"
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.1
  - AC-TASKS-ERRAND-HISTORY-001.2
  - AC-TASKS-ERRAND-HISTORY-001.3
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.5
  - AC-TASKS-ERRAND-HISTORY-001.6
  - AC-TASKS-ERRAND-HISTORY-001.7
  - AC-TASKS-ERRAND-HISTORY-001.8
  - AC-TASKS-ERRAND-HISTORY-001.9
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Prove ordinary compatibility and measured bounded history

## Scope, implementation acceptance and exclusions

Own proposed `apps/web/e2e/tests/session/errand-history-loading.spec.ts` and `mobile-errand-history-loading.spec.ts`, focused fixture additions, package Results and public docs `docs/public/coordination.md` / `agent-communication.md` as required by docs-maintainer. Use the production build served by isolated backend; acquire shared heavy-check lock and run browser projects sequentially, one worker. Use existing suite mechanics, no arbitrary sleep or main-instance seeding.

Matrix on desktop and native mobile: zero children; normal siblings plus 100/1000 finished; direct ordinary URL; child URL with parent/missing parent/flag off; normal/maximized saved layouts with unknown identity; reload/reconnect/task switch; no history reads until expansion; page/load-more/retry/end/cache eviction; live queue/attention/pending-result and Stop all across unloaded rows; stale response/delete/continuation races; saved-card reload with zero child reads; parent Back anchor and ordinary subscription lifetime. Render screenshots prove disclosure states, count/range, focus, 44px controls, viewport/safe-area/footer containment and single phone scroll owner.

Repeat task00's fixed environment protocol and raw sample count, report pre/post bytes, rows, requests, mount counts, usable-parent median/p95 and retained heap/subscriptions against frozen numeric tolerances. Deterministic acceptance: zero finished rows in ordinary boot/list/reconciliation, zero child normal-panel mounts, zero history reads on mount/reconnect, at most 26 queried history rows and 25 returned per page, at most 100 cached summaries per open surface, child-only subscriptions return to baseline after Back. Record query aggregate cost separately. Validate existing cleanup separately without attributing runtime resource savings to UI loading.

Acceptance: (1) matrix and focused rendered desktop/phone checks pass, (2) actual timing/memory samples meet predeclared tolerances alongside structural limits, (3) docs and work-order/spec traceability are reconciled with evidence; failures remain open. Exclude broad audit, publishing or Git operations.

## ASCII UI preview

UI-01: Desktop parent Errands popover, finished history explicitly opened.

```text
Errands                    1 running
Parent conversation
  Running audit          [Open] [Stop]
  Done: result pending          [Open]
v Finished (1000)
  Loading... / No finished errands
  Could not load                [Retry]
  Review checklist              [Open]
  Showing 1-25 of 1000     [Load more]
                         [Stop all]
```

UI-02: Phone Errands pill -> inset drawer -> full-screen child.

```text
+-----------------------------+
| Errands                 [X] | fixed header
| 1 running, 1 result pending |
| Audit         [Open] [Stop] |
| v Finished (1000)           | one internal scroller
| Review checklist     [Open] |
|                     [More]  |
| [Stop all]                  | fixed safe-area footer
+-----------------------------+
          Open ->
+-----------------------------+
| [Back] Review checklist     |
| Child transcript            | full-screen reader
| ...                         |
+-----------------------------+
```

Loading, empty and error are alternative history-body states; retry retains live actions. At the 100-summary cache bound show the retained range and Start again; at end show an end marker. A changed chain offers Refresh rather than silently appending stale pages. Labels are illustrative localized copy, spacing is not a pixel requirement. Required structure maps to AC-TASKS-ERRAND-HISTORY-001.3/.4/.7; task04 proves desktop geometry, phone scroller/44px controls, focus, safe areas and Back anchor restoration.

## Verification

Baseline review disposition: the frozen numeric caps describe successful-run distributions. Preserve the separately reported original phone timeout; do not present the baseline as failure-free or relax its caps. Use the common protocol corrected before production changes, including logical history traversal independent of simultaneous retained rows. Keep existing ordinary-session suites and mixed-task variants as compatibility gates; recorded subscription IDs alone do not prove subscription lifetime.

Run from repository root after implementation authorization; commands are future checks, not design evidence. Heavy checks require the shared lock, GOMAXPROCS=2 and no overlapping browser runs. Task00 must record the exact chosen isolated seeding/measurement invocation before its gate can pass. New task04 E2E files are proposed, not present yet. Also run changed-Go lint with explicit changed Go paths and exact-file web lint for files edited by this work order, following scoped AGENTS.md.

```bash
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errand-history-loading.spec.ts -- --workers=1 --retries=0
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errand-history-loading.spec.ts -- --workers=1 --retries=0
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Ordinary-task control suites

These existing controls protect behavior outside Errands. Extend representative cases with zero-Errand and mixed-task fixtures where task02 tests do not already cover the same boundary. Preserve tests that assert complete ordinary tabs alongside visible-only rich streams, ordinary reopen/numbering, direct selection, sessionless switching, saved geometry and maximized layouts. New history tests supplement these controls; they do not substitute for them.

```bash
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/multi-session-ux.spec.ts tests/session/visible-session-demand.spec.ts tests/task/journey-boot-loading.spec.ts tests/task/task-switch-efficiency.spec.ts tests/layout/saved-layout-session-isolation.spec.ts tests/layout/saved-layout-arrangement.spec.ts tests/layout/dockview-owner-release.spec.ts tests/task/session-isolation.spec.ts tests/task/sessionless-task-switch.spec.ts tests/task/sessionless-switch-panel-leak.spec.ts -- --retries=0
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-visible-session-demand.spec.ts tests/task/mobile-journey-boot-loading.spec.ts tests/task/mobile-task-switch-efficiency.spec.ts -- --retries=0
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 KANDEV_HISTORY_MEASUREMENT_PHASE=candidate scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errand-history-baseline.spec.ts -- --repeat-each=7 --retries=0 --output=/tmp/kandev-history-candidate-desktop
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 KANDEV_HISTORY_MEASUREMENT_PHASE=candidate scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errand-history-baseline.spec.ts -- --repeat-each=7 --retries=0 --output=/tmp/kandev-history-candidate-phone
```

The two measurement specs are created by task00; their candidate mode only labels evidence. Compare raw output with the fixed task00 budgets and retain both manifests. No new production instrumentation export is assumed.

## Results

Parent acceptance work started after Task03 remediation. Existing demand-history desktop/phone checks pass; the candidate performance and ordinary compatibility matrix is running sequentially. The shared baseline helper now accepts both the old completed-toggle test ID and the candidate parent-specific history-toggle ID. This changes selector compatibility only: workload, timing endpoints, child identities, GC points and frozen budgets remain unchanged. No performance acceptance or delivery is claimed.


### First candidate measurement checkpoint

Desktop completed 21/21 browser scenarios against one unchanged backend/fixture/web build (`/tmp/kandev-run.e2e.sVHldNvn.log`, raw samples `/tmp/kandev-history-candidate-desktop`). These scenario passes are not performance acceptance: zero-history retained-heap p95 is 68,111,980 bytes against 64,345,340; 1000-history cold-reload p95 is 4,746.91 ms against 3,828.61 ms. All five 100-history metrics pass their frozen caps. Ordinary membership stays three rows; the maximum retained history is 100 summaries and all measured child subscriptions are released after Back. The phone cohort is running against the same artifacts. Preserve all samples, including the regressions, before any remediation or rerun. No tolerance changes are authorized or made.

The additional desktop/phone retry-refresh specs and affected existing Errands selectors now follow parent-specific history disclosures. Reconnect and settlement ordering tests explicitly refresh stale chains, as required by the approved design. Those new/adapted tests remain unverified until their queued static checks and managed runs finish. The public coordination how-to explains explicit history loading, range/retry/refresh, and unchanged ordinary conversations; its 55-page validator and 63-test validator suite pass.


The complete first candidate produced 42/42 scenario passes, with five failed frozen budgets recorded in [candidate-results.json](candidate-results.json). The parent reproduced and fixed two additional state-repair cases: stale running events against newer retained history and missed terminal events corrected by an authoritative compact summary (RED 2 failed/4 passed; GREEN focused suite 179/179). Explicit complete-list compatibility inventories are excluded from operational pruning.

A temporary deferred history-component import was measured and removed: zero-history heap remained 64,508,488 desktop / 50,376,596 phone, above their caps and without a meaningful improvement over the eager candidate. It is not part of the final implementation. During that experiment the rebuilt desktop and phone smoke sets passed 3/3 each, including new real-browser failure/retry/refresh/reload controls (`/tmp/kandev-run.e2e.mOtWOycy.log`, `/tmp/kandev-run.e2e.sV2FeoeD.log`). These are limited experiment receipts; the final eager build still needs its combined compatibility runs. Frozen tolerances remain unchanged.


### Ordinary compatibility and recovered-child remediation

The eager candidate passed 31/31 desktop ordinary/demand-history controls (`/tmp/kandev-run.e2e.R7kAT9pO.log`) and 7/7 phone controls (`/tmp/kandev-run.e2e.ELlDc9zK.log`). Additional existing Errands scenarios exposed compatibility gaps that those suites did not cover. The parent corrected explicit child selection in Kanban preview and Threads without restoring children to ordinary membership; a known compact-history child now invalidates history on session-only events without materializing a full normal row. A page response rejected after its observation became stale now ends its loading state and retains explicit Refresh. The ordering browser helper exercises another explicit refresh when a settlement invalidates an in-flight refresh; it does not accept stale rows or automatically fetch in production. Desktop Stop geometry waits for finite animations before measurement.

RED evidence: preview/session-event regressions failed in `/tmp/kandev-history-compat-red.log`; Threads selection failed in `/tmp/kandev-history-threads-red.log`; stale-response loading failed in `/tmp/kandev-history-stale-page-red.log`. Focused GREEN suites passed 206 tests and then 127 tests, with typecheck and scoped lint passing (`/tmp/kandev-history-compat-*`, `/tmp/kandev-history-final-fixes-*`). The first rebuilt seven-case desktop run passed four and retained three failures (`/tmp/kandev-run.e2e.qb729lgs.log`). After the additional Threads and stale-loading fixes, all three failed scenarios passed against a fresh build (`/tmp/kandev-run.e2e.gXXr5kyi.log`, 3/3). The phone batch recorded 10 passes before external termination (exit 143, `/tmp/kandev-run.e2e.YoT61URS.log`); its remaining normal-context case passed separately 1/1 (`/tmp/kandev-run.e2e.j2J6O0wI.log`). No test/fixture process remained after termination, but its cause is unproven. The phone history screenshot was inspected: bounded inset drawer, internal scrolling, Load more and fixed Stop-all remain contained. These are passing individual scenario receipts, not a completed 11-case batch. No performance acceptance follows from these functional receipts.

An isolated, temporary backend diagnostic measured ordinary-row reads below 1 ms while the complete selected-task boot path spent hundreds of milliseconds in existing unread and sidebar projections. Its profile is diagnostic evidence, not proof of the candidate timing regression's cause. The temporary test source was removed; no live instance or database was changed. Frozen performance caps remain unchanged and the first candidate's five failures remain open. No commit or push has occurred.


A second, temporary experiment deferred the entire management panel module until disclosure. Its 26 focused component tests and rebuilt desktop zero-history/retry controls passed (2/2, `/tmp/kandev-run.e2e.XYk2TunZ.log`), but the zero-history retained heap was 64,476,600 bytes, still above the frozen 64,345,340-byte cap and essentially unchanged from the eager candidate. The experiment was removed. The detached temporary control checkout at the original source commit completed its diagnostic measurements using the common test-only harness. These controls do not replace the frozen budgets.


### Current acceptance boundary

Functional remediation is complete for the exercised paths. The final combined component run recorded 310 passes and one failure in the strengthened Threads tab-navigation test. That test incorrectly sent click rather than the mouse-down used by the existing tab tests. After correcting the event, the focused test passes; no additional production change was needed. Typecheck and scoped lint pass. Exact receipts and the outstanding gate are summarized in [task-04-compatibility-handoff.md](task-04-compatibility-handoff.md).

Fresh original-build controls are retained in [baseline-control-results.json](baseline-control-results.json): two desktop samples and seven phone zero-history samples. Desktop zero-history heap is 66,933,600 bytes against the original 64,345,340-byte cap; the 1000-history reload is 6,919.16 ms against 3,828.61 ms. Phone zero-history heap ranges from 50,071,960 to 50,202,144 bytes (median 50,173,668), all above 49,778,260. The control production source is the original commit; frontend and fixture artifact hashes match the original baseline. The backend binary hash differs, so identical complete build artifacts are not claimed. These observations demonstrate that the frozen limits also fail on current original-build controls; they do not establish that the candidate has no regression.

The parent requested the user's decision between retaining the original absolute limits and using a fresh paired baseline/candidate comparison for acceptance. That decision remains pending. All original caps, raw samples and five candidate failures remain unchanged. Task04 stays in progress. No commit or push was made. All owned verification processes ended; approximately 20 GiB remains free. The temporary control checkout is retained for possible paired reuse. Main-worktree artifacts were last built during the removed panel-import experiment: any subsequent candidate browser run must rebuild the restored eager source before using those artifacts.


The user authorized the fresh paired comparison on 2026-11-14. The [paired protocol](paired-performance-protocol.md) fixes interleaving, samples, unchanged threshold formula and structural gates before collection. Earlier references to a pending user decision are historical; Task04 remains in progress until this matrix passes.


### Paired comparison and full-detail retention remediation

The authorized paired matrix completed 84/84 scenarios across 28 sequential managed runs. All production-source entries and each build’s artifact/browser identities remained stable. Twenty-five of thirty paired limits pass; [the paired handoff](paired-performance-handoff.md) records the five failures without changing original caps. The candidate materially reduces large-history payload and heap, but warm-switch, zero-history phone heap and one phone cold-navigation gate remain open.

The parent then extended retention coverage beyond compact rows/subscriptions. Three RED store tests showed that 100 distinct inspected children left 100 full session rows and that background state events independently materialized child rows. A separate real-handler RED test showed 100 context entries. Remediation evicts unowned child session/transcript/turn/runtime caches while preserving ordinary sessions and shared environments; a layout-phase reader lease protects inspection handoff and late inspection responses remain fenced. Unseen Errand state events still invalidate browser observations and retain failure/Office notification behavior without creating detail caches.

Focused GREEN receipts: 115 tests in six files, then 95 tests in four handler/ownership files; typecheck passes. A fresh managed desktop run passed 4/4 (`/tmp/kandev-run.e2e.shx7VfIZ.log`), including repeated child inspection, bounded history, ordinary tabs and real recovery refusal. Phone on the same build passed 3/3 (`/tmp/kandev-run.e2e.YDaaNh35.log`), including repeated child inspection and both Threads navigation cases. Both platforms measured zero retained child session rows, transcripts and turn caches after Back; [retention-browser-results.json](retention-browser-results.json) preserves hashes and artifacts. The function-length warning was resolved by extracting the existing starting-configuration invalidation; its 92-test handler/retention rerun and scoped lint pass. No commit or push. CPU profiling of the remaining warm-switch issue is diagnostic work, not replacement performance evidence.


The final deferred-fetch RED demonstrated loading metadata reappearing after reader release. Its fix passes the 355-test combined slice/hook/handler suite, typecheck and full web lint. [The retention handoff](retention-handoff.md) records exact boundaries, source fingerprints and browser evidence limitations. The separate 28-sample zero-history diagnostic preserves all samples and still fails three of ten limits; it does not replace the first paired matrix or close Task04. The fixed-memory allowance question is pending, with strict thresholds unchanged. HTTP route timing is under focused diagnosis.

### Parent status projection performance correction

The paired candidate measurements exposed history-dependent work in ordinary session serialization. An isolated CPU profile traced it to `ParentErrandState`, which lists finished children and calls `ProjectSnapshot` for each child to compute existence and outstanding-work booleans. A 1000-child ordinary HTTP read took approximately 378–411 ms despite returning only 2250 bytes. Replace that production read with one scalar repository query reusing the authoritative recovery/outstanding predicate; preserve feature-off behavior, parent scoping, pending results and recovery uncertainty. Verify service dispatch avoids child materialization, differential SQLite/PostgreSQL semantics and persistence checks, then rerun performance acceptance without relaxing frozen limits. This correction implements the existing ordinary-loading boundary; it adds no lifecycle transition or new recovery authority.

### Second paired cohort: acceptance still open

The complete second cohort passed 84/84 scenarios with no retry/skip and unchanged production source. All structural bounds pass; 28/30 strict numeric limits pass. Every 100/1000-history limit and every separate no-Errand warm-switch control passes. Remaining gates are low-history desktop cold start (2540.976 ms versus 2484.660 ms limit) and low-history phone retained heap (50,357,064 bytes versus 50,262,616 limit). Low-history here means zero finished children plus two outstanding controls. Full evidence: [paired-performance-v2-handoff.md](paired-performance-v2-handoff.md) and [complete metrics](paired-performance-v2-results.json). No threshold relaxation or sample replacement is authorized; the optional fixed-memory tolerance question remains unanswered. Work order stays in progress. The bounded cold-start diagnosis completed without isolating another actionable defect: generic rendering/GC time increased, while both builds recorded 117 commits in the separate render-count check. Temporary instrumentation is restored, no owned process remains, and no new production edit, commit or push is claimed.

## Upstream-main comparison requested 2026-11-14

The user requested a pure ordinary-task comparison against main because the earlier baseline already contains Errands. Execute the [frozen main comparison protocol](main-comparison-protocol.md) using matched and latest main plus candidate flag-off/on. Keep existing gates unchanged and report the full feature-branch overhead separately from history-growth savings. Comparison complete: 56/56 scenarios passed, source/build identities stayed stable, and [the handoff](main-comparison-handoff.md) records fixed heap and navigation-tail overhead. This is measurement completion, not performance acceptance; existing strict gates remain unchanged.
