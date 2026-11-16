---
created: 2026-11-10
status: done
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-001
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRAND-COMPLETION-001
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/startup-housekeeping.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-result-duration.md
legacy_specs: []
---

# Finish the remaining Errands behavior

## Outcome and current state

The finishing source checkpoint is `ee0f077cb9d8061cf17437021f087326978aed05`, incorporating upstream main `585824a5d0`. It adds bounded history, child-cache release, exact late-tool replay and shared-summary publication to the earlier delivered startup, cleanup, duration and panel fixes. The final upstream checks passed 605 top-level Go cases with three opt-in fixture skips; existing 459-test web evidence, typecheck/localization, affected-package lint and rebuilt desktop/phone checks pass. The matching-main cohort passed 28/28 scenarios with all eight latency measures within their unchanged warning boundaries. Fixed heap overhead and the earlier two strict-limit misses are explicitly dispositioned in the [final performance report](../errand-history-loading/summary-followup-handoff.md). [Final integration evidence](final-integration-handoff.md) records the checks; the Kandev plan owns the remote push receipt. The [isolated bounded-use pilot](bounded-pilot-receipt.md) is complete with qualified outcomes; the [quality package](../errands-simplification/plan.md) owns continuing maintenance and final verification. No main-instance update is inferred.

### Earlier delivery receipts

All four implementation groups are on `feature/plan-errands-first-d-424`: exact parent notification delivery, truthful recovery attention and safe continuation, immutable historical elapsed time, and durable successful runtime cleanup with retained conversations. The user requested direct primary execution on 2026-11-12; no additional agents were launched. Source checkpoint `452c9d4f0d3bc763ccfc43661ee91f7282425de0` is pushed and remote-verified. It includes `kdlbs/kandev` main through `37f081a976526de1a6967f8714e72e247ec0b386`, merged at `e35b7f6e23` after the earlier 80-commit upstream integration at `0067adc210`.

Fresh managed browser acceptance passed all 21 desktop and 19 phone cases. Follow-up verification corrected lifecycle-only turn selection and exact-authorized cold resume without a retained runtime row. Old browser fixture expectations now require completed child state. The gateway fixture now supplies its real Hub and verifies authenticated subscription identity; its full race suite passed. A failing lifecycle case also exposed literal native filenames being interpreted as URI syntax; both SQLite factories now reuse the existing escaping helper, with independent writer/reader regressions and a passing lifecycle rerun. Affected backend race suites, SQL guard, store conformance, frontend typecheck and 228 tests, translation/wait guards and the normal optimized build passed. Final delivery evidence belongs in [the integration handoff](integration-handoff.md), which owns exact commands, counts, limits and checkpoint history.

No deployment, live database repair, transcript deletion, worktree removal or universal provider certification is included.

## Completed: load finished Errands on demand

Keep ordinary task loading approximately constant as completed Errands accumulate. This work addresses the Errands regression, with general asset compression, bundle splitting and unrelated optimization excluded. The [investigation](loading-performance-investigation.md) records the supplied HAR: 107 Errands account for about 243 KB of the parent's 250 KB session list, compared with 7.2 KB for its three ordinary sessions, and historical children enter boot and normal session reconciliation. This is payload/workload evidence, not a before/after latency or memory benchmark.

Implemented outcomes:

- Load compact active, queued and needs-attention summaries immediately. Preserve truthful counts, capacity, unread/result indicators and Stop/Stop-all semantics without enumerating finished child sessions.
- Fetch finished-history summaries asynchronously when the user opens history and paginate larger histories. An eager background fetch of all completed Errands does not satisfy this outcome.
- Load a child's transcript, session detail and associated subscriptions only when opened. Direct finished-child links load the selected child and minimal parent identity without fetching all history.
- Exclude finished children from ordinary-session boot/list reconciliation and prevent Errand panels from materializing during saved-layout restoration. Preserve ordinary sibling tabs, parent ownership and explicit child navigation.
- Render existing result cards from saved content, preserving immutable duration and legacy omissions without fetching the child conversation.
- Reuse the desktop popover and phone drawer/full-screen child composition. Provide localized loading, empty, retry and next-page states; preserve internal scrolling, safe areas, touch targets and Back navigation. Coalesce duplicate reads and discard stale task/parent/page responses.

The user approved proceeding after reviewing [the impact walkthrough](loading-impact.html). The [history package](../errand-history-loading/plan.md) Tasks00–04 are implemented and accepted, with source preserved in `dcc3de55a2` and integrated through `ee0f077cb9`. Ordinary sessions retain their existing loading behavior; finished history and child details load on demand. The [second paired cohort](../errand-history-loading/paired-performance-v2-handoff.md), [original main comparison](../errand-history-loading/main-comparison-handoff.md) and [final main comparison](../errand-history-loading/summary-followup-handoff.md) preserve all observations. The final report explicitly dispositions the fixed costs and earlier strict-limit misses. No failed measurement has been removed or relabelled as passing.

The [late-tool replay work order](../errand-tool-terminal-replay/task-01-exact-tool-replay.md) is implemented and verified, with [exact evidence](../errand-tool-terminal-replay/implementation-handoff.md). The observed completed review was blocked behind a missing-identity tool cancellation. Repair requires exact retained source evidence and an independent real terminal; historical records without that proof remain unresolved. The earlier launch hold is historical: the [isolated pilot](bounded-pilot-receipt.md) exercised the integrated repair. The unidentified historical record and main-instance deployment remain outside that acceptance.

History acceptance is complete with the disclosed fixed-cost exceptions in the final performance report:

