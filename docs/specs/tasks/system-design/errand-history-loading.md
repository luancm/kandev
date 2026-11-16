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

Tasks own the browser inspection contract. The [Errands ADR](../../../decisions/2026-10-23-errands-first-class-delegation.md), [recovery settlement authority](../../../decisions/2026-11-09-errand-recovery-settlement-authority.md), [Errands design](errands.md) and [conversation navigation](errand-conversation-navigation.md) remain authority. This adds a browser read projection, not another lifecycle owner. The projection/query choices are sufficiently captured here; no additional ADR is needed.

Source inspection at requested base `6e4c5ca9bc`: `internal/errands/list.go` calls `ListErrandsByTask`, then loads launched sessions and enriches every row. `status.go` separates `ProjectStatus`, `IsLive` and `IsOutstanding`; needs-attention recovery and pending results must survive the split. `internal/task/repository/sqlite/errands.go` has immutable creation identity, row `version` and independent `result_revision`; neither alone orders all session-derived observations. `internal/task/handlers/errand_ws_handlers.go` exposes complete `errand.list` and gates it with `errandGuard`. `apps/web/lib/api/domains/errand-api.ts` returns `Promise<Errand[]>`; silently changing its meaning to a partial page is forbidden.

`internal/backendapp/boot_state_task_detail.go` uses `ListTaskSessionSummaryObservations` and builds session/model/environment/worktree maps. `apps/web/lib/state/slices/session/session-slice.ts` reconciles full ordinary-list input. `hooks/domains/session/use-task-errands.ts` coalesces complete lists by store/task/read scope/connection/reset epoch. `lib/ws/handlers/errands.ts` and `lib/state/slices/errands/` apply live revisions. `components/task/dockview-layout-restore.ts` invokes `api.fromJSON` for ordinary and maximized snapshots; its foreign-environment filter permits unknown identities. Filtering after hydration does not prevent panel creation.

[Investigation](../../../plans/errands-remaining/loading-performance-investigation.md) and [impact illustration](../../../plans/errands-remaining/loading-impact.html) support payload/work hypotheses, not latency or heap conclusions. Normal session detail gating already exists and stays intact.

## Browser read contracts

The separate typed WS actions are `errand.browser_summary`, `errand.history_page` and `errand.inspect`. Preserve complete `errand.list`, `Service.List`, repository inventories and MCP responses. New actions authorize task workspace read before returning any rows, counts, cursor or parent identity. They are read-only and bypass only the feature-enabled gate, never authorization; cancellation/send-result/new work remain gated. Missing/denied tasks and foreign child IDs use the existing not-found policy. Malformed cursor/limit returns validation error; backend failure preserves a retryable error rather than an empty-success response. Unsupported actions fail explicitly; never fall back to downloading the complete legacy list. An old client continues using unchanged legacy actions, while the new bundled browser requires these new actions.

| Action          | Request                                                             | Response                                                                                                         |
| --------------- | ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| browser_summary | task_id                                                             | scope, compact outstanding rows, authoritative task totals and parent totals                                     |
| history_page    | task_id, parent_session_id, limit (default/max 25), cursor optional | scope, compact finished rows, total_finished, has_more, next_cursor or null                                      |
| inspect         | task_id, child_session_id                                           | verified immutable role/parent identity, compact errand, minimal parent id/title/role/environment or unavailable |

Summary rows include id/task/parent, title, profile identity, created time, projected status/attention, live/outstanding classification, unread boolean, result-state/reception indicator, and existing measured context counts. Omit objective bodies, result text, MCP attachment state and transcript. Counts include live capacity usage and configured cap separately from queued count, outstanding count, finished count, pending result count and unread count; parent aggregates use the same predicates. Never derive totals or capacity from loaded pages. A terminal pending result retains its terminal status with a pending-result indicator in the outstanding section; it is not relabeled running. Overlap is forbidden: history is the complement of outstanding eligibility.

Ordinary parent serialization reads `hasErrands` and outstanding work through a scalar repository projection using the same outstanding/recovery predicate. It must not enumerate child DTOs or project individual child sessions for the waiting indicator. Both booleans share one statement snapshot; a read failure remains an error, and observation grants no settlement authority.

Inspect is an identity/read projection, not transcript transport. After identity validation, reuse existing targeted session/detail/transcript APIs for that one selected child and existing active-view subscription ownership. Do not route it through complete task session enumeration. A queued reserved child with no session row is shown as queued, without fabricated detail. Parent unavailable is distinct from missing child. Parent identity supplies hosting/environment needs without fetching parent transcript unless it is shown under existing retained-parent behavior.

## Storage queries and counts

