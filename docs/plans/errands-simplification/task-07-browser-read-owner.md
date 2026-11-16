---
id: 07-browser-read-owner
title: Own concrete Errand browser reads below React consumers
status: pending
wave: 7
depends_on:
  - 06-prompt-outcome-owner
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.1
  - AC-TASKS-ERRAND-HISTORY-001.2
  - AC-TASKS-ERRAND-HISTORY-001.3
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.6
  - AC-TASKS-ERRAND-HISTORY-001.7
  - AC-TASKS-ERRAND-HISTORY-001.8
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
---

# Task07: one concrete browser-read owner

The store-scoped coordinator shares promises, but the summary/history hook, child Open, active route admission and mounted inspection still reconstruct read keys, observations, identity validation, publication and release. Presentation components also coordinate history-owner IDs. Deepen the existing reader so callers request a concrete operation and own their lifetime rather than repeat its protocol. This is an ownership refactor with unchanged presentation, API, ordinary-session and performance contracts.

## Selected design

Keep one reader per existing store and reuse its transport, scoped attempts and reducers. Expose concrete summary reads, a panel-owned history handle, and validated child inspections. Summary capture/key/begin/success/failure publication belongs in the reader. A history handle owns one active task/parent chain, cursor, leases and close/dispose behavior; every parent disclosure in one panel shares that handle. Opening another parent releases the previous chain. Preserve explicit Load more, Retry, Refresh/Start again and the four-page/100-row bound.

Validated inspection centralizes compact task/child identity and existence checks for the three live callers. Keep transport waiters distinct from retained-data owners. A fulfilled response is not permanently fresh: retaining it must revalidate its captured scope/generation/task epoch/revision at the publication boundary. The current retention reducer stamps current scope rather than independently checking a captured observation, so moving the caller checks without an equivalent owner-side check is forbidden. Use an explicit retain-if-current operation on the captured inspection/lease if needed; completion alone must not navigate or retain details indefinitely. An opener releases its provisional retained-data owner on failure, supersession or cancellation while it remains mounted, unless responsibility transferred to the visible consumer. Another independent mounted owner must survive that release.

React callers retain eligibility and view selection. Summary/history remain feature-gated; history never loads on mount, focus or reconnect. Reconnect leaves history visibly stale. Explicit Open and route admission retain their existing eligibility rather than inheriting a new universal gate. Open owns navigation generation, latest attempt, mount and embedded context; full-session role/parent validation remains separate from compact inspection validation. Captured inspection-owner currency must also guard the later full-session fetch publication and final selection: scope changes, task reset and deletion during that second await must not resurrect a session row, retained inspection or navigation. Route admission preserves the ordinary fast path and unavailable versus read_failed. Mounted inspection retains its independent pending-detail reservation and Task04 subscription budget. Keep reducer tombstone, scope, revision, cursor and attempt guards even when cancellation fails.

Migrate real callers and delete their superseded key builders, lease maps and repeated compact-inspection predicates. Keep the low-level coalescing machinery private if useful; no second cache, global subscription, query framework or generic resource API is needed. Task-scoped Stop all and parent-scoped counts remain separate; mutation APIs stay outside the reader.

Consolidate the duplicated conversation-cache inventory used by errand-conversation-cache.ts and session-slice.ts behind their existing actions. Share which cache entries are removed, while keeping explicit unowned-detail eviction versus session-removal policies. Eviction remains reopenable and protects other readers; deletion advances the existing generations/tombstones and preserves shared-environment rules. Keep those policy checks at their current owners, and do not introduce a generic cleanup registry or a mode that silently gives eviction deletion semantics. This completes the same browser data-lifetime boundary; both real-store suites remain.

The static audit found resolveTaskRoute only through fetchSessionDataForTask/fetchTaskNavigationData, with those wrappers and fetchSessionData unused in production. The live route uses readTaskNavigationIdentity, assertRequestedSession and buildTaskNavigationShellData. Verify exports/generated consumers before deletion; migrate useful route/hydration/enrichment assertions to live entry points, remove unreachable wrappers/resolver without replacement adapters, and correct the stale apps/web/AGENTS.md path guidance. Keep live session-page-state shell/enrichment/type exports. A newly demonstrated external consumer must be reported rather than silently broken.

Keeping the current arrangement avoids migration but preserves multiple caller protocols. This design removes those protocols while retaining independent navigation, cache and transport lifetimes. It does not claim a latency improvement.

## Ownership and sequence

