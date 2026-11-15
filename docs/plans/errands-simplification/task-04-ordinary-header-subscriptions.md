---
id: 04-ordinary-header-subscriptions
title: Bound ordinary Errand view-model subscription work
status: in_progress
wave: 4
depends_on:
  - 03-owned-await
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.1
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.6
  - AC-TASKS-ERRAND-HISTORY-001.7
  - AC-TASKS-ERRAND-HISTORY-001.9
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Task04: bound ordinary view-model subscription work

At accepted source `77b155597c`, both `ErrandHeader` and `MobileSessionPickerRow` call `useErrandViewModel` for ordinary sessions. The hook installs eleven store subscriptions plus its feature subscription before returning no model. Later selector checks run through store listeners, even though ordinary sessions need only reactive identity recognition. No notification loop or unstable snapshot was found.

The final ordinary comparison found a desktop return-navigation warning: candidate p95 1039.246 ms versus the unchanged 958.562 ms boundary. Two traces and one CPU capture per arm identified substantial shared React/store work but did not establish a comparative causal trigger. This packet removes demonstrated avoidable subscription work; it does not assume that doing so resolves the full warning. Preserve the original results, diagnostic limitations and all thresholds.

## Selected design and ownership

Replace the eleven independent view-model store reads with one internal shallow-compared snapshot, using the project's existing `useShallow` pattern. Evaluate recognition from the same current state: session role OR feature-enabled browser row OR scoped inspection, with `session.task_id` taking precedence over the task hint. Return shared empty inputs only for an unrecognized session; select child-only presentation and inspection inputs only after recognition. Keep the public hook signature and both callers unchanged. A reactive snapshot is simpler than adding recognition wrappers and ownership obligations to both components.

Keep hooks unconditional, existing owner IDs, layout/passive effects and their individual dependencies. A recognized child with incomplete task or presentation data must still retain its detail as before, even when the returned model is null. Keep stored references and primitive fields stable; do not depend on the aggregate object for effect lifetime. Reading the existing feature boolean within the snapshot may replace the convenience hook; no flag definition, default or gating contract changes.

The implementation owner may edit `apps/web/components/task/chat/errands/use-errand-view-model.ts`, its existing live tests, the header and mobile-picker tests, and a focused resource test alongside the hook. Own this work order's Results and a sibling Task04 handoff. No production caller, store, API, backend, layout, locale, performance harness or threshold changes are assigned. Coordinate an unexpected need before editing. Other checkout work and earlier evidence must remain intact. Parent owns review, final comparison, integration and delivery.

## Acceptance

1. With the real store and exported surfaces, the ordinary view-model adds at most two active store subscriptions above the surface's existing baseline. Record the pre-change count and callback work as a failing resource regression, then the final count. Unrelated updates and ordinary metadata changes preserve UI, create no child inspection/detail ownership, and unmount returns to baseline. Do not assert private helper names or exact implementation shape.
2. Preserve ordinary/absent-session and flag-on/off behavior, role-only children, enabled browser-row-only identity, scoped-inspection identity, late identity arrival without prop changes, session-task precedence, and recognized-but-incomplete child retention. Phone selection/picker visibility and desktop parent navigation remain correct.
3. Child and parent-title changes update presentation without unnecessarily reacquiring inspection. Connection and observation changes still refresh it. Retain existing delayed-response rejection, pending-detail lifetime and unmount-release assertions, including scope changes and flag-off inspection/navigation behavior.
4. Run focused component/hook tests, typecheck, changed-file lint/formatting and whitespace checks. Review frozen source before managed browser checks. Desktop and phone demand-history, ordinary-loading and repeated Open/Back controls must pass on the final build; no rendered design changes are intended, so the existing mobile interaction contract remains authoritative.
5. Parent performs a fresh matched ordinary comparison after the source change with the existing workload, formula and limits. Resource reduction alone is not performance acceptance. Preserve a remaining warning and stop for an explicit disposition rather than repeating unchanged runs.

## Execution and resources

One writer owns the shared checkout, including docs. Use TDD for the new resource/behavior oracles. Retain raw failed and passing receipts, source hashes and process handles. Use the pinned pnpm 9.15.9 wrapper and serialize heavy checks with `/tmp/kandev-errands-repair-build.lock`. Check disk before and between heavy operations; remove only proven inactive owned regenerable artifacts. Do not clean during measurements. Reuse current caches and normal managed global setup; no live runtime/database operations or provider launches.

Initial focused scope: `components/task/chat/errands/errand-header.test.tsx`, `components/task/chat/errands/use-errand-view-model-live.test.tsx`, the new resource/recognition tests and `components/task/mobile/mobile-session-picker-row.test.tsx`. Existing managed browser controls are the three desktop and three phone specs listed in Task01. Parent will authorize the browser/build phase after source review so a failed source review does not waste a rebuild.

## Results

The parent accepted the source phase on 2026-11-15 after independent no-findings review, verification of all four source hashes and all 26 evidence-file hashes, and explicit writer/process release. Browser and performance acceptance remain pending, so this work order stays in progress. Normal-hook checkpoint delivery precedes the fresh final-build controls; no timing improvement is claimed from the resource count alone.

Source implementation and scoped review are complete on 2026-11-15; status remains `in_progress` for the separate browser/performance acceptance. The hook now selects one shallow-compared snapshot from current state, including the feature flag, and shares empty inputs only while unrecognized. Child recognition, task precedence and incomplete-child retention remain covered. The inspection-owner function, detail effect/dependencies and inspection invocation match the original bytes. Header and phone-picker production callers are unchanged. Independent read-only review reported no findings and confirmed all four frozen source/test hashes and the protected callers.

The real-store resource RED measured 12 added subscriptions and 12 callbacks per unrelated update for each ordinary consumer, with the flag both on and off. GREEN measures 1/1 against the unchanged budget of at most two. Existing surface baselines are 1 listener for the header and 15 for the real phone picker; unmount returns to zero. Ordinary metadata/UI and absence of child inspection ownership also pass. This proves reduced subscription work, not a latency improvement.

The initial baseline was 26 passed / 7 failed because the mobile-picker tests had an incomplete store mock. All seven original cases/assertions were retained and passed against unchanged production after repairing their fixtures to use the real store and bounded summary publication. The existing picker presentation stub remains in that suite; the resource suite exercises the actual picker. Four resource cases and ten live recognition/lifetime cases were added; no tests were removed. Final scope: **47/47** (23 header, 13 live, 7 mobile, 4 resource), plus clean web typecheck, exact-file lint, Prettier check and whitespace check. Intermediate fixture/static failures are retained separately from the valid resource RED.

Raw commands, complete logs, counts, source hashes and limitations are in the [Task04 source handoff](ordinary-header-subscriptions-handoff.md) and `/tmp/kandev-task04-subscriptions.xd_z_4rm/`. All check handles are terminal, the build lock is free, and no check process remains. Checkout/process ownership is released for the parent checkpoint; no staging, commit, push, build, browser run or live operation was performed. Free disk at release: 9,954,062,336 bytes (9.27 GiB).

Parent must checkpoint the accepted source, rebuild, run the existing desktop/phone controls, and perform the unchanged matched comparison. The original desktop warning (1039.246 ms p95 versus 958.562 ms boundary), historical numeric misses, fixed-heap exception and diagnostic attribution limits remain open/unchanged. The separate pilot still waits for performance disposition.