Add browser-only repository methods alongside complete methods in `internal/task/repository/interface.go` and `sqlite/errands*.go`; these shared repository implementations must retain SQLite/PostgreSQL portability. Filter role at SQL level for the new ordinary-browser list observation path before scanning rows or computing per-session enrichment. Preserve existing complete service/repository methods for internal callers; add an explicit ordinary-browser option/action rather than changing their default semantics. Audit browser callers separately: `use-task-removal.ts` retains the complete list for deletion inventory. All ordinary navigation boot/list refresh/reconnect callers must use the explicit navigation read; ordinary row fields, revision guards and reconcile behavior stay unchanged. Remove finished-child entities from every boot graph branch, including errand/model/environment/worktree side maps and per-session ID collections; retaining them in a parallel boot map would defeat the bound. Compact outstanding summaries use the separate browser projection. Errand detail admitted by explicit inspection must not re-enter `taskSessionsByTask` ordinary membership.

History SELECT must apply task/parent, finished eligibility, keyset ordering and LIMIT 26 before row materialization; trim lookahead before enrichment. Select only compact columns, never `errandColumns` result bodies. Match immutable `created_at DESC, id ASC`; next predicate is timestamp older OR timestamp equal and id greater. Cursor encodes format version, task, parent, ordering and last emitted tuple; reject foreign scope, invalid encoding and unsupported versions. Parameterize all values. Cursor is not authorization and does not grant a snapshot or expose SQL. Store-generated creation timestamps are non-null; fixtures with legacy malformed times need explicit deterministic normalization matching the existing panel order, not client-dependent ordering.

Compute counts in SQL in the same read transaction as page/summary membership. Reuse the authoritative predicates demonstrated by `errands_summary_scenarios_test.go` and `errands_summary_batch_scenarios_test.go`, extending them for recovery and current unread state. SQL eligibility must match `ProjectStatus`/`IsOutstanding`, including session state, current permission, unresolved recovery and terminal pending results. Query joins must select current turn/revision evidence, not fan out historical messages. Differential table-driven tests compare SQL classifications to existing service observations across all states. Never fetch all children to classify them in Go. Query-plan tests require indexed task/parent/order access and no per-finished-child detail/enrichment calls; aggregate database work can depend on indexed history size, but materialization and browser reconciliation cannot. Add only indexes justified by query plans; no new lifecycle state or provider rewrite.

Summary row count scales with actual outstanding work (including queued and undelivered results), not retained finished history. Capacity does not bound queue length; this design does not pretend otherwise. Aggregate counts remain complete even when rows are not loaded. Stop all continues the existing server task-wide cancellation path; it never loops over browser cache IDs.

## Client ownership, transitions and bounds

Keep compact Errand browser data separate from ordinary session slice membership and detailed session caches. Migrate every browser `useTaskErrands` consumer and selector away from the assumption that its array is complete; use explicit summary, outstanding rows and page state types. Retain legacy `listErrands` signature for compatibility consumers. Audit `apps/web/lib/types/errands.ts`, WS action types and Go DTOs together in one sequential work order. Opening history belongs to the current normal parent's disclosure; no automatic fetch on mount, idle callback, focus or reconnect.

Use existing read-scope and live-revision machinery, extended with parent/page request identity. Coalescing key is store + authorization scope + task + parent + cursor + limit + connection generation + reset epoch. The store owns shared request cancellation; unmount of one waiter releases only that waiter. Last waiter/task exit may abort; late completion must still be rejected. A replacement waiter after abort obtains a new attempt. Mutations/events advance the local observation revision before application. A response with a different captured revision, generation, reset epoch, scope or attempt identity cannot replace current membership/counts. Retry summary through one coalesced fresh read, never a history chain; history shows Refresh/Retry on explicit demand after invalidation. Test cancellation of the initiating consumer while another remains mounted.

Row `version` protects errand-owned fields, but session-only status/unread/context events also advance observation revision. Deletion records a local tombstone for the active task epoch so pre-delete pages cannot resurrect the ID. Reconnect or epoch reset discards old attempts and caches together; no permanent tombstone accumulation is needed. Summary replacement is authoritative for outstanding membership only, never permission to delete saved transcript cards or ordinary sessions.

Keep history cache only for the selected task/parent, at most four pages of 25 summaries (100). Load more after the cap evicts the oldest displayed page and exposes a localized start-again control; list scroll anchor and visible range are updated explicitly. Closing disclosure drops pages/cursors; switching parent/task drops them too. Retain only current selected child detail plus the already retained parent; release child transcript subscriptions on Back/switch under current view ownership, without shortening ordinary sibling lifetimes. Remove child-only detail entries on exit unless another visible surface owns them. Preview/Threads use per-visible-surface ownership and the same 100-row bound per open surface; they do not pin background task histories. No global unlimited history map.

