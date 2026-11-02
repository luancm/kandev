---
id: "24-mixed-parent-browser-acceptance"
title: "Verify mixed-parent ordering and transitions"
status: done
wave: 24
depends_on:
  - "27-history-disclosure"
  - "29-conversation-navigation-and-history"
  - "13-adopt-reserved-launches"
  - "15-disabled-prepared-delivery"
  - "17-authoritative-summary-repair"
  - "18-fallback-session-identity"
  - "20-context-snapshot-freshness"
  - "22-preview-menu-isolation"
  - "23-visible-status-announcements"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-005
  - REQ-TASKS-ERRAND-ORDER-001
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-005.7
  - AC-TASKS-ERRANDS-004.13
  - AC-TASKS-ERRAND-ORDER-001.1
  - AC-TASKS-ERRAND-ORDER-001.2
  - AC-TASKS-ERRAND-ORDER-001.3
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.4
  - AC-TASKS-ERRANDS-011.7
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-panel-order.md
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
---

# Task 24: Verify mixed-parent ordering and transitions

## Scope and authority

Repairs review finding(s) task17 acceptance at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. The implementation and acceptance evidence are recorded below; the inherited route failure required the scoped conformance correction documented in Results.

## Low-level implementation and regression plan

This is additional contract coverage, not a new ordering feature. Keep the original Errands-plan Task17 ordering acceptance open until final desktop AND phone evidence exists; this is distinct from repair Task17 summary reconciliation. The user-approved Task29 parent-owned panel supersedes the former task-wide grouped panel. Test ordering within each parent and isolation when switching parents; never restore a task-wide list to satisfy historical assertions. Reuse the existing ordering implementation and independent timestamp/ID oracle.

Owned files: apps/web/e2e/helpers/errands.ts; apps/web/e2e/tests/session/errands.spec.ts and mobile-errands.spec.ts. Extend helper with a production-shaped same-task second parent fixture, or keep scenario-specific setup in the spec. Never call startErrands twice and call that mixed parents: it creates two different tasks.

Fixture algorithm:
1. Start one real task and parent A with two gated child objectives through the existing MCP scenario. Use actual returned IDs and wait for RUNNING.
2. On the SAME task launch normal parent B with two gated children. Keep the task within its five-live cap. Reuse Task29's mixed-parent fixture where possible.
3. Capture created_at and IDs from authoritative server data; order independent expectations by the documented timestamp/tie rules without importing the production ordering helper or sleeping to force timestamps.
4. Complete one child of each parent through real service paths. Each parent now has active and completed work. Do not inject status into Zustand or overwrite errand rows.
5. Open A's panel/drawer. Assert only A's children appear, active work precedes initially collapsed completed history, and expanding history reveals the expected finished rows. B's rows must remain absent. Verify each section's complete row order, not just its first row.
6. Settle A's remaining child while its panel stays open. Assert the active row moves to completed history and parent-scoped counts update. B remains active in the same task, so the established task-wide Stop-all action remains available. Switching to B shows only B's correctly ordered children; returning to A preserves access to its history.
7. Apply context metadata through the real fixture API and verify order does not change. Verify switching normal conversations, Open/Back, reload and reconnect keep the intended parent scope and deterministic ordering. Repeat both parents so isolation is not an accidental first-parent filter.
8. On native phone use the real drawer with enough completed history to scroll. Completed history starts collapsed; expand to inspect/order/navigate it. Keep one internal list scroller, fixed safe-area footer, 44px targets and zero document overflow. After all task errands settle, Stop-all disappears; neither parent's completed transcripts are deleted.

Regression names: 'desktop parent panels isolate errands and preserve ordering through settlement and switching'; 'phone parent drawer preserves scoped ordering through settlement and reload'. @covers AC-TASKS-ERRAND-ORDER-001.1/.2/.3 plus AC-TASKS-ERRANDS-010.4/.8, 011.2/.4/.7. Test-only contract coverage may already pass this HEAD; TDD explicitly permits that. Record it as coverage completion, not fictional production RED.

UI-ORDER-01, parent-owned hierarchy (Task29 supersedes the older task-wide groups):
~~~
Desktop: parent A popover       Phone: parent A bottom drawer
Errands for A                 +--------------------------------+
Active                        | Errands for A              [x] | fixed
  A-live              [Open]  | Active                         |
> Completed (1)               |   A-live                   >   | 44px row
  expand -> A-done     [Open]  | > Completed (1)                |
                              |   expand -> A-done         >   | list scrolls
[Stop all errands]            |--------------------------------|
                              | [       Stop all errands     ] | task-wide
                              +--------------------------------+
After A-live settles: A history contains both children.
B-live remains accessible only from B's panel; Stop-all remains available.
Switch to B: only B's active work and collapsed completed history appear.
~~~

