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

[Errands design map](errands.md#errands-design-map) lists shared contracts and companion ownership.

## Scope and requirement mapping

Extend the task-owned panel from [Errands](errands.md) with passive per-child context information. Row composition, session-specific data and accessibility satisfy AC-001.1/001.4; shared context interpretation and restoration satisfy AC-001.2; existing workspace delivery and disabled detail subscriptions satisfy AC-001.3. The depth-one/shared-workspace and workflow boundaries in [the Errands ADR](../../../decisions/2026-10-23-errands-first-class-delegation.md) remain unchanged.

The opened errand already receives `TokenUsageDisplay` through the shared `TaskChatPanel` and input toolbar. The addition belongs in `errands-panel-row.tsx`, shared by the popover and drawer. Keep the panel as the management surface and historical transcript cards as delivered history.

## Data and existing transport

The panel row reads its own `ErrandBrowserRow.metadata` from the existing summary or history response. Workspace-scoped updates refresh those known rows and preserve the passive panel contract. The panel does not create transcript subscriptions or additional context requests. Ordinary session context continues through its session metadata and transport.

Percentage means `used / size`, using the same rounding as `TokenUsageDisplay`. It is context occupancy, independent of cumulative token/cost accounting. Use the capacity and source associated with the observation. A settled Errand retains the last reading associated with its current harness binding.

## Persisted summary projection

Project admitted context events from the Errand delivery stream into compact metadata on the matching harness binding. Persistence shares the event projection transaction. The browser reads the latest binding of the current Errand execution generation. A successor without an observation starts without a reading. A late event can update the binding that admitted it, while the browser keeps the current binding’s reading.

Use the existing compact context keys `size`, `used`, `remaining`, `efficiency`, `source`, and `timestamp`. Preserve measured, zero-used, and exactly-full values. An observation with an invalid size or usage clears the reading, as does a current binding without a sample during a fresh read.

Missing and cleared values yield no reading on a fresh load. An explicit null remains authoritative when reconciling an existing browser row. Compact payload size and selected-conversation behavior remain verification requirements.

## Shared presentation rules

Factor only the existing size/used reliability and pending/percentage derivation out of `components/task/chat/token-usage-display.tsx` into a dependency-neutral pure helper, `lib/context-window-usage.ts`. Both the existing widget and row consume that helper. Preserve the existing widget's output, token formatter, color thresholds and tooltip interaction. No helper imports UI components or store implementations.

Reuse the current predicate semantics: size must be positive and used must not exceed size. Missing data or a failed reliability predicate hides the row line; size with used zero uses explicit unmeasured text; used equal to size is valid. Calculate the percentage from size/used, rather than trusting a separately reported efficiency field. Continue respecting the existing reset invalidation and explicit-null rules; a clear must not be replaced with the previous persisted fallback.

Prefer existing localized task keys for the context label, measured percentage and unmeasured state. If compact copy needs a new key, add it to every shipped locale and pseudo, following the normal Traditional Chinese conversion. The readout is neutral secondary text and does not add another status announcer.

## Desktop and phone composition

Add the readout below title/profile/status in `RowSummary`. Keep desktop Open separate. Derive the context view once for each `ErrandsPanelRow` and pass it to its presentation branches. The phone `DrawerRow` is already a whole-row button; an interactive `TokenUsageDisplay` inside it would nest buttons. Use passive text instead. Associate the emitted text with the phone row using `aria-describedby` when present, so the existing explicit Open accessible name does not hide context information from assistive technology.

The existing Errands drawer is the mobile exemplar: temporary parent-scoped navigation, one internal list scroller, a fixed bulk-action footer and safe-area clearance. Preserve its 44px row/actions, including 767px fine-pointer layouts. Allow the context line to wrap within the row; do not introduce horizontal scrolling, a second overlay or another tap target. Open retains the existing full-screen child view and Back. Source and compaction details stay in that child's existing widget.

## Verification and compatibility

Use pure helper cases and existing hook/widget tests for known, unmeasured, unavailable, impossible, exactly-full, persisted, updated and explicitly cleared readings. Test two children with different samples and a different parent sample. The passive hook path must keep child detail subscriptions at zero while still reacting to shared data.

Browser proof uses real Errands with samples on their owned harness bindings. Leave the conversations closed while checking reload, a subsequent sample, and an explicit clear in the same panel. The fixture supplies provider samples through the owned delivery pipeline and checks persisted readback. A synthetic context clear verifies the panel’s invalidation behavior.

| Existing telemetry              | Panel behavior                                   | Evidence                                                |
| ------------------------------- | ------------------------------------------------ | ------------------------------------------------------- |
| ACP effective window            | Child's measured percentage, or unmeasured state | Helper/widget tests and workspace-event browser fixture |
| Existing API catalogue fallback | Same interpretation of the supplied sample       | Existing source tests plus shared helper                |
| Missing or unreliable window    | Omit the line                                    | Helper and panel browser cases                          |
| Persisted completed child       | Retain supplied sample after reload              | Errands desktop/phone browser cases                     |
| Explicit null clear             | Remove stale line until new data                 | Hook/event regressions and persisted browser clear      |

This change does not extend provider certification. The [work order](../../../plans/errands/task-15-context-window.md) owns implementation and exact checks.

## Used-token and capacity refinement

Replace the row percentage-only label with compact used/capacity token counts and a thin passive occupancy bar. Reuse the existing token-widget formatter convention and localized number formatting; preserve reliability, unmeasured, full-window and clear semantics. Keep original numeric values available for accessible descriptions. The bar is decorative (`aria-hidden`) because its reading is already described; it adds no focus target. Use a neutral track and existing context-widget occupancy color thresholds, with tabular numerals and wrapping counts. No percentage-only fallback when token counts are known. Do not change the opened widget or backend projection.

Desktop and phone retain the current row hierarchy: title, profile/status, token counts, thin bar. The phone drawer remains the single list scroller with fixed footer and one row Open target. Missing readings add no line; unmeasured readings include known capacity and explicit wording, without a measured bar. Capture fixtures persist each Errand’s own context through its admitted delivery stream. The dark-mode storyline shows a measured active child, parent follow-up while that child runs, parent response and subsequent delivery. Seeded telemetry is disclosed as synthetic demo data.

## Authoritative snapshot reconciliation

Accepted context updates use the workspace-scoped `errand.updated` notification. A sample or explicit null updates the metadata of the known compact row. An omitted context payload preserves its prior value. Metadata updates advance the browser observation revision so a delayed summary, history, or inspection response cannot overwrite a newer sample. The same update keeps a current loaded summary and loaded history rows fresh. The row consumes the compact metadata directly. The [recovery design](errand-recovery-and-delivery.md) and [repair work order](../../../plans/errands-review-repairs/task-20-context-snapshot-freshness.md) define the relevant freshness and regression cases.
