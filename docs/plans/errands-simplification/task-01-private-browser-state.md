---
id: 01-private-browser-state
title: Remove unused private Errands inventory state
status: done
wave: 1
depends_on: []
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
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Task01: remove unused private inventory state

Remove `setTaskErrands`, `ErrandsListObservation` and setter-only helpers; the five obsolete `loaded*`/`liveRevision*` maps; and unused inventory selectors `selectTaskErrands`, `selectErrandsByParent`, `selectParentErrands`, `selectLiveErrandCount`. Remove their initialization, reset and deletion bookkeeping. Simplify operational upsert/pruning only where it serves the removed complete-list path.

Own `apps/web/lib/state/slices/errands/`, the affected bookkeeping in `session/session-slice.ts`, `lib/state/default-state.ts`, and their direct tests. The associated WS tests may change their setup and assertions to use bounded or operational consumers. Update this work order's Results; coordinate any unexpected file before editing it. Do not edit other roadmap/docs files while another writer owns them.

Keep `listErrands` and both API suites, all backend/MCP inventories, `byTaskId`, operational identity/title/objective lookups, live-event timestamp validation, tombstones/reset epochs, browser observation revisions and summary/history/inspection ownership. Keep host.store and the published plugin SDK unchanged. No component markup, product copy, navigation semantics or locale changes.

## Acceptance

1. No production or test consumer requires the deleted private loader or maps. Retained code has one bounded browser loader, without a replacement compatibility wrapper.
2. Stale/equal/malformed-timestamp events, deletion during an in-flight bounded read, reconnect, shared-request ownership and ordinary membership retain meaningful assertions. Replace setup through `setTaskErrands` with real bounded reads or live events. A test-specific private-map assertion is not a product contract.
3. Relevant component/store/API checks and existing desktop/phone flows pass. Record actual counts and limitations, not historical receipts as current runs.

## Verification

Run the focused test command before editing and after the final change, from the repository root:

```bash
flock -x /tmp/kandev-errands-repair-build.lock pnpm --dir apps/web test lib/state/slices/errands hooks/domains/session/use-task-errands.test.ts lib/ws/handlers/errands.test.ts lib/ws/handlers/agent-session.errand-adoption.test.ts lib/state/slices/session/remove-task-session.test.ts lib/state/default-state.test.ts lib/api/domains/errand-api.test.ts lib/api/domains/errand-api.browser.test.ts components/task/chat/errands/use-errand-view-model-live.test.tsx
flock -x /tmp/kandev-errands-repair-build.lock pnpm --dir apps/web run typecheck
git diff --check
```

Run exact-file Oxlint and Prettier for changed web files. Use the managed runner, sequentially, rebuilding final source:

```bash
cd apps/web
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project chromium -- tests/session/errand-history-demand.spec.ts tests/session/errand-history-loading.spec.ts tests/session/errand-detail-retention.spec.ts
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/session/mobile-errand-history-demand.spec.ts tests/session/mobile-errand-history-loading.spec.ts tests/session/mobile-errand-detail-retention.spec.ts
```

Mobile-parity disposition: state-only removal within the existing presentation. The named phone scenarios cover its user value; no new layout or touch test is required. Existing timing/memory cohorts remain frozen; parent owns final affected performance checks.

## Results

Accepted and delivered. The following paragraphs retain the implementation checkpoints; final evidence is 100 focused tests, desktop 3/3, phone 3/3 and a no-findings review as recorded below. Baseline at `6f0053b803`: the exact focused command above passed 13 files / 116 tests; log `/tmp/kandev-task01-baseline.log`. Initial disk availability was 20 GiB. No live runtime or database operations were part of this packet.

Test audit before editing: delete the `setTaskErrands` describe block, the four inventory-selector describe blocks, and the obsolete live-revision assertion because their production APIs have no callers. Those tests protect only the removed complete-list cache (replacement, sorting/grouping, list-specific races and private revision ownership); public complete inventories remain covered by the unchanged API suites. Repair the upsert tests that seed this dead loader by loading retained summary/history/inspection state or publishing real live events, preserving timestamp, receipt, omission, attention and live-row outcomes. Replace the `removeTaskSession` in-flight legacy-list case with pending bounded summary/history completion and retain delayed-event rejection. Replace the misleading hydration list test with real browser-ownership reset assertions against `mergeInitialState`, the production hydration boundary; `createAppStore` overwrites initial Errands state. Retain bounded growth, inspection, reconnect/shared-owner, ordinary-session membership and API suites. History: `dcc3de55a2` introduced bounded browser state; `23d161bc43` and `f71b9e09d4` supplied delivery/recovery protections that the migrated oracles must retain.

