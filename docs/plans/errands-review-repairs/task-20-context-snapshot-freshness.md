---
id: "20-context-snapshot-freshness"
title: "Converge authoritative context and cache"
status: done
wave: 20
depends_on:
  - "19-reconnect-freshness"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-CONTEXT-001
acceptance_criteria:
  - AC-TASKS-ERRANDS-CONTEXT-001.1
  - AC-TASKS-ERRANDS-CONTEXT-001.2
  - AC-TASKS-ERRANDS-CONTEXT-001.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-context-window.md
---

# Task 20: Converge authoritative context and cache

## Scope and authority

Repairs review finding(s) 14 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation and acceptance are recorded in Results; curated Task25 media remains separate.

## Low-level implementation and regression plan

### Root cause

The real list setter accepts metadata.context_window:null and updates taskSessions.items, but the independent contextWindow.bySessionId entry is untouched. useSessionContextWindow returns that cache and its hydration effect exits if contextWindow exists, so neither authoritative clear nor updated snapshot can reach the row. Existing hook test manually deletes the cache before asserting clear; it masks the defect. Live WS extraction already clears the cache, which is why old live-event/reload tests pass.

Use ingestion ownership instead of adding a second hook refresh request. Context metadata is authoritative; contextWindow is a cache, not another authority.

### Exact ownership

- apps/web/lib/state/slices/session-runtime/context-window.ts: parse remains compatible; add dependency-neutral presence/merge helpers for the context_window plus context_compaction_count pair as needed.
- apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts: atomic cache publication/invalidation and existing live set/clear actions; no ContextWindowState wire/persisted-field addition is needed.
- apps/web/lib/state/slices/session/types.ts and session-slice.ts: put the private counter in taskSessions.contextWindowEpochBySession?:Record<string,number>, initialized empty, captured through the existing SessionEpochState shape. Runtime set/clear actions have the full root draft and can increment this session-owned epoch. buildRemoveTaskSessionAction purges it next to existing activity/read-cursor epochs; the runtime cache purge remains unchanged.
- apps/web/lib/state/slices/session/{types.ts,hydration-epochs.ts,session-slice.ts}: capture context epoch and accept/suppress only context fields at snapshot merges; cache synchronization in setTaskSessionsForTask and setTaskSession.
- apps/web/lib/state/hydration/hydrator.ts: same accepted context handling for route/boot hydration, including itemsByTaskId mirrors. When hydrating a task list without canonical items, synchronize each accepted row as well; do not assume only the full detail boot path can carry context.
- apps/web/lib/ws/handlers/agent-session.ts: normalize both metadata aliases and keep live updates atomic/ordered at the accepted state event boundary.
- apps/web/hooks/domains/session/use-session-context-window.ts: read cache/persisted projection without making it a competing writer.
- apps/web/lib/state/task-session-reads.ts and apps/web/hooks/use-task-sessions.ts: connection-generation ownership for existing full-session reads, described below; regression files lib/state/task-session-reads.test.ts and hooks/use-task-sessions.stale-reads.test.ts.
- Extend existing tests in hooks/domains/session/use-session-context-window.test.ts, lib/state/slices/session-runtime/context-window.test.ts and session/session-slice.upsert.test.ts; preferably put new real-store regressions in new lib/state/slices/session/session-context-window.test.ts and hydration/hydrator-context-window.test.ts to avoid oversized files.
- Existing handler regression suites lib/ws/handlers/agent-session.test.ts and session-models.test.ts exist. Add new focused lib/ws/handlers/agent-session.context-window.test.ts rather than lengthening the already broad agent-session suite.

### State/data rules and algorithm

