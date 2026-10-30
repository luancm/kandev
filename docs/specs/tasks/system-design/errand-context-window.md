---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRANDS-CONTEXT-001
created: 2026-10-30
owners:
  - kandev
---

# Errand Context Window Design

## Scope and requirement mapping

Extend the task-owned panel from [Errands](errands.md) with passive per-child context information. Row composition, session-specific data and accessibility satisfy AC-001.1/001.4; shared context interpretation and restoration satisfy AC-001.2; existing workspace delivery and disabled detail subscriptions satisfy AC-001.3. The depth-one/shared-workspace and workflow boundaries in [the Errands ADR](../../../decisions/2026-10-23-errands-first-class-delegation.md) remain unchanged.

The opened errand already receives `TokenUsageDisplay` through the shared `TaskChatPanel` and input toolbar. The addition belongs in `errands-panel-row.tsx`, shared by the popover and drawer. Keep the panel as the management surface and historical transcript cards as delivered history.

## Data and existing transport

Use `useSessionContextWindow(errand.errand_id, false)`. It selects `contextWindow.bySessionId`, hydrates a missing entry from `taskSessions.items[childId].metadata.context_window` and reads the session's persisted compaction count. The false detail flag prevents `subscribeSession(childId)`; this is a local read of shared session data. Mount this read only with the existing panel rows. Do not create a second loader or add context fields to the Errand DTO.

Existing backend context handling in `internal/agent/orchestrator/event_handlers_git.go` publishes `session.state_changed`; `internal/gateway/websocket/task_notifications.go` broadcasts that event to the workspace. Frontend `lib/ws/handlers/agent-session.ts` already consumes `metadata`/`session_metadata`, updating or clearing the per-session cache. Task-session summary DTOs expose a bounded metadata projection, not full session metadata. Extend that existing projection for reload as described below. No new route, schema, event or producer change is needed.

Percentage means `used / size`, using the same rounding as `TokenUsageDisplay`. It is context occupancy, independent of cumulative token/cost accounting. Preserve the supplied window and ACP/API source semantics; do not infer a missing size from another child or model catalogue request. A completed errand retains its latest available session reading and can receive a later reading after follow-up.

## Persisted summary projection

The task system owns both persisted session telemetry and the task-session summary consumed by the passive hook. Preserve the compact-summary boundary in [viewport activation](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md): inactive children do not acquire detail streams. Extend `internal/task/repository/sqlite/session_summary.go` and the fallback `taskSessionSummaryObservation` in `internal/task/service/service_sessions.go`, carrying context through the existing observation and DTO metadata map. Do not serialize full metadata or add an Errand DTO field.

Project only `context_window` scalar fields consumed by the existing parser (`size`, `used`, `remaining`, `efficiency`, `source`, `timestamp`) and the scalar `context_compaction_count`. Use SQL dialect helpers for SQLite and PostgreSQL. Build a fixed-field context object rather than selecting an arbitrary provider object; unrelated metadata, nested payloads and provider catalogues must remain excluded. Preserve measured, zero-used and exactly-full values without inferring a size. Invalid shapes yield unavailable context rather than failing session-list hydration. Keep existing model/error projection behavior and authorization intact.

Missing context and persisted explicit null must yield no reading on a fresh load. An explicit null must remain authoritative when reconciling an existing client cache; do not accidentally revive a prior value through metadata merge or hook fallback. Preserve workspace-event clear semantics and test reload following clear. Repository and service fallback paths must agree on the projected field set and clear behavior. The existing metadata wire shape remains compatible; no storage migration or extra request is needed. The projection applies wherever this shared summary is used, so compact payload-size and selected-session detail regressions remain required.

## Shared presentation rules

Factor only the existing size/used reliability and pending/percentage derivation out of `components/task/chat/token-usage-display.tsx` into a dependency-neutral pure helper, `lib/context-window-usage.ts`. Both the existing widget and row consume that helper. Preserve the existing widget's output, token formatter, color thresholds and tooltip interaction. No helper imports UI components or store implementations.