On terminal settlement, remove a row from outstanding only once pending-result/recovery eligibility clears; update counts and mark any open history chain changed. On continuation, move it into outstanding and remove any loaded finished copy. Result receipt can clear outstanding without changing its historical terminal status. Deletion removes cached copies, advances revision and updates counts through summary refresh. New terminal events never insert unseen history rows into cache; WS handlers update only already loaded summary identities or outstanding rows, then invalidate history membership. Reconnect refreshes current summary, marks open history stale and offers Refresh; it cannot refill all previously visited pages. Keyset pages are a live listing, not a frozen snapshot: membership changes invalidate the chain and require restart from page one on demand, preventing skipped continuations from being treated as complete history.

## Layout and navigation compatibility

Apply the authoritative ordinary-session allowlist BEFORE every `fromJSON`/`applyLayout` path, including saved environment, global/default migration, maximized, pre-maximized and switch/restore-maximize paths. `currentSessionIds = null` means unresolved, not allow-all. Restore non-session panels while holding unresolved session panels out of materialization; once the existing ordinary list resolves, restore ordinary panels with their saved order/geometry and surviving maximize group. Do not persist this temporary filtered layout over the user's original snapshot. No fetching historical identities to sanitize saved blobs. An ordinary row arriving through WS is admitted only with verified non-Errand role. Retain any existing foreign-environment exclusions.

Implement targeted child routing in the same work order as ordinary-list isolation, before switching its consumers; do not leave an intermediate build unable to resolve an excluded child. Direct child routes first inspect exactly that ID and retain the selected URL during identity resolution. Resolve host from minimal parent identity, never automatic primary/first-row fallback. A missing parent uses the existing generic child reader with explicit unavailable-parent UI; no child tab or manufactured ordinary parent. Direct ordinary routes retain their existing targeted lookup, selection and subscription behavior. Preserve normal sessions' initial contents and detail laziness. Saved result disclosure renders persisted payload/duration and legacy omissions without calling inspect or history; only explicit Open performs the child read.

## Desktop and phone

Reuse `components/task/chat/errands/errands-panel.tsx`, its existing popover/Drawer wrappers, ordering derivation and parent-owned child navigation. Outstanding rows remain first. The initially collapsed finished disclosure shows server count; its first expansion requests one page. Loading/empty/error/retry/load-more/end states live within that disclosure and cannot disable Stop all or the parent composer. Localize all new copy in all shipped locales, use status announcements without repeated focus theft, preserve keyboard disclosure controls and restore focus on dismissal.

Phone entry remains the Errands pill, opening the existing inset bottom drawer for short list selection. Header and Stop-all footer are fixed; the list has one `min-h-0` internal scroller, `dvh` containment and safe-area padding. Load-more/retry controls have at least 44px targets. Child content is deep reading, so Open navigates to the existing full-screen reader with Back; it is not squeezed into the drawer. Parent reader anchor and desktop preferences survive. See UI-01/UI-02 in the [plan](../../../plans/errand-history-loading/plan.md).

## Traceability and evidence

| Criteria | Design boundary                                    | Verification owner |
| -------- | -------------------------------------------------- | ------------------ |
| .1, .5   | ordinary queries and pre-materialization allowlist | tasks 02, 04       |
| .2, .6   | SQL eligibility, totals, observation guards        | tasks 01, 02, 03   |
| .3, .7   | bounded cache and responsive disclosure            | tasks 03, 04       |
| .4, .8   | inspect and existing transcript ownership          | tasks 02, 03, 04   |
| .9       | reproducible performance protocol                  | tasks 00, 04       |

All criterion suffixes refer to AC-TASKS-ERRAND-HISTORY-001. Existing tests include backend `service_session_summary_projection_test.go`, `errand_ws_handlers_test.go`, repository summary scenarios, web `use-task-errands.test.ts`, `errands-slice.test.ts`, `lib/ws/handlers/errands.test.ts`, `dockview-layout-restore.test.ts`, `dockview-layout-restore-maximize-session.test.ts` and desktop/mobile `e2e/tests/session/*errands.spec.ts`.

## Risks and unresolved measurement

No numeric timing or memory tolerance is invented: task00 must record baseline variance and numeric budgets before production work can start. A query predicate parity failure is a blocker, not justification for a full-list fallback. Query-plan/index cost and recovery predicate portability need proof in task01. No HAR evidence establishes ordinary transcript fan-out or settings-page regression. No runtime cleanup improvement is claimed. Public docs updates and already-implemented cleanup verification remain separately scoped; implementation must report browser history cost and runtime resource evidence separately.

## Implementation plans

[Bounded history package](../../../plans/errand-history-loading/plan.md) records the completed implementation, source identity, verification and explicit first-delivery performance disposition. The measured results retain the original thresholds and disclose fixed costs separately from history-growth bounds.
