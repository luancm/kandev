---
created: 2026-11-10
status: done
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRAND-COMPLETION-001
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-result-duration.md
legacy_specs: []
---

# Finish the remaining Errands behavior

## Outcome and current state

All four implementation groups are on `feature/plan-errands-first-d-424`: exact parent notification delivery, truthful recovery attention and safe continuation, immutable historical elapsed time, and durable successful runtime cleanup with retained conversations. The user requested direct primary execution on 2026-11-12; no additional agents were launched. Source checkpoint `452c9d4f0d3bc763ccfc43661ee91f7282425de0` is pushed and remote-verified. It includes `kdlbs/kandev` main through `37f081a976526de1a6967f8714e72e247ec0b386`, merged at `e35b7f6e23` after the earlier 80-commit upstream integration at `0067adc210`.

Fresh managed browser acceptance passed all 21 desktop and 19 phone cases. Follow-up verification corrected lifecycle-only turn selection and exact-authorized cold resume without a retained runtime row. Old browser fixture expectations now require completed child state. The gateway fixture now supplies its real Hub and verifies authenticated subscription identity; its full race suite passed. A failing lifecycle case also exposed literal native filenames being interpreted as URI syntax; both SQLite factories now reuse the existing escaping helper, with independent writer/reader regressions and a passing lifecycle rerun. Affected backend race suites, SQL guard, store conformance, frontend typecheck and 228 tests, translation/wait guards and the normal optimized build passed. Final delivery evidence belongs in [the integration handoff](integration-handoff.md), which owns exact commands, counts, limits and checkpoint history.

No deployment, live database repair, transcript deletion, worktree removal or universal provider certification is included. The unrelated untracked `loading-performance-investigation.md` remains preserved outside this delivery.

## Delivery and acceptance

| Deliverable | Implemented behavior | Work order and checkpoint |
| --- | --- | --- |
| Parent terminal identity | Exact admission attributes a terminal missing its turn ID; eligible stored effects reconcile without guessing, and Auto-run composes and dispatches the empty server notification once | [Terminal identity](task-02-parent-terminal-identity.md), `a96adeec38` |
| Recovery attention and continuation | Unresolved recovery remains visible and consumes capacity; unsafe input is refused without side effects; explicit follow-up requires exact settlement | [Recovery settlement](../errand-orphan-settlement/task-01-recovery-settlement.md), `f71b9e09d4`, integrated at `67b6b9ba9d` |
| Successful completion cleanup | Result and exact completion intent precede physical stop; close, receipt and completed projection retry independently; cancellation and newer work retain ownership | [Completion cleanup](../errand-completion-cleanup/task-01-completion-cleanup.md), `4becbc47a1` plus integration corrections |
| Historical result duration | Optional measured elapsed time is frozen in delivered metadata, including zero; legacy or invalid measurements are omitted; collapsed and expanded history retain it after reload and continuation | [Result duration](task-01-result-duration.md), `8ed56d94ef` |

The completed source also preserves normal picker/handoff exclusion, phone navigation, parent conversation ownership, flag-off retention, Send Now accounting and native-resume behavior from upstream. Ordinary completed, failed and cancelled sessions do not gain Errand continuation permission.

## Execution boundaries

The primary owns integration judgment, documentation, tests and final delivery. Earlier Lunaldinho implementation and Era Sol reviews are historical accepted inputs, recorded in their handoffs; they are not active work. The user explicitly replaced delegation with direct primary execution. Ancient running labels are not evidence of a live writer or a new implementation obligation. Supported cancellation was used where available; no direct SQL repair of historical live records was performed.

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