Own apps/web/lib/state/slices/errands/ browser-read/coordinator and directly affected reducer/selectors/types, hooks/domains/session/use-task-errands.ts, the Errands panel/history/Open/view-model consumers, src/task-session-admission.tsx and active route wiring, directly affected private lib/ssr loaders and tests, the narrowly shared conversation-cache inventory in session/session-slice.ts and errand-conversation-cache.ts, and the one stale apps/web/AGENTS.md reference. No backend/schema, locale/copy, component layout, mutation API, public plugin contract or general Dockview/chat redesign. Own this Results section and one concise receipt; parent owns other plans and integration.

First establish real-store Open/admission coverage before changing the reader. If the new controls reveal a current contract failure, retain its RED and repair that narrowly as a separately reported phase before the structural migration; do not silently present changed behavior as refactoring. Then migrate summary, panel history and the three live inspection callers; consolidate the two cache inventories without changing either release policy. Remove unreachable wrappers after mapping their retained test obligations. Keep parent work and other owners' files; no staging, commit, push, live instance/provider/database actions or delegation. One shared-checkout writer, including documentation.

## Acceptance

1. Callers no longer construct serialized read keys, observations or duplicate transport/publication protocols. One panel owns one bounded history chain. Compact inspection retention rejects stale fulfilled responses; navigation and retained-data lifetime remain independent.
2. Real StateProvider/store/coordinator/reducer tests prove targeted Open only, ordinary navigation during each await, another mounted owner's survival after opener release, final-owner eviction, scope/reset/deletion during both awaits, failed-operation release while the opener stays mounted, direct-child admission/reload, retained feature-off inspection, missing-parent fallback, ordinary fast-path zero inspections, direct-route failure/retry and zero ordinary child membership. Keep existing coalescing, shared-owner cancellation, parent switching, reconnect, A–B–A and deletion races. Mock transport/navigation adapters, not the state boundary.
3. Focused unit/type/lint and managed desktop/phone demand/loading/retention controls pass against one frozen source/artifact set. Record removed paths/obligations and remaining compatibility consumers. Performance is measured separately on final browser source using unchanged protocols and thresholds; functional pass is not numeric acceptance.

## Verification

From repository root, use pinned pnpm9.15.9 and the existing shared lock:

```bash
flock -x /tmp/kandev-errands-repair-build.lock pnpm --dir apps/web test hooks/domains/session/use-task-errands.test.ts lib/state/slices/errands lib/ws/handlers/errands.test.ts components/task/chat/errands src/task-detail-route-session-selection.test.tsx lib/ssr/session-page-state.test.ts lib/ssr/session-page-state-child-route.test.ts lib/state/slices/session/remove-task-session.test.ts
flock -x /tmp/kandev-errands-repair-build.lock pnpm --dir apps/web run typecheck
```

Capture the unmodified baseline, add the real-store boundary tests and run them before migration, then repeat the final selector. Do not manufacture RED for a behavior-preserving refactor or drop assertions tied to a real retained contract. Exact-file Oxlint/Prettier and git diff --check are required. Name any new contract test in the final selector and distinguish existing historical RED evidence from new results.

From apps/web, sequentially on frozen production source:

```bash
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project chromium -- tests/session/errand-history-demand.spec.ts tests/session/errand-history-loading.spec.ts tests/session/errand-detail-retention.spec.ts
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/session/mobile-errand-history-demand.spec.ts tests/session/mobile-errand-history-loading.spec.ts tests/session/mobile-errand-detail-retention.spec.ts
```

Also run these existing compatibility cases against the same frozen artifacts (use the managed runner's supported --no-build option only after verifying artifact/source identity):

```bash
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project chromium tests/session/errands.spec.ts --grep 'two clients remove deleted errands and flag-off restart retains detached sessions$'
flock -x /tmp/kandev-errands-repair-build.lock pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errands.spec.ts --grep 'task29 phone separates normal conversations and parent errands with temporary child navigation and quiet history$'
```

The desktop case supplies real flag-off retained direct navigation; the phone case supplies direct-child reload/Back and parent/ordinary parity. New real-store admission coverage must explicitly cover feature-off inspection and missing-parent/ordinary routes; the six enabled-feature browser cases alone are insufficient. Parent switching, independent panels and Strict Mode cleanup must retain one bounded history owner per surface.

Use established bounded Go build settings including the managed Make override, preserve artifact hashes, and retain all process handles. No UI structure or touch behavior changes are intended; existing desktop/phone compositions remain authoritative, so no new ASCII preview is required. The parent coordinates matching-main ordinary performance and history-growth controls once all affected browser source is settled; do not change workloads, caps or select only favorable samples.

## Results

Pending.
