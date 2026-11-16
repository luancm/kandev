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

Accepted and delivered. Removed 184 net production lines from four private-state files and repaired/pruned seven direct tests against retained browser contracts. Complete public/API inventories, plugin host/SDK and operational live rows remain. [The exact completed Results](evidence-index.md#task-01-private-browser-state) retain commands, all source/artifact hashes and intermediate receipts.

Baseline passed 116 tests; final scoped verification passed 100 tests across 13 files, typecheck, exact-file lint/formatting and whitespace checks. Initial lint fixture/size findings were corrected without changing limits. Changed-oracle probes produced the intended timestamp (2), hydrated-history (1), deletion-revision (2) and tombstone/resurrection (3) failures; each source was restored byte-for-byte before the production deletion.

Fresh managed desktop 3/3 and phone 3/3 passed demand history, loading/revision/error and repeated Open/Back; child session rows, transcripts and turn caches returned to zero. Screenshots were inspected and all 11 source hashes matched before/after. Independent review found no issues; the parent accepted the packet. Scoped mock-browser evidence does not certify provider behavior or approve the separate performance gate. The [completed pilot](../errands-remaining/bounded-pilot-receipt.md) and [package plan](plan.md) own later outcomes.
