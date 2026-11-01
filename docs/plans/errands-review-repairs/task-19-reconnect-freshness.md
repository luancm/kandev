---
id: "19-reconnect-freshness"
title: "Refresh each task across connection generations"
status: in_progress
wave: 19
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.11
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 19: Refresh each task across connection generations

## Scope and authority

Repairs review finding(s) 13 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Partial production and regression changes survived the interrupted implementation. This packet resumes their acceptance; the later Task29 integration has already repaired the required connection.generation test-fixture shapes.

## Low-level implementation and regression plan

### Root cause / owned files

use-task-errands.ts remembers previousConnection in a hook-local ref, while loadedByTaskId means loaded forever. A reconnect while B is selected refreshes B only; visiting A sees loaded:true and never refreshes it. Its loadsByStore entry is also only task-keyed, so a still-pending request from the old transport can swallow the new reconnect request and later write an obsolete snapshot.

Own:
- apps/web/lib/state/slices/ui/{types.ts,ui-slice.ts}, plus a new connection-generation.test.ts beside the slice.
- apps/web/lib/state/slices/errands/{types.ts,errands-slice.ts} and errands-slice.test.ts.
- apps/web/hooks/domains/session/use-task-errands.ts and use-task-errands.test.ts.
- apps/web/lib/state/default-state.ts only if default merging needs explicit compatibility handling for new optional maps. No WS client mutation is required: client.ts already rejects callbacks from replaced sockets using socket identity.
- Use existing stateReadScopeIdentity/storeReadScopeIdentity from shared-resource-reads.ts without changing that generic helper.

### Exact private state and ownership semantics

1. Add connection.generation, initialized 0. In setConnectionStatus increment synchronously only on a real transition where previous status is connected OR new status is connected. Thus disconnected(0) -> connected(1) -> disconnected(2) -> connecting(2) -> connected(3). Duplicate connected callbacks do not increment. Both leaving a connection and entering a new one invalidate old requests, including when no Errands hook is mounted.
2. Add task snapshot freshness maps under errands: loadedConnectionGenerationByTaskId and loadedReadScopeByTaskId. loadedByTaskId keeps its existing public meaning, 'some snapshot was applied', so cached rows can remain visible while refresh runs. New maps are runtime-only, never boot/persisted wire data. Treat absent legacy generation as not current.
3. Preserve one in-flight entry per store/task/current scope+generation, with an immutable attempt object containing promise, captured read-scope identity, connection generation, requestRevision and task reset epoch. Replace (do not join) a pending entry when generation/scope differs.
4. Add resetEpochByTaskId to the errands slice, increment on removeTaskErrands, clear data/freshness maps and do not reset the epoch to zero. Capture it in the attempt and compare before every write so task removal invalidates already admitted reads. This is a private reset fence; existing per-session removed-ID tombstones still prevent late row revival. The integrating session must serialize changes with any other repair of this same action, retaining one reset authority.
5. Hook effect depends on enabled/task ID, connection status+generation, store and read-scope identity. It runs only while enabled, scoped task exists and status is connected. Load if no snapshot OR successful snapshot identity/generation is stale. Consumers mounting in the same tick coalesce on the same current entry. Merely opening a panel never causes a second read if current.
6. Before response application check: feature still on, status connected, connection generation unchanged, read scope unchanged, task reset epoch unchanged, and this exact attempt is still the map owner. Apply rows through existing setTaskErrands with captured liveRevision and a new optional fourth ErrandsListObservation argument {connectionGeneration,readScopeIdentity,taskResetEpoch}, then stamp successful freshness in the same slice transaction. Three-argument internal/test callers remain compatible but do not claim a transport-fresh snapshot; only the owned loader passes the observation. Recheck the observation at the slice action entry before writing. Preserve timestamp ordering, live-race merge and removed-session filters.
7. A stale success or stale failure is a no-op. finally deletes map entry only if it still equals this attempt; an old promise must not clear its replacement. On failure retain data and old freshness; do not mark refreshed or start an effect-driven retry loop. Existing reconnect/remount retries remain.
8. A consumer unmount/task switch does not own or cancel a store-owned admitted request. It may complete for the original task if its scope/generation remains valid; it cannot write another task. Latest admitted same-key attempt wins. A different store is always isolated.
9. loaded/read freshness cannot be reset by a second consumer. If feature off, expose empty values and send no request; a deferred enabled response must not repopulate displayed Errands after disabling.

Do not replace this with clearing all errand rows on disconnect, wall-clock freshness, request-per-row, or a timer. No API/WS/DB schema change; TaskErrands return shape can remain unchanged.

### Permanent hook/store regressions