1. Preserve three inputs: metadata omits context_window -> unknown/no-op; own property context_window:null (or non-object rejected by existing parser) -> explicit unavailable/clear; object -> parse with existing semantics and scalar compaction count. A snapshot omitting this optional field does not revoke a valid live reading. Fresh empty cache + omitted metadata yields no reading.
2. Add a monotonically increasing live context epoch per session at taskSessions.contextWindowEpochBySession (include local model/reset invalidations). Extend TaskSessionHydrationEpoch with optional contextWindow. Capture it at request start through captureTaskSessionHydrationEpoch[s], which existing useTaskSessions, route/recovery and session-state-reconciler callers already use.
3. Before mergeTaskSessionSnapshot merges context from a snapshot, compare captured/current context epoch using the existing hydrationEpochChanged rule. If a live context sample/clear landed since request start, ignore only the snapshot's context_window/context_compaction_count pair, retaining exactly the existing pair, including an explicit null or an absent property. Do not restore other stale fields or remove unrelated incoming metadata. Do not synthesize context from parent usage.
4. Also apply existing strict session updated_at stale-snapshot ordering to context acceptance. A delayed snapshot with an older authoritative row timestamp cannot replace context from a newer accepted snapshot. Do not use client Date.now as source version. Do not increment the LIVE epoch simply because a snapshot was accepted: that would make the first of two same-epoch snapshots win incorrectly. Existing shared read owner supplies latest-request ownership; updated_at supplies persisted snapshot ordering.
5. For every accepted explicit context property, update the canonical session metadata and contextWindow cache in the same store transaction. Null deletes only the cache entry and leaves explicit metadata null. Valid object updates cache even if an old cache entry exists. Metadata/cache changes and itemsByTaskId mirror must agree. All affected children are synchronized even when no row/context hook is mounted.
6. Live events arriving through metadata OR session_metadata must go through the same context publication semantics. buildSessionUpdate currently reads session_metadata only, while extractContextWindow accepts either alias. Normalize context presence from both without silently dropping metadata.context_window; preserve current alias priority when both are supplied (existing extractor prefers a present metadata context). Retain the existing stale-session-event timestamp guard before any live context write. Do not publish twice via a new ingestion helper plus old extractContextWindow: one accepted event increments the live epoch once.
7. setContextWindow/clearContextWindow from genuine live/model-reset paths update the live epoch and the in-memory canonical metadata pair too, so clearing the runtime cache cannot immediately resurrect an older metadata fallback. Clearing on model change is local invalidation; keep the server's persisted write contract unchanged. Cache-hydration from retained metadata must not count as a new live sample.
8. Hook can keep subscription effect unchanged. Its synchronous return is cached accepted context, or a parsed retained metadata fallback only when the live epoch is zero and no invalidation forbids it. An existing live epoch plus an absent cache is an invalidated reading, never permission to resurrect retained metadata. Remove its current 'set cached context if missing' writer once ingestion/boot fallback owns this, avoiding effect races across multiple row consumers. A metadata null always prevents fallback. detailActive:false never subscribes or loads.
9. Route hydration currently deep-merges session metadata. Apply context presence/order resolution before that deep merge and synchronize the cache from the ACCEPTED value afterward; never take a rejected incoming null and apply it to the cache. Protect both canonical items and task list mirror. Boot/initial StateProvider data still renders from retained metadata without requiring a child stream.
10. Full task-session read transport freshness: include connection.generation from Task 19 in getTaskSessionReads' per-store owner identity in addition to storeReadScopeIdentity. On a generation change dispose the old SharedResourceReads owner (abort old HTTP reads and invalidate isCurrent), make a new one, and extend its scopeIsCurrent closure to compare the FULL identity including current connection.generation on EVERY completion, even before a hook renders. In useTaskSessions explicitly select generation as well as status; make useTaskSessionReconnect track generation, not just previous status, so a batched connected->disconnected->connected still forces a list refresh. Existing ownership cleanup releases old loading/pending waiters; reconnect refresh retains/reloads the current task. An old attempt's finally must not clear a new owner's loading/request state. Keep task-session responses consumer-owned as existing SharedResourceReads specifies; this differs intentionally from store-owned Errands-list completion in Task 19. Do not add connection generation to generic shared-resource-reads globally. A still-active old request cannot supply the 'fresh' reconnect context snapshot.
11. Purge session removes cache and its context epoch/associated invalidation data; workspace/auth reset must not leak values to a different scope. Default merge supports older fixtures lacking counters.

No provider telemetry, repository summary projection or wire schema changes. Producer apps/backend/internal/orchestrator/{event_handlers_git.go,context_window.go} and persisted fixed-scalar projection stay intact. The Errand DTO stays unchanged. No new context fetch, polling, detail subscription or visible copy.

### Sequential the implementer-sized implementation suborders

Issue the following source leases in order; shared helper/session files are intentionally handed off sequentially. Each suborder inherits the exact omission/clear semantics, ACs, no-wire-change exclusions and failure rules above, and owns only the subset stated here.