Implementation is frozen for parent review. Four production files (`errands-slice.ts`, `types.ts`, `selectors.ts`, `session-slice.ts`) remove 184 net lines; seven direct tests remove 22 net lines while repairing the retained-consumer oracles. `default-state.ts` needs no edit because its Errands initialization/hydration delegates to the slice. API clients/suites, backend inventories and plugin host/SDK are unchanged. No remaining deleted-symbol references in `apps/web`.

Final scoped Vitest passes 13 files / 100 tests (116 baseline; removed only dead-loader/selector contracts, added malformed-timestamp cases and the bounded removal matrix). Typecheck, exact-file Oxlint and Prettier, and `git diff --check` pass. Logs: `/tmp/kandev-task01-{final-vitest,typecheck,lint,prettier}.log`. The initial lint run found a repeated fixture identity and an oversized test group; both were fixed without changing limits before final checks.

Changed-oracle proof: disabling timestamp rejection caused 2 intended failures; retaining hydrated history caused 1; suppressing session-deletion browser revision caused 2; suppressing the deletion tombstone caused 3 (summary, history and delayed WS resurrection). Every source was restored byte-for-byte in `finally` before production deletion. Evidence: `/tmp/kandev-task01-mutation-*.log`; pre-mutation SHA-256 `errands-slice.ts=e21ad83a27ae8d076c372472020b31269e0cf2318a2fe3fb6ff85fbf10780629`, `session-slice.ts=7f3d1906d323813f984b841ac1f91e68570c6469c3fc2b43be8145149d40d0e8`.

Final 11-file source manifest: `/tmp/kandev-task01-source.sha256`. Production SHA-256: `errands-slice.ts=6beff3e411b326ff95ed3927c179377d1aebc9fa32134972d4003c021fe34860`; `selectors.ts=48cbc19539221be69644117b74e7c606bd737c74a01007c03a5a516d2d743412`; `types.ts=897bfc1452c4b6a1cd1ebdd18852733f367d43875a808ce0297dd2c222c3a01b`; `session-slice.ts=65268257da0a47c861a01c094aaf4c248fe3f5d27834a0973526d0b9f6eb5259`.

Managed browser acceptance passes on freshly rebuilt source: desktop 3/3 (34.3s), phone 3/3 (35.3s), covering demand history, loading/revision/error controls and repeated child Open/Back. Both retention samples report `sessionRows=0`, `transcripts=0`, `turnCaches=0` after Back. Desktop/phone demand-history screenshots were inspected. Logs: `/tmp/kandev-task01-e2e-logs/kandev-run.e2e.40pGx2en.log` (desktop) and `kandev-run.e2e.k9l4qHbf.log` (phone); screenshots and samples remain in `/tmp/kandev-task01-desktop-results/` and `/tmp/kandev-task01-phone-results/`. Runs used the managed host runner, one shard, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, the pinned pnpm wrapper, and the shared build lock sequentially. Existing owned baseline Go caches were reused; nothing was deleted.

The parent's read-only review found no concrete issues and verified all 11 hashes before/after; manifest SHA-256 is `d5d3b1159e8bf659647d85a5e4478c22a917e6f19b016ae1a907869e9b039c04`. All hashes matched again after both browser runs. Final build fingerprints: backend `c4349f167fa5d667f3f00054b7b20529bd698f904d7c026f5dc74e93b9ffa45e`, web `dist/index.html` `6ee9efe71ad66bba25a90dcfa844ff79216331339f0bdb2e5f2f4f40e8b41b34`.

All owned process handles have exited, including browser sessions 55475/79997. No process remains for either isolated E2E runtime directory; the build lock is free and disk availability is 19 GiB. Writer/build ownership is released to the parent. The parent accepted this work order on 2026-11-15 after inspecting the handoff, no-findings review, desktop/phone receipts and independently matching all 11 source hashes. No commit, staging, push, branch change, live server or live database mutation occurred. These are scoped state/browser results; parent-owned integrated performance comparisons and the separate live bounded-use pilot remain outside this work order, with their thresholds unchanged.