- [x] Compare the same isolated task with zero, 100 and a larger completed-Errand history, holding ordinary sessions, build, network and cache conditions fixed. Measure boot/list bytes and request counts, session reconciliation work, panel creation/removal, time to usable parent conversation, and retained subscriptions/memory after repeated navigation.
- [x] Keep ordinary reload/task-switch work bounded independently of finished-history size, with history requests following explicit demand and pagination. Establish timing/memory tolerances from the controlled baseline before accepting the implementation; no unmeasured performance claim counts as completion.
- [x] Prove desktop/phone reload, warm switching, history loading/pagination/retry, direct historical links, self-contained result cards, live status and Stop, stale-response isolation and absence of temporary Errand tabs. Validate runtime cleanup separately from retained-history UI loading.

## Delivery and acceptance

| Deliverable                         | Implemented behavior                                                                                                                                                                               | Work order and checkpoint                                                                                                   |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Parent terminal identity            | Exact admission attributes a terminal missing its turn ID; eligible stored effects reconcile without guessing, and Auto-run composes and dispatches the empty server notification once             | [Terminal identity](task-02-parent-terminal-identity.md), `a96adeec38`                                                      |
| Recovery attention and continuation | Unresolved recovery remains visible and consumes capacity; unsafe input is refused without side effects; explicit follow-up requires exact settlement                                              | [Recovery settlement](../errand-orphan-settlement/task-01-recovery-settlement.md), `f71b9e09d4`, integrated at `67b6b9ba9d` |
| Successful completion cleanup       | Result and exact completion intent precede physical stop; close, receipt and completed projection retry independently; cancellation and newer work retain ownership                                | [Completion cleanup](../errand-completion-cleanup/task-01-completion-cleanup.md), `4becbc47a1` plus integration corrections |
| Historical result duration          | Optional measured elapsed time is frozen in delivered metadata, including zero; legacy or invalid measurements are omitted; collapsed and expanded history retain it after reload and continuation | [Result duration](task-01-result-duration.md), `8ed56d94ef`                                                                 |

The completed source also preserves normal picker/handoff exclusion, phone navigation, parent conversation ownership, flag-off retention, Send Now accounting and native-resume behavior from upstream. Ordinary completed, failed and cancelled sessions do not gain Errand continuation permission.

## Execution boundaries

The requested [code quality and redesign roadmap](code-quality-roadmap.md) is a follow-up to this completed delivery, alongside the bounded-use pilot. Its first bounded audit selected [private browser-state removal and explicit lifecycle integration](../errands-simplification/plan.md), with contract-test strengthening before the backend refactor. It does not reopen accepted implementation work orders or claim that functional verification proves maintainability.

The primary owns integration judgment and final delivery. Earlier Lunaldinho implementation and Era Sol reviews are historical accepted inputs, recorded in their handoffs; they are not active work. The current task plan records the user-authorized Coordinate assignments and one-writer ownership for the follow-up. Ancient running labels are not evidence of a live writer or a new implementation obligation. Supported cancellation was used where available; no direct SQL repair of historical live records was performed.

Heavy checks run sequentially under `/tmp/kandev-errands-repair-build.lock`, with two Go workers and one browser worker. Test builds disable inlining only for the large orchestrator package; a separate normal optimized backend build verifies production compilation. Disk headroom is monitored, and rebuildable caches are cleared only between builds or after hooks when no compiler/linter owns them. PostgreSQL without a DSN remains an explicit skip.

## UI acceptance

Existing surfaces carry these outcomes on desktop and phone:

```text
Parent history
  > 1 errand finished (recorded elapsed time)
  expanded: title     Done (recorded elapsed time)     Open

Recovery row / opened child
  Recovery needs verification
  Existing retained conversation and refusal explanation

Successful retained child
  Completed
  Retained transcript; explicit follow-up uses existing composer
```

The localized shared disclosure owns history. The phone retains its bottom drawer, full-screen child, Back action, touch targets and composer. Browser checks cover keyboard/touch disclosure, Open/Back, reload, passive inspection without runtime creation, explicit continuation and unchanged earlier result metadata. Long-duration phone screenshots were inspected; an unrelated update toast covers part of one child header, so that screenshot alone is not evidence of an unobstructed header.

## Acceptance ledger

- [x] Exact ordinary-parent terminal attribution and repair, including automatic composed notification dispatch without Send Now.
- [x] Recovery attention, capacity accounting, localized refusal and supported explicit continuation after exact settlement.
- [x] Success-close ordering, cancellation/successor isolation, retry, retained completed state and conservative legacy repair are implemented.
- [x] Historical recorded duration, legacy compatibility and immutable history.
- [x] Fresh desktop 21/21 and phone 19/19 acceptance, including completed-child continuation and repeated native resume.
- [x] Final affected backend/frontend/build checks and completion records; normal-hook delivery and remote verification are recorded in the task plan and integration handoff.

## Evidence and limits

[Integration checkpoints](integration-handoff.md) records final acceptance and previous delivery receipts. [Cleanup backend handoff](../errand-completion-cleanup/backend-handoff.md), [terminal identity handoff](parent-terminal-identity-handoff.md), [notification acceptance](notification-acceptance-handoff.md), [duration implementation](duration-handoff.md) and [duration browser evidence](duration-browser-handoff.md) retain the earlier RED/GREEN and review history.

Managed browser runs use isolated local fixtures and a mock provider. They prove the tested integration and retained conversation behavior; they do not certify every provider/executor or repair unknown historical live evidence. Missing/conflicting identity remains held. No claim of a machine OOM or power outage is made from an interrupted Kandev process alone.