**UI-F2a: authoritative context property/cache ingestion.** Own session-runtime/context-window.ts and session-runtime-slice.ts, session/session-slice.ts context synchronization, agent-session.ts context alias normalization, use-session-context-window.ts, and new session/session-context-window.test.ts plus existing hook/handler tests. First RED: authoritative null from setTaskSessionsForTask leaves the populated runtime cache. Add named cases `authoritative reconnect snapshot clears populated child context`, `accepted snapshot replaces cached child sample`, `omitted context leaves reading unchanged`, and `retained clear cannot rehydrate on row remount`. Implement explicit-property parsing and atomically update metadata/cache/mirrors; keep the hook a passive reader. Centralize live alias parsing once; preserve stale-event guard. Same accepted input has one cache publication, no extra subscription. This suborder passes ordinary snapshot/live clear tests and leaves interleaving acceptance for b/c explicitly open. Commands:
~~~
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-context-window.test.ts lib/state/slices/session-runtime/context-window.test.ts lib/state/slices/session/session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts lib/ws/handlers/session-models.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/state/slices/session-runtime/context-window.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session/session-slice.ts lib/ws/handlers/agent-session.ts hooks/domains/session/use-session-context-window.ts lib/state/slices/session/session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts hooks/domains/session/use-session-context-window.test.ts)
~~~

**UI-F2b: context-specific stale snapshot fence and hydration.** Own session/types.ts, hydration-epochs.ts, session-slice.ts epoch capture/merge/removal, session-runtime-slice.ts live-counter increments, hydration/hydrator.ts plus new hydrator-context-window.test.ts and deferred cases in session-context-window.test.ts. RED: defer measured snapshot, apply real live clear, resolve old snapshot; current baseline can revive it. Paired RED: defer null snapshot, apply live sample, resolve old null. Implement live-only epoch increments, exact pair retention on rejected snapshot, authoritative updated_at ordering and atomic accepted cache publication through F2a helper. Full route hydration and list-only mirror payload must use this same helper. Generic setTaskSession calls that spread existing metadata are not new live samples: increment live epoch only at explicit original event/local invalidation entrypoints, never from a copied merged record or accepted snapshot. Commands:
~~~
(cd apps/web && pnpm exec vitest run lib/state/slices/session/session-context-window.test.ts lib/state/hydration/hydrator-context-window.test.ts lib/state/slices/session/session-slice.upsert.test.ts hooks/domains/session/use-session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/state/slices/session/types.ts lib/state/slices/session/hydration-epochs.ts lib/state/slices/session/session-slice.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/hydration/hydrator.ts lib/state/hydration/hydrator-context-window.test.ts lib/state/slices/session/session-context-window.test.ts)
~~~

**UI-F2c: obsolete-transport full-session response/finalization.** Own lib/state/task-session-reads.ts and .test.ts, hooks/use-task-sessions.ts and use-task-sessions.stale-reads.test.ts. Depend on Task 19's public-internal connection.generation. RED: old connection list remains pending, connection leaves/returns, resolve new list then old; old response cannot revive context or clear replacement loading. Also RED for connected/disconnected/connected batched into a single render and same-session rerender while a pending request exists. Add generation to read-owner identity AND scopeIsCurrent comparison, explicitly observe it in the hook, force refresh on generation changes, retire old request handles/loading waiters without touching replacement ownership. Preserve existing final-consumer cancellation and shared-read/trailing-refresh behavior. These hooks already have cleanup/finalization code; extend its identity guard, do not add a second owner. Commands:
~~~
(cd apps/web && pnpm exec vitest run lib/state/task-session-reads.test.ts hooks/use-task-sessions.test.ts hooks/use-task-sessions.stale-reads.test.ts hooks/use-task-sessions.foreground.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/state/task-session-reads.ts lib/state/task-session-reads.test.ts hooks/use-task-sessions.ts hooks/use-task-sessions.stale-reads.test.ts)
~~~

Task 20 final browser missed-clear proof belongs after c. No early suborder marks finding #14 or task16's media complete.

### Test matrix

Use real createAppStore/StateProvider + actual setters/hooks; mock only transport:

- authoritative reconnect snapshot clears a populated cache without manual deletion (promote the reviewed diagnostic, using a real captured hydration epoch when there has been a live write).
- accepted newer snapshot replaces 68k by 82k while the panel stays mounted; unopened child detail subscriptions remain zero.
- omitted field preserves known reading; explicit null clears; unknown/invalid shapes obey parser rules and cannot freeze old usage.
- deferred old measured snapshot + intervening real registered WS clear -> release snapshot; cache and metadata remain null. Reverse: old null snapshot + live fresh sample -> fresh value survives.
- same-session unrelated state/message rerender does not invalidate response ownership or leave loading stuck.
- after clear, unrelated partial event, hook remount, route hydration and second consumer do not resurrect old metadata. Later genuine sample renders normally.
- latest authoritative same-key snapshot vs earlier response ordering; context cache and task session mirrors agree, including after route hydration.
- old-connection read finishing after reconnect cannot clear or restore new reading; releasing initiating consumer cannot wipe another consumer's valid shared cache.
- a local model-change clear does not immediately rehydrate old metadata on hook remount; a later genuine sample is usable. Keep existing session-models tests. This does not add provider sample-version guarantees or change the persistence fallback policy after a failed metadata clear.

Browser acceptance adds a MISSED-event clear case to existing desktop/phone context tests, not a duplicate of their working live-event clear: open measured panel, take browser's WS transport offline, persist explicit null through the separate ApiClient (browser misses its event), reconnect, wait for authoritative task-session list, assert context line removed without reload; then reload and assert it remains absent. Pair with another child's different reading surviving, no automatic child activation/subscription and zero horizontal overflow. Use browser context network offline/online or a test-controlled real socket close; do not replace the production handler with a direct-store clear. Existing causal helpers/strict WS accounting should observe the actual accepted list response. REST calls from ApiClient remain available while browser context is offline.

~~~
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-context-window.test.ts lib/state/slices/session-runtime/context-window.test.ts lib/state/slices/session/session-context-window.test.ts lib/state/hydration/hydrator-context-window.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/task-session-reads.test.ts hooks/use-task-sessions.stale-reads.test.ts)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/agent-session.context-window.test.ts lib/ws/handlers/session-models.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-session-context-window.ts lib/state/slices/session-runtime/context-window.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session/types.ts lib/state/slices/session/hydration-epochs.ts lib/state/slices/session/session-slice.ts lib/state/hydration/hydrator.ts lib/ws/handlers/agent-session.ts lib/state/task-session-reads.ts hooks/use-task-sessions.ts)
~~~

lib/state/task-session-reads.test.ts, hooks/use-task-sessions.stale-reads.test.ts, lib/ws/handlers/agent-session.test.ts and session-models.test.ts were verified present. The new session-context-window.test.ts, hydrator-context-window.test.ts and agent-session.context-window.test.ts are planned files, not existing evidence. Lint all newly added/changed tests too. Run the full owning session/hydration/handler Vitest folder when full-module mocks or hook shape changes require it, rather than assuming typecheck detects missing mock exports.

## Dependencies and ownership

