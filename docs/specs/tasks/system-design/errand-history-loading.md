---
status: current
system: tasks
created: 2026-11-12
owners:
  - kandev
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
---

# Errand history loading design

[Errands design map](errands.md#errands-design-map) lists shared contracts and companion ownership.

## Authority and source grounding

The task repository owns the compact browser projection. The [Errand ownership decision](../../../decisions/2026-11-17-errand-owned-conversations.md) places the transcript and execution under the Errand. [Recovery settlement authority](../../../decisions/2026-11-09-errand-recovery-settlement-authority.md) and [conversation navigation](errand-conversation-navigation.md) govern result evidence and parent hosting.

A task’s ordinary session inventory supplies boot, navigation, layout restoration and reconnect. The Errand browser reads use independent row versions, bounded scopes and client observations. An ordinary session snapshot or removal affects only ordinary state. The Errand read coordinator owns membership, revisions and eviction for its own summary, history and inspection.

[Investigation](../../../plans/errands-remaining/loading-performance-investigation.md) and [impact illustration](../../../plans/errands-remaining/loading-impact.html) support payload/work hypotheses, not latency or heap conclusions. Normal session detail gating already exists and stays intact.

## Browser read contracts

The separate typed WS actions are `errand.browser_summary`, `errand.history_page` and `errand.inspect`. Preserve complete `errand.list`, `Service.List`, repository inventories and MCP responses. New actions authorize task workspace read before returning any rows, counts, cursor or parent identity. They are read-only and bypass only the feature-enabled gate, never authorization; cancellation/send-result/new work remain gated. Missing/denied tasks and foreign child IDs use the existing not-found policy. Malformed cursor/limit returns validation error; backend failure preserves a retryable error rather than an empty-success response. Unsupported actions fail explicitly; never fall back to downloading the complete legacy list. The bundled browser requires the compact actions. Complete Errand listing has its own contract.

| Action          | Request                                                             | Response                                                                                                         |
| --------------- | ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| browser_summary | task_id                                                             | scope, compact outstanding rows, authoritative task totals and parent totals                                     |
| history_page    | task_id, parent_session_id, limit (default/max 25), cursor optional | scope, compact finished rows, total_finished, has_more, next_cursor or null                                      |
| inspect         | task_id, child_session_id                                           | verified Errand identity, compact row, owned conversation availability, and minimal parent identity or unavailable |

Summary rows include id/task/parent, title, profile identity, created time, projected status/attention, live/outstanding classification, unread boolean, result-state/reception indicator, and existing measured context counts. Omit objective bodies, result text, MCP attachment state and transcript. Counts include live capacity usage and configured cap separately from queued count, outstanding count, finished count, pending result count and unread count; parent aggregates use the same predicates. Never derive totals or capacity from loaded pages. A terminal pending result retains its terminal status with a pending-result indicator in the outstanding section; it is not relabeled running. Overlap is forbidden: history is the complement of outstanding eligibility.

Unread compares the latest qualifying owned transcript message with the Errand's persisted read cursor. Owned messages use the shared transcript visibility predicate and an indexed message rank. A visible conversation acknowledges its newest loaded message through `errand.mark_read`. Task write authorization and exact task, Errand, and message ownership are required. The cursor advances in transcript order, so a delayed request cannot mark later output as read. Ordinary session unread updates do not feed Errand counts.

Ordinary parent serialization reads `hasErrands` and outstanding work through a scalar repository projection using the same outstanding/recovery predicate. It must not enumerate child DTOs or project individual child sessions for the waiting indicator. Both booleans share one statement snapshot; a read failure remains an error, and observation grants no settlement authority.

Inspection returns the compact Errand row and parent hosting identity. On Open, the selected Errand view reads its own transcript and uses its own conversation actions. A queued reservation can appear in the summary before its transcript is available. A direct link to a retained Errand can show that transcript when the parent is unavailable.

## Storage queries and counts

The shared task repository implements the browser queries for SQLite and PostgreSQL. Ordinary navigation reads preserve ordinary session fields, revision guards and reconciliation. Boot builds its session, model, environment and worktree maps from ordinary sessions. Only the compact Errand projection contributes Errand rows to the browser summary and history.

History SELECT must apply task/parent, finished eligibility, keyset ordering and LIMIT 26 before row materialization; trim lookahead before enrichment. Select only compact columns, never `errandColumns` result bodies. Match immutable `created_at DESC, id ASC`; next predicate is timestamp older OR timestamp equal and id greater. Cursor encodes format version, task, parent, ordering and last emitted tuple; reject foreign scope, invalid encoding and unsupported versions. Parameterize all values. Cursor is not authorization and does not grant a snapshot or expose SQL. Store-generated creation timestamps are non-null; fixtures with legacy malformed times need explicit deterministic normalization matching the existing panel order, not client-dependent ordering.

Compute counts in SQL in the same read transaction as page and summary membership. Reuse the shared predicates for queued and admitted work, current permissions, unresolved recovery, pending results, and unread output. Errand status comes from its lifecycle, latest generation, and current admission. The ordinary session row supplies the parent identity and its waiting state. Differential fixtures and paired store tests verify the projected counts and membership. Query-plan tests require indexed task, parent, order, and current message access. Aggregate work can depend on indexed history size. Materialization and browser reconciliation stay bounded by the returned rows.

Summary row count scales with actual outstanding work (including queued and undelivered results), not retained finished history. Capacity does not bound queue length; this design does not pretend otherwise. Aggregate counts remain complete even when rows are not loaded. Stop all continues the existing server task-wide cancellation path; it never loops over browser cache IDs.

## Client ownership, transitions and bounds

Keep compact Errand browser data separate from ordinary session slice membership and detailed session caches. Migrate every browser `useTaskErrands` consumer and selector away from the assumption that its array is complete; use explicit summary, outstanding rows and page state types. Keep complete operational reads under the separate `listErrands` contract. Audit `apps/web/lib/types/errands.ts`, WS action types and Go DTOs together in one sequential work order. Opening history belongs to the current normal parent's disclosure; no automatic fetch on mount, idle callback, focus or reconnect.

Use existing read-scope and live-revision machinery, extended with parent/page request identity. Coalescing key is store + authorization scope + task + parent + cursor + limit + connection generation + reset epoch. The store owns shared request cancellation; unmount of one waiter releases only that waiter. Last waiter/task exit may abort; late completion must still be rejected. A replacement waiter after abort obtains a new attempt. Mutations/events advance the local observation revision before application. A response with a different captured revision, generation, reset epoch, scope or attempt identity cannot replace current membership/counts. Retry summary through one coalesced fresh read, never a history chain; history shows Refresh/Retry on explicit demand after invalidation. Test cancellation of the initiating consumer while another remains mounted.

Row `version` protects Errand fields. Errand status changes advance browser observation revision. Deletion records a local tombstone for the active task epoch so pre-delete pages cannot resurrect the ID. Reconnect or epoch reset discards old attempts and caches together; no permanent tombstone accumulation is needed. Summary replacement is authoritative for outstanding membership only, never permission to delete saved transcript cards or ordinary sessions.

Keep history cache only for the selected task/parent, at most four pages of 25 summaries (100). Load more after the cap evicts the oldest displayed page and exposes a localized start-again control; list scroll anchor and visible range are updated explicitly. Closing disclosure drops pages/cursors; switching parent/task drops them too. Retain only current selected child detail plus the already retained parent; release child transcript subscriptions on Back/switch under current view ownership, without shortening ordinary sibling lifetimes. Remove child-only detail entries on exit unless another visible surface owns them. Preview/Threads use per-visible-surface ownership and the same 100-row bound per open surface; they do not pin background task histories. No global unlimited history map.

On terminal settlement, remove a row from outstanding only once pending-result/recovery eligibility clears; update counts and mark any open history chain changed. On continuation, move it into outstanding and remove any loaded finished copy. Result receipt can clear outstanding without changing its historical terminal status. Deletion removes cached copies, advances revision and updates counts through summary refresh. New terminal events never insert unseen history rows into cache; WS handlers update only already loaded summary identities or outstanding rows, then invalidate history membership. Reconnect refreshes current summary, marks open history stale and offers Refresh; it cannot refill all previously visited pages. Keyset pages are a live listing, not a frozen snapshot: membership changes invalidate the chain and require restart from page one on demand, preventing skipped continuations from being treated as complete history.

## Layout and navigation compatibility

Apply the authoritative ordinary-session allowlist BEFORE every `fromJSON`/`applyLayout` path, including saved environment, global/default migration, maximized, pre-maximized and switch/restore-maximize paths. `currentSessionIds = null` means unresolved, not allow-all. Restore non-session panels while holding unresolved session panels out of materialization; once the existing ordinary list resolves, restore ordinary panels with their saved order/geometry and surviving maximize group. Do not persist this temporary filtered layout over the user's original snapshot. No fetching historical identities to sanitize saved blobs. Ordinary session events replenish ordinary membership. Keep foreign-environment exclusions.

Direct Errand links validate one Errand and keep its URL while resolving the recorded parent. The parent selects its ordinary session. If that parent is unavailable, the direct link can use the owned reader with the unavailable-parent state. Direct ordinary routes retain their targeted lookup, selection and subscription behavior. Normal session content stays lazy. Saved disclosures use their recorded payload and open the Errand only on explicit request.

## Desktop and phone

Reuse `components/task/chat/errands/errands-panel.tsx`, its existing popover/Drawer wrappers, ordering derivation and parent-owned child navigation. Outstanding rows remain first. The initially collapsed finished disclosure shows server count; its first expansion requests one page. Loading/empty/error/retry/load-more/end states live within that disclosure and cannot disable Stop all or the parent composer. Localize all new copy in all shipped locales, use status announcements without repeated focus theft, preserve keyboard disclosure controls and restore focus on dismissal.

Phone entry remains the Errands pill, opening the existing inset bottom drawer for short list selection. Header and Stop-all footer are fixed; the list has one `min-h-0` internal scroller, `dvh` containment and safe-area padding. Load-more/retry controls have at least 44px targets. Child content is deep reading, so Open navigates to the existing full-screen reader with Back; it is not squeezed into the drawer. Parent reader anchor and desktop preferences survive. See UI-01/UI-02 in the [plan](../../../plans/errand-history-loading/plan.md).

## Traceability and evidence

| Criteria | Design boundary                                    | Verification owner |
| -------- | -------------------------------------------------- | ------------------ |
| .1, .5   | ordinary queries and pre-materialization allowlist | tasks 02, 04       |
| .2, .6   | SQL eligibility, totals, observation guards        | tasks 01, 02, 03   |
| .3, .7   | bounded cache and responsive disclosure            | tasks 03, 04       |
| .4, .8   | inspection and owned transcript          | tasks 02, 03, 04   |
| .9       | reproducible performance protocol                  | tasks 00, 04       |

All criterion suffixes refer to AC-TASKS-ERRAND-HISTORY-001. Existing tests include backend `service_session_summary_projection_test.go`, `errand_ws_handlers_test.go`, repository summary scenarios, web `use-task-errands.test.ts`, `errands-slice.test.ts`, `lib/ws/handlers/errands.test.ts`, `dockview-layout-restore.test.ts`, `dockview-layout-restore-maximize-session.test.ts` and desktop/mobile `e2e/tests/session/*errands.spec.ts`.

## Risks and unresolved measurement

No numeric timing or memory tolerance is invented: task00 must record baseline variance and numeric budgets before production work can start. A query predicate parity failure is a blocker, not justification for a full-list fallback. Query-plan/index cost and recovery predicate portability need proof in task01. No HAR evidence establishes ordinary transcript fan-out or settings-page regression. No runtime cleanup improvement is claimed. Public docs updates and already-implemented cleanup verification remain separately scoped; implementation must report browser history cost and runtime resource evidence separately.

## Implementation plans

[Bounded history package](../../../plans/errand-history-loading/plan.md) records the completed implementation, source identity, verification and explicit first-delivery performance disposition. The measured results retain the original thresholds and disclose fixed costs separately from history-growth bounds.