Reuse the current predicate semantics: size must be positive and used must not exceed size. Missing data or a failed reliability predicate hides the row line; size with used zero uses explicit unmeasured text; used equal to size is valid. Calculate the percentage from size/used, rather than trusting a separately reported efficiency field. Continue respecting the existing reset invalidation and explicit-null rules; a clear must not be replaced with the previous persisted fallback.

Prefer existing localized task keys for the context label, measured percentage and unmeasured state. If compact copy needs a new key, add it to every shipped locale and pseudo, following the normal Traditional Chinese conversion. The readout is neutral secondary text and does not add another status announcer.

## Desktop and phone composition

Add the readout below title/profile/status in `RowSummary`. Keep desktop Open separate. Derive the context view once for each `ErrandsPanelRow` and pass it to its presentation branches. The phone `DrawerRow` is already a whole-row button; an interactive `TokenUsageDisplay` inside it would nest buttons. Use passive text instead. Associate the emitted text with the phone row using `aria-describedby` when present, so the existing explicit Open accessible name does not hide context information from assistive technology.

The existing Errands drawer is the mobile exemplar: temporary task-wide navigation, one internal list scroller, a fixed bulk-action footer and safe-area clearance. Preserve its 44px row/actions, including 767px fine-pointer layouts. Allow the context line to wrap within the row; do not introduce horizontal scrolling, a second overlay or another tap target. Open retains the existing full-screen child view and Back. Source and compaction details stay in that child's existing widget.

## Verification and compatibility

Use pure helper cases and existing hook/widget tests for known, unmeasured, unavailable, impossible, exactly-full, persisted, updated and explicitly cleared readings. Test two children with different samples and a different parent sample. The passive hook path must keep child detail subscriptions at zero while still reacting to shared data.

Browser proof extends the existing desktop/phone Errands specs. Create real errands and settle the fixture children without opening them. Seed existing child metadata with `ApiClient.seedTaskSession(taskId, { sessionId: childId, ... })`, preserving state, completion timestamp and error fields. The existing test harness persists the metadata and broadcasts a real workspace session event. Demonstrate persisted reload, a subsequent sample while the panel remains open, and explicit `context_window: null` clearing across reload. Do not use the context-source helper's direct-store injection or its overridden clear handler for this proof. Synthetic metadata clear proves the consumer's invalidation behavior; it does not recertify the production reset command.

| Existing telemetry | Panel behavior | Evidence |
| --- | --- | --- |
| ACP effective window | Child's measured percentage, or unmeasured state | Helper/widget tests and workspace-event browser fixture |
| Existing API catalogue fallback | Same interpretation of the supplied sample | Existing source tests plus shared helper |
| Missing or unreliable window | Omit the line | Helper and panel browser cases |
| Persisted completed child | Retain supplied sample after reload | Errands desktop/phone browser cases |
| Explicit null clear | Remove stale line until new data | Hook/event regressions and persisted browser clear |

This change does not extend provider certification. The [work order](../../../plans/errands/task-15-context-window.md) owns implementation and exact checks.

## Used-token and capacity refinement

Replace the row percentage-only label with compact used/capacity token counts and a thin passive occupancy bar. Reuse the existing token-widget formatter convention and localized number formatting; preserve reliability, unmeasured, full-window and clear semantics. Keep original numeric values available for accessible descriptions. The bar is decorative (`aria-hidden`) because its reading is already described; it adds no focus target. Use a neutral track and existing context-widget occupancy color thresholds, with tabular numerals and wrapping counts. No percentage-only fallback when token counts are known. Do not change the opened widget or backend projection.

Desktop and phone retain the current row hierarchy: title, profile/status, token counts, thin bar. The phone drawer remains the single list scroller with fixed footer and one row Open target. Missing readings add no line; unmeasured readings include known capacity and explicit wording, without a measured bar. Capture fixtures must persist child-specific readings through existing seedTaskSession metadata, preserving lifecycle fields. The dark-mode storyline shows a measured active child, parent follow-up while that child runs, parent response and subsequent delivery. Seeded telemetry is disclosed as synthetic demo data.