[Task 19](task-19-reconnect-freshness.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

### UI-F2a: authoritative context property/cache ingestion

F2a implementation and verification are complete. The overall Task 20 remains in progress for F2b/F2c and the later browser missed-event acceptance.

- TDD RED: `(cd apps/web && pnpm exec vitest run lib/state/slices/session/session-context-window.test.ts --maxWorkers=1)` reproduced the populated-cache clear defect: 1 test failed because the 68k runtime entry remained after `setTaskSessionsForTask` accepted `context_window: null`; log `/tmp/errands-task20-f2a-red.log`.
- TDD RED: the same focused command reproduced metadata rehydration after `clearContextWindow`: 3 tests passed and `retained clear cannot rehydrate on row remount` failed because the old 68k sample returned; log `/tmp/errands-task20-f2a-red-remount.log`.
- GREEN: `(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-context-window.test.ts lib/state/slices/session-runtime/context-window.test.ts lib/state/slices/session/session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts lib/ws/handlers/session-models.test.ts --maxWorkers=1)` passed 5 files and 39 tests; log `/tmp/errands-task20-f2a-vitest-final.log`.
- Adjacent regressions: `(cd apps/web && pnpm exec vitest run lib/ws/handlers/agent-session.test.ts lib/state/slices/session/session-slice.upsert.test.ts --maxWorkers=1)` passed 2 files and 119 tests; log `/tmp/errands-task20-f2a-adjacent-final.log`.
- Scoped lint: `pnpm exec eslint --max-warnings 0 lib/state/slices/session-runtime/context-window.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session/session-slice.ts lib/ws/handlers/agent-session.ts hooks/domains/session/use-session-context-window.ts lib/state/slices/session-runtime/context-window.test.ts lib/state/slices/session/session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts hooks/domains/session/use-session-context-window.test.ts` passed with zero warnings; log `/tmp/errands-task20-f2a-eslint-final.log`.
- Formatting: Prettier passed for all nine changed frontend files; final runtime helper formatting also passed. Logs `/tmp/errands-task20-f2a-format-final.log` and `/tmp/errands-task20-f2a-format-last.log`.
- Typecheck: default `pnpm run typecheck` exited 134 at Node's 2 GiB heap before diagnostics; this is an infrastructure limit, not a type failure. `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed; logs `/tmp/errands-task20-f2a-typecheck.log` and `/tmp/errands-task20-f2a-typecheck-final.log`.
- The four required snapshot cases use actual `createAppStore`/`StateProvider` state and setters: authoritative clear, accepted replacement while the hook remains mounted, omission no-op, and retained clear across row remount. Coverage also checks sibling isolation, both metadata aliases and their existing precedence, stale-event rejection before writes, one cache publication per accepted live event, and zero child detail subscriptions for inactive rows.

F2a adds dependency-neutral `readContextWindowMetadata` (own-property presence, including explicit null/invalid values), `readContextWindowMetadataFromAliases` (metadata wins when it carries `context_window`, otherwise session_metadata), and `updateContextWindowMetadata` (canonical metadata write for runtime set/clear). `parseContextWindowEntry` retains its existing shape; `normalizeContextCompactionCount` is now exported. `setTaskSessionsForTask`, `setTaskSession` and `upsertTaskSessionFromEvent` synchronize accepted explicit metadata with the runtime cache in their store transaction. Runtime `setContextWindow`/`clearContextWindow` synchronize canonical session metadata and its task-list mirror. `agent-session.ts` normalizes aliases before the accepted upsert and retains the stale-event guard; it uses the direct runtime action only when no session row can be upserted.

F2b handoff: the private cache/metadata helper APIs above are available for epoch-aware accepted-pair resolution. F2a does not add context epochs, timestamp ordering, route/boot hydration synchronization, or read-owner generation fencing; those remain F2b/F2c. No browser test was run, and Task 20 is not complete.

### UI-F2b: context-specific stale snapshot fence and hydration

F2b implementation and verification are complete. Task 20 remains `in_progress` for F2c transport freshness and the final browser missed-event acceptance.

- TDD RED: `/tmp/errands-task20-f2b-red.log` reproduced stale measured/null overwrites, route revival, and missing route cache publication (9 failing regressions); `/tmp/errands-task20-f2b-red-rowless.log` reproduced rowless-clear resurrection (2 failing regressions); `/tmp/errands-task20-f2b-hook-red.log` reproduced retained-metadata fallback after a live invalidation; `/tmp/errands-task20-f2b-timestamp-red.log` reproduced timestamp-watermark regression that let an intermediate snapshot replace the newer context.
- GREEN and adjacent ingestion/model coverage: `(cd apps/web && pnpm exec vitest run lib/state/slices/session/session-context-window.test.ts lib/state/hydration/hydrator-context-window.test.ts lib/state/slices/session/session-slice.upsert.test.ts hooks/domains/session/use-session-context-window.test.ts lib/ws/handlers/agent-session.context-window.test.ts lib/ws/handlers/session-models.test.ts lib/state/slices/session-runtime/context-window.test.ts hooks/domains/session/session-state-reconciler.test.ts lib/state/slices/session/workspace-recovery-projection.test.ts --maxWorkers=1)` passed 9 files and 104 tests; final rerun `/tmp/errands-task20-f2b-vitest-final-3.log`.
- Lint: scoped ESLint with `--max-warnings 0` passed for all ten changed/new TypeScript files; final rerun `/tmp/errands-task20-f2b-eslint-final-3.log`.
- Formatting: Prettier passed for all ten changed/new TypeScript files; final rerun `/tmp/errands-task20-f2b-format-final-3.log`. `git diff --check` passed.
- Typecheck: `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed; final rerun `/tmp/errands-task20-f2b-typecheck-final-3.log`.
- Implementation inventory: added `taskSessions.contextWindowEpochBySession`, captured it in `captureTaskSessionHydrationEpoch[s]`, incremented it only from explicit context-bearing live upserts and runtime set/clear actions, and purged it with session removal. Accepted snapshot context is fenced by request-start epoch and strict persisted `updated_at`; rejected pairs retain exact existing own-property presence/value while unrelated metadata still merges, and stale rows retain the newer `updated_at` watermark.
- Route and list hydration resolve context before deep merge, then synchronize the accepted pair into the runtime cache and canonical/list copies. List-only rows work without a mounted context hook. The context hook reads retained metadata only at epoch zero, so a cleared cache with a live epoch cannot revive stale metadata.
- Owned production inventory: `apps/web/lib/state/slices/session/types.ts`, `apps/web/lib/state/slices/session/hydration-epochs.ts`, `apps/web/lib/state/slices/session/session-slice.ts`, `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`, `apps/web/lib/state/hydration/hydrator.ts`, and the epoch-aware fallback in `apps/web/hooks/domains/session/use-session-context-window.ts`. Regression inventory: `apps/web/lib/state/slices/session/session-context-window.test.ts`, `apps/web/lib/state/hydration/hydrator-context-window.test.ts`, `apps/web/hooks/domains/session/use-session-context-window.test.ts`, and focused epoch assertions in `apps/web/lib/ws/handlers/agent-session.context-window.test.ts`. The row-remount test dispatches a real `session.models_updated` event and verifies its model-change clear advances the context epoch before remount.
- F2c transport-owner files were not changed, browser suites were not run, and no shared plan or Git state was changed.

### UI-F2c: connection-generation ownership and missed-event acceptance

F2c implementation and Task20 acceptance are complete. The full F2a/F2b/F2c algorithm, required regressions, and desktop/phone browser gates passed; Task25 curated media remains separate.

- TDD RED: `/tmp/errands-task20-f2c-red.log` reproduced the pending old-connection response being accepted after reconnect; generation ownership tests also proved the owner did not change before the fix.
- GREEN focused transport regressions: `pnpm exec vitest run lib/state/task-session-reads.test.ts hooks/use-task-sessions.test.ts hooks/use-task-sessions.stale-reads.test.ts hooks/use-task-sessions.foreground.test.ts --maxWorkers=1` passed 4 files and 42 tests.
- The full-session read owner identity and completion scope now include `connection.generation`; the hook selects generation independently of status and refreshes when it changes, including a disconnected-to-connected batch in one render. Existing shared-read/trailing-refresh behavior and last-consumer cancellation remain covered.
- Desktop missed-event acceptance passed on current E2E assets: `pnpm e2e:run --no-build --project chromium e2e/tests/session/errands.spec.ts -- --grep "desktop errand rows read each child"` reports 1 Chromium test passed in `/tmp/errands-task20-f2c-desktop-e2e-final-visual.log`. Pixel 5 acceptance passed: `pnpm e2e:run --no-build --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --grep "phone errand rows expose passive context"` reports 1 mobile-chrome test passed in `/tmp/errands-task20-f2c-mobile-e2e-final.log`.
- Each browser case starts from two measured child rows, installs a transparent route for the real `/ws` only for the final reconnect phase, takes the browser offline and closes its live socket, persists explicit null through the separate `ApiClient`, reconnects, and observes the authoritative full-session list response carrying `context_window: null`.
- Both cases assert that the target context row clears without reload while the sibling's 23.0K sample remains, no child detail request or child chat is activated, reload stays clear, and the desktop/Pixel 5 viewport has no horizontal overflow. Inspected desktop and phone captures show the cleared row beside the retained sibling sample.
- `flock -x /tmp/kandev-errands-repair-build.lock env NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed after the final test edits. Scoped `pnpm exec eslint --max-warnings 0` and `pnpm exec prettier --check` passed for all owned production/test/E2E files. The exact focused Vitest rerun passed 4 files and 42 tests.
- `make build-web-e2e` rebuilt the browser assets before the `--no-build` desktop and phone runs; the current backend/mock artifacts were already fresh, so no Go/backend rebuild was needed. No shared plan edit, Git state operation, commit, push, or live instance mutation occurred.
