---
id: "00-baseline"
title: "Capture reproducible baseline before production edits"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.9
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Capture reproducible baseline before production edits

## Scope, implementation acceptance and exclusions

Own test-only measurement fixtures and this work order's Results. Proposed new harness files are `apps/web/e2e/tests/session/errand-history-baseline.spec.ts` and `mobile-errand-history-baseline.spec.ts`, with a shared helper under `e2e/helpers/`. They must run unchanged on the baseline and candidate production builds. `KANDEV_HISTORY_MEASUREMENT_PHASE` below is a proposed harness input, not an existing product option; use it only to label captured attachments, never to vary production behavior. Start from unchanged production source, identify exact SHA/build and ordinary task/session structure. Seed zero/100/1000 finished delivered children in an isolated instance; hold ordinary siblings, active counts, payload shapes, viewport, network, CPU and cache policy fixed. Include one terminal-pending and recovery case separately so finished-size scaling is not confused with outstanding growth. Reuse `apps/web/e2e/helpers/errands.ts`, `e2e/tests/session/errands.spec.ts` and mobile equivalents as fixture patterns; no live runtime or DB operations are authorized by this design package.

Freeze the history workload as every finished identity in deterministic 25-row logical ranges, in fixture order, followed by restoring the first range and opening/Back-navigating the same first five children. A baseline full-list response satisfies these ranges from its already-rendered identities; a candidate must use the visible Load more/Next page control for each additional range and Start again when the bounded cache has evicted the first range. Record each range and identity visited separately from finished rows retained in the panel/store. Observe legacy `errand.list`, new history-page frames and session-store row counts, but do not wait for or assert a particular eager-list response or row count. Across cold navigation, reload, task switch and saved-layout restore, assert exact ordinary session identities. `KANDEV_HISTORY_MEASUREMENT_PHASE` remains a label only.

Record boot/list bytes and request callers, SQL row materialization/enrichment counts, ordinary reconciliation rows/time, restored child panel mount/removal count including delayed identity and maximize, usable-parent latency (composer enabled and accepts a test input), retained child/ordinary subscriptions, and post-GC heap where available after repeated Open/Back and task switching. Record browser/process measurement limitations explicitly. At least seven runs per condition, separate cold/warm samples; retain raw data with environment manifest and deterministic seed recipe. Freeze numeric median/p95 latency and retained-memory regression tolerances derived from baseline spread BEFORE production edits; record exact values here. Missing heap access needs a supported measurement method before claiming memory acceptance. No invented universal millisecond threshold.

Acceptance: (1) another session can reproduce seeds and raw baseline, (2) numeric timing/memory budgets and measurement definitions are recorded before task01, (3) no performance fix is claimed from HAR estimates. Exclude production optimization and generic benchmarking of settings/assets.

## Verification

Run from repository root after implementation authorization; commands are verification evidence, not design evidence. Heavy checks require the shared lock, `GOMAXPROCS=2`, `GOFLAGS=-p=1`, `GOMEMLIMIT=3GiB`, one worker/shard and no overlapping browser runs. The original full matrices are historical baseline evidence and need not be repeated for this protocol correction; use the same helper unchanged for a candidate, and supplement only affected heap cohorts if the measured workload or GC endpoint changes. Exact-file web lint, formatting, typecheck, E2E sleep ratchet and representative managed desktop/phone baseline smoke are required for this correction. Do not treat Task01/02 SQL or controlled reconciliation evidence as part of Task00.

```bash
git rev-parse HEAD
df -h . /tmp
corepack pnpm@9.15.9 --dir apps exec oxlint -c oxlint.config.mjs --deny-warnings web/e2e/helpers/errand-history-baseline.ts
corepack pnpm@9.15.9 --dir apps exec prettier --check web/e2e/helpers/errand-history-baseline.ts ../docs/plans/errand-history-loading/task-00-baseline.md ../docs/plans/errand-history-loading/baseline-handoff.md
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB corepack pnpm@9.15.9 --dir apps/web run typecheck
corepack pnpm@9.15.9 --dir apps/web run lint:e2e-sleeps
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline-corrected scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errand-history-baseline.spec.ts -- --grep '1000 finished Errands' --retries=0 --output=/tmp/kandev-history-correction-final-desktop-1000
flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB GOCACHE=/tmp/kandev-history-baseline-gocache GOMODCACHE=/tmp/kandev-history-baseline-gomodcache GOLANGCI_LINT_CACHE=/tmp/kandev-history-baseline-golangci-cache PATH=/tmp/kandev-history-baseline-bin:$PATH KANDEV_HISTORY_MEASUREMENT_PHASE=baseline-corrected scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errand-history-baseline.spec.ts -- --grep '1000 finished Errands' --retries=0 --output=/tmp/kandev-history-correction-final-phone-1000
python3 scripts/list-docs.py validate
```