Use real StateProvider/createAppStore and actual useTaskErrands, real slice, deferred listErrands network boundary only. These are stateful-hook tests permitted by TDD, not component markup tests.

- refreshes cached task A after reconnect occurred while task B was selected (promote diagnostic exactly).
- refreshes a cached task after reconnect while every consumer was unmounted.
- reconnect starts a fresh generation while an old list is unresolved; resolve new first then old, ensure final rows/freshness stay new and old finally cannot delete the new request.
- disconnection before resolve cannot stamp freshness; connect retries; failure of refresh leaves cached rows and permits next connect to repair.
- two consumers share one request; initiating consumer unmount does not prevent successful terminal publication to the remaining consumer.
- live upsert during current list survives; stale list cannot roll back newer row; removal/reset during list prevents resurrection.
- second store, scope/workspace/auth switch and task switch cannot consume old-attempt result. Scope identity changes at the same production store boundary used by task-session reads.
- connection generation increments only at actual connected boundaries, independent of mounted hooks; duplicate statuses stay stable.

~~~
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-errands.test.ts lib/state/slices/errands/errands-slice.test.ts lib/state/slices/ui/connection-generation.test.ts lib/ws/handlers/errands.test.ts lib/api/domains/errand-api.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-task-errands.ts hooks/domains/session/use-task-errands.test.ts lib/state/slices/errands/types.ts lib/state/slices/errands/errands-slice.ts lib/state/slices/errands/errands-slice.test.ts lib/state/slices/ui/types.ts lib/state/slices/ui/ui-slice.ts lib/state/slices/ui/connection-generation.test.ts)
~~~

The two new counters are private ownership/freshness metadata, not user-visible statuses. Add any changed default-state file to ESLint. Existing mock state shapes need generation:0 or nullish-compatible handling; run rg for full-module useTaskErrands mocks before final suite.

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

The reconnect regressions were run against the committed hook at HEAD `6400a265bb` before relying on the partial implementation. The A-to-B reconnect case failed behaviorally (expected three list calls, observed two); the actual zero-consumer reconnect and auth-scope cases each failed because no replacement read began. Logs: `/tmp/errands-task19-red-baseline.log` and `/tmp/errands-task19-red-matrix.log`.

The hydration regression failed before normalization because `mergeInitialState` retained runtime-only generation, scope and reset maps from the incoming state. The Errands slice now merges cache rows while resetting those maps. RED: `/tmp/errands-task19-red-hydration.log`; isolated GREEN: `/tmp/errands-task19-green-hydration.log`.

Final regression command, including every required work-order test and the changed default-state, UI-slice and nearest consumer suites:

~~~sh
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-errands.test.ts lib/state/slices/errands/errands-slice.test.ts lib/state/slices/ui/connection-generation.test.ts lib/state/slices/ui/ui-slice.test.ts lib/state/default-state.test.ts lib/ws/handlers/errands.test.ts lib/api/domains/errand-api.test.ts components/task/chat/errands/errands-chip.test.tsx components/task/mobile/mobile-sessions-section.errands.test.tsx components/task/mobile/mobile-sessions-section.test.tsx --maxWorkers=1)
~~~

Result: 10 files and 147 tests passed; `/tmp/errands-task19-final-vitest-3.log`.

Scoped ESLint passed with `--max-warnings 0` for every changed Task19 source and test file; `/tmp/errands-task19-final-eslint.log`. Prettier check passed for those files; `/tmp/errands-task19-final-prettier.log`. Full web typecheck passed with the workspace's 8 GiB Node heap; `/tmp/errands-task19-typecheck.log`.

No browser suite was rerun: this packet changes only store freshness and hydration metadata, with no markup, layout, navigation or input behavior. Existing desktop and native-phone Errands evidence remains applicable.

### Reopened by the scope review

The user-requested scope review found a remaining stale-response race in the existing live/list merge contract. `setTaskErrands` uses a changed `liveRevisionByTaskId` to preserve removals, but still replaces an existing row when the incoming timestamp is equal. Await receipt/recovery projection updates `errand_await_attempt` without advancing the errand row's `updated_at`, so a delayed list can overwrite a later live `received` observation with `uncertain`, or hide a newly published warning, then stamp the cache fresh. The previous test receipt remains valid for its covered cases but does not establish this schedule.

Task19 is reopened for a focused regression in the existing slice and `use-task-errands` tests: capture a list request, publish a delivery-state change with identical `updated_at`, then resolve the older response last. Cover warning introduction/removal and an unrelated row update. Preserve the newer live observation with per-row request/event ownership or an equally complete existing authority; do not use the unchanged errand timestamp as projection ordering. No new public API, UI layout or browser behavior is proposed. Run the existing Task19 focused command after correction. This checkpoint records the finding only; no additional production repair was made during the scope review.
