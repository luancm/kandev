# Recovered Task03 review, 2026-11-14

Recovered read-only from the live SQLite transcript for Errand `bc22088f-2849-49e5-bb8c-3ba4c7b89496`. Its final text was persisted before delivery failed. This is the reviewer’s source inspection, not independently rerun validation.

Task03 is **not ready for acceptance**. Read-only inspection found three blockers:

1. **P1: Finished full rows accumulate outside the bounded history cache.** [errands-slice.ts:205](apps/web/lib/state/slices/errands/errands-slice.ts:205) replaces every previously observed live row with its terminal snapshot and never evicts it. Repeated live → finished cycles therefore retain unlimited full `Errand` objects, including result data, in `byTaskId`. Per-row revision entries also accumulate for unseen terminal events. The existing replacement test explicitly expects terminal retention. **Minimal fix:** separate operational retention from compatibility-list state, evict finished operational rows when no explicit detail owner needs them, and bound revision bookkeeping. Add repeated-cycle coverage, not just unseen-terminal coverage.

2. **P1: Loaded history survives authorization-scope changes.** [use-task-errands.ts:245](apps/web/hooks/domains/session/use-task-errands.ts:245) cleans up history on owner/task/parent changes, but not read-scope changes. [selectors.ts:132](apps/web/lib/state/slices/errands/selectors.ts:132) exposes retained chains without checking their observation scope. Changing authentication/workspace generation can consequently leave old rows displayed; a denied summary refresh does not remove them. **Minimal fix:** synchronously invalidate scoped browser state and leases when scope changes, and fence selectors against mismatched scope. Cover already-loaded pages and outstanding requests, including denial.

3. **P2: Selected-child inspection masks subsequent live updates.** `selectErrandBrowserRowBySessionId` prioritizes `detailBySessionId`; [use-errand-view-model.ts:58](apps/web/components/task/chat/errands/use-errand-view-model.ts:58) reuses that inspection indefinitely. WebSocket reconciliation updates summary/history rows but never the inspection row. A child opened while running can therefore retain running status and Stop after settlement, or stale recovery/result indicators. **Minimal fix:** reconcile retained inspection projections with current observations, preserving immutable inspection identity and owner lifetimes. Add selected-child running → finished and recovery-resolution tests.

The five source hashes and all three screenshot hashes match the work-order Results exactly. I visually inspected the dark desktop-summary and phone images: they show summary counts, history range controls and the phone’s fixed footer/internal list composition. Screenshots establish appearance, not race handling or navigation correctness.

I inspected the focused tests and both new browser specs; **I did not rerun tests, lint, typecheck or builds**. The reported 176 tests, managed desktop/phone passes and raw recapture remain author-recorded evidence. No application browser `listErrands()` callers were found; remaining calls are E2E helpers/tests. The page chain enforces four retained pages, and matching-parent/shared-owner handling is present.

Generated pseudo/Traditional Chinese changes appear consistent with catalog regeneration; no blanket restoration is warranted.

Task04 still owns the frozen 0/100/1000 performance matrix and broader rendered compatibility evidence, including Back/anchor restoration, retry/refresh/empty states and viewport/focus checks. “Deferred to Task17” is an inaccurate dependency in this five-order package: existing global ordering must remain preserved, with remaining browser proof assigned to Task04.

I reviewed the recovered implementation and recorded evidence without edits or execution; the three correctness/retention findings above block Task03 acceptance.