## Results

Parent accepted Task00 on 2026-11-13 after Era Sol verified the original cohorts and all 30 cap calculations, and Lunaldinho corrected the common harness. Parent inspected the eager-assumption removal, logical-range traversal, ordinary identity restore and nested cleanup, verified helper SHA-256 `ef7d21f3ea03b0dfadb60c24e40c7cb764cca455e32643ac42fc35054a870f64`, and read both managed 1000-history smoke receipts (desktop 1/1, phone 1/1). The original caps remain frozen. Extra range observations do not mutate the baseline application state; its resident history, five child loops and forced-GC endpoint remain comparable, so no supplemental heap cohort is required. The original phone timeout stays reported separately. SQL materialization/enrichment, deterministic reconciliation bounds and controlled identity races are explicit Task01/02 obligations; broad renderer timing does not establish isolated reconciliation timing. This accepts the baseline gate, not candidate performance or product implementation.

Implementation explicitly authorized on 2026-11-13. The test-only baseline harness and isolated finished-history fixture are in place, and source HEAD stayed at `6e4c5ca9bce14b952d8cbaa52dc0282af870d835` throughout measurement. Desktop completed 21/21 runs; phone completed 20/21 in the full matrix, then all seven phone 1000-history samples needed for a same-build cohort passed in follow-up runs. One phone 1000-history test timed out at 240 seconds in the original matrix and remains recorded as a failure; no runs were skipped or retried.

Captured 48 successful raw samples across desktop and phone, with at least seven same-build samples for each 0, 100, and 1000 finished-history condition. Baseline-derived p50, nearest-rank p95, MAD, tolerance, and frozen upper limits are recorded in [baseline-handoff.md](baseline-handoff.md) and [baseline-results.json](../errands-simplification/evidence-index.md#baseline-results). Freeze these limits before Task01 production edits; no speedup is claimed.

The harness uses an isolated E2E database and direct test-only repository seeding, with zero agent processes launched. Production source and public contracts were not changed. SQLite rows scanned/joined/enriched and isolated ordinary-session reconciliation time remain unavailable without a production measurement seam; the harness records browser-store row materialization, request/response bytes, renderer deltas, panel lifecycle, subscriptions, composer input acceptance, and forced-GC heap instead.

### 2026-11-13 acceptance correction

The reviewer found that the original helper blocked on complete `errand.list` responses during navigation/reload/switch, required finished children in ordinary session membership, and required `finishedCount + 2` simultaneous history rows. The helper now records those transport and row counts without making them pass conditions, verifies the exact three ordinary session identities instead, and records logical 25-ID ranges, visited IDs and rendered/retained counts separately. Saved-maximized restore waits for the ordinary identities in the store and proves the parent remains while no finished-child panel mounts; it does not require historical identities or a full-list HTTP response.

Cold navigation, reload and warm-switch latency endpoints still stop immediately after the same composer input is accepted and before history browsing; their original measurements and frozen limits remain comparable. The history phase now explicitly walks all logical ranges, returns to the first range, then performs the same five child Open/Back loops. Repeated-navigation heap is still captured after those loops and forced GC. Any new heap cohort required by this corrected browsing protocol is supplemental and must not replace or recompute the original limits in [the immutable baseline results](../errands-simplification/evidence-index.md#baseline-results).

The original phone 1000-history 240-second timeout remains an unresolved, separately reported failure. Its awaited step is still unknown because the original cleanup masked it; no successful follow-up converts that failed run into a pass. SQLite materialization/enrichment, deterministic ordinary-reconciliation bounds and controlled identity-race proof remain Task01/02 responsibilities.

The corrected helper passed one fresh managed 1000-history smoke on desktop and one on phone against unchanged production source SHA `6e4c5ca9bce14b952d8cbaa52dc0282af870d835`; both recorded all 40 logical ranges and 1000 visited identities, separately reporting 1000 retained finished rows and 1002 total panel rows on the old baseline UI. Both runs also passed composer input, exact ordinary membership, parent retention, no child panel after saved-layout identity resolution on desktop, and zero retained finished-child subscriptions. Raw smoke samples and logs are listed in [baseline-handoff.md](baseline-handoff.md); they are not added to the original seven-run numeric cohorts.

No supplemental heap cohort is needed for this correction. On the unchanged baseline, all identities were already loaded and retained before the same five child Open/Back loops; the helper's range checks do not mutate application state or retain additional browser data, and the browser heap is still forced-GC sampled after those same loops. The retained state and GC endpoint represented by the original heap cohorts therefore remain comparable. The corrected helper and protocol are ready for parent acceptance; this work order remains `in_progress` until the parent accepts its baseline gate.