~~~
(cd apps/web && pnpm exec vitest run lib/errand-panel-order.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/helpers/errands.ts e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --retries=0
~~~

The same final suites include Task 04's uncertainty lifecycle: live offered attempt has no warning, recovered unresolved current revision shows uncertainty, Open/Back reaches the retained child, and a trusted matching receipt clears the warning and stays clear after reload. Use an isolated backend fixture for the durable attempt state and the real DTO/event/list path; do not mutate Zustand or DOM to manufacture reception. Task 04's native-shaped journal/inbox tests prove receipt authority. Browser fixtures prove presentation and navigation, not real-provider reception. Include both desktop and native phone, zero-item/status-only negative controls and a stale receipt for a superseded revision.

Run final two whole Errands files after all changes that touch them; report discovered counts, actual passes/skips and exact log paths. Don't keep quoting 8/6 if new cases increase discovery count. A newly added test failing fixture gate/profile setup is not product RED.

## Dependencies and ownership

[Task 29](task-29-conversation-navigation-and-history.md), [Task 13](task-13-adopt-reserved-launches.md), [Task 15](task-15-disabled-prepared-delivery.md), [Task 17](task-17-authoritative-summary-repair.md), [Task 18](task-18-fallback-session-identity.md), [Task 20](task-20-context-snapshot-freshness.md), [Task 22](task-22-preview-menu-isolation.md), [Task 23](task-23-visible-status-announcements.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Pending. Record each final command, observed result, test count and log path; distinguish skips, infrastructure failures and reproduced defects from passes.


### Inherited phone route reliability observation

Task04C uncertainty acceptance passed desktop 1/1 and phone 1/1 on a diagnostic retry with matching frozen Task07 backend/helper artifacts. One intermediate native-phone attempt rendered Task unavailable despite task/session HTTP 200 responses; its cause is unknown. Preserve and investigate `/tmp/errands-task04c-run-quiet/kandev-run.e2e.FweWaGGr.log`; the passing retry `/tmp/errands-task04c-run-quiet/kandev-run.e2e.hd7kh0OH.log` included a resource 503 and WebSocket reconnect. Do not classify the route failure as fixed merely because retry passed, or weaken assertions. A read-only trace may narrow the next regression before this combined acceptance gate.


The read-only trace `/tmp/errands-phone-route-trace.md` narrows the failure to navigation after successful admission and delivery. The test opens `/t/<taskId>` without a session query but expects the explicitly launched parent; default route selection uses the primary or first owned session. Truncated response data does not prove the wrong session was selected, and the failed screenshot was overwritten by retry. Before navigation, capture the full primary-session ID and session-list identities alongside the expected parent. On failure, preserve the screenshot, visible chat IDs, selected session, route read/error state and hydration readiness in a unique attempt directory before retry. Keep the bounded wait and exact parent assertion. If fixture identity is inconsistent, repair the fixture; if identity is valid, reproduce the route/hydration failure with its existing stale-response guards intact. The passing retry's 503 and WebSocket reconnect do not establish causation. Root cause remains unknown; no production fix is accepted from this trace.


### Fixture integration boundary (2026-11-02)

Primary-owned extension: the existing isolated lifecycle delivery-fixture binary and mock-agent scenario gain test-only await recovery controls and an after-await file gate. These support the already-required live-offered/recovered/receipt browser assertions through repository and real list/DTO boundaries; no application endpoint or production contract is added. Final verification includes rebuilding the fixture/mock binaries and scoped backend checks. Initial desktop run has reproduced the inherited Task unavailable route failure; it is not accepted as a pass.


### Inherited route failure: confirmed cancellation handling defect

The recorded route reliability investigation reproduced Task unavailable on valid /t routes, with authoritative task/session responses and `AbortError: Task navigation read aborted` captured in `/tmp/errands-route-349a1017-c76e-4dc1-820c-b50db4bfa5b0.json`. The shared navigation reader treats cancellation as control flow, but useTaskDetails records the aborted same-task read as a permanent error. The integrating primary adds `task-page-content.tsx` and its existing test to this packet: reject obsolete cancellation before setting taskLoadError using the existing isTaskNavigationAbort predicate, preserving request identity fences and genuine permanent-error handling. Existing interactive-read requirements remain unchanged. Regression: authoritative route hydration followed by cancellation of an older read must retain the task without a permanent error. Verify the owning task-page-content/permanent-read and task-navigation-read suites, then the final desktop and native-phone files. This is a conformance correction required to complete the already-authorized route acceptance, with no new UI or persistence contract.

The owning reconnect tests also reproduce an existing stale reader capture: navigation ownership lasts across WebSocket generations, while its captured task-session reader does not. Both reconnect cases fail against unchanged HEAD (`/tmp/errands-task24-route-existing-baseline.log`). Include `lib/state/task-navigation-reads.ts` in the same conformance fix, resolving the current session reader per navigation identity read rather than at owner construction. Preserve scope fences and assert real session refresh after reconnect. The first existing reconnect fixture also needs its missing listTaskSessions response.

### Final acceptance (2026-11-02)

Final retries-disabled browser runs passed 14/14 desktop and 12/12 phone tests with no skips. Coverage includes both parent scopes, active and completed ordering, live transitions, retained navigation, reconnect, durable uncertainty, stale and matching native receipts, phone scrolling and footer reachability. The route correction passed 49 frontend unit tests. Backend fixture race selectors passed four mock-agent tests and three native-receipt tests with no skips; scoped Go and web lint, formatting and whitespace checks passed. These isolated SQLite/mock/native-adapter checks do not certify PostgreSQL or real providers.

Receipts: `/tmp/errands-task24-final-receipts/` contains desktop/phone logs, screenshots, backend JSONL results, lint output and source hashes. Exact final browser logs are `/tmp/kandev-run.e2e.CNpeojz6.log` and `/tmp/kandev-run.e2e.8VWyjr8R.log`; frontend unit results are `/tmp/kandev-run.test.YEivcHPV.log`. Task29's parent-scoped navigation contract supersedes the original Task17 task-wide grouping wording. Task24 closes original Task17 acceptance, but does not close Task25 dark promotional media.
