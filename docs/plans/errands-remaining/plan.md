---
created: 2026-11-10
status: in_progress
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

Current checkpoint, 2026-11-11: parent terminal correction `a96adeec38` and historical duration `8ed56d94ef` are committed and pushed. Recovery checkpoint `f71b9e09d4` is integrated in the resolved index, with the sole conflict preserving both attention reason and elapsed duration. Combined acceptance passed: 128 primary and 13 additional backend race tests, SQL guard, race store conformance, web typecheck, 162 Vitest tests, and 3/3 desktop plus 3/3 phone managed browser cases. PostgreSQL and the environment-gated standalone browser fixture skips are explicit. Era Sol `523eba58` found no integration issues; parent verified all nine receipt hashes and seven reviewed source blobs. Normal merge hooks passed without bypass; merge `67b6b9ba9d` is committed and pushed with remote SHA verified.

On 2026-11-12 the user requested direct primary execution. Supported cancellation of implementation Errand `6968f572` preserved its code; no replacement was launched. The primary completed durable success cleanup, cancellation/successor fencing, storage and projection retry, retained completed-session composition and continuation. Cleanup checkpoint `4becbc47a1f38d869f5e8b90bce44cc019a9bd6b` passed normal hooks without bypass. Evidence before that checkpoint includes the full Errands service race suite, 376 affected backend race tests with 23 explicit skips, 12 cleanup regressions after the final successor-cancellation fix, scoped lint with zero issues, SQL guard/store conformance, web typecheck, 11 frontend tests, i18n and formatting. Final integrated race/browser acceptance remains required.

Fetched upstream `dab0d02e4571c147adf8b9c193db595dbafdd190` adds 80 commits beyond the prior shared base. All 17 conflicts are resolved and staged, preserving exact terminal repair, successful cleanup, Errand isolation, upstream session-state convergence, worktree guidance and quota-wait summaries. Frozen dependencies were refreshed with pnpm 9.15.9. The merged backend build passed. Integrated race, lint, persistence, web and desktop/phone acceptance is running; the merge commit and final push remain pending. Disk headroom is monitored and only unreferenced stale compiler scratch is removed. No deployment or live database repair is included.

The following interruption audit and older dispatch entries are historical evidence; the checkpoint above governs current ownership and acceptance.

Interruption audit, 2026-11-11 09:10 UTC: Kandev restarted at 01:18 and abandoned terminal-identity, duration-browser and recovery-frontend turns. The host reports 13 days uptime; a machine power outage is unconfirmed. No test/compiler/browser processes remained, the shared build lock was free, saved edits remained intact and disk had 37 GiB free. Direct resume of the two parent-owned workers was refused by unresolved exact settlement; supported cancellation succeeded for both. Replacement Lunaldinho workers are `c10897a4-a87c-4ff5-a418-7a07bb4fe009` (terminal identity) and `8671c3be-404e-4c39-9191-a1dcf6ed39ad` (duration browser). The recovery coordinator was resumed to recover its frontend worker under the same single-writer policy. Ancient merge Errand `1a1282fd` still refuses cancellation due to unresolved admission; its merge was already delivered, so it remains a historical non-writing artifact rather than duplicate work. No live SQL repair was performed. Earlier worker IDs below are historical; interrupted validation is not counted as passed.

The user authorized all three remaining deliverables on 2026-11-10 and selected Lunaldinho for implementation Errands and Era Sol Que Me Faltava for reviews. This primary session owns design, integration, scope and delivery. Completed means implementation, required regression/browser checks, review disposition and integration into the parent feature branch; a delegated task finishing or a partial commit is not complete acceptance.

The parent branch contains the tested core recovery fix at `b2bbe9d5b7`. The recovery subtask `866daa42-1b11-47c8-ac03-2ab4e37362b0` owns attention/refusal/continuation and its acceptance handoff. Source ownership is released; reviewed recovery checkpoint `f71b9e09d4` passed hooks and is pushed on its subtask branch; parent integration remains. Parent terminal and duration files are frozen for lint/review.

## Delivery order and ownership

| Deliverable | Observable result | Owner and state | Dependency |
| --- | --- | --- | --- |
| Parent terminal identity remediation | Exact completion releases queued notifications without another user prompt, including repairable stored effects | Parent-owned [repair work order](task-02-parent-terminal-identity.md); committed as `a96adeec38`; checks and review accepted | Blocks final recovery integration acceptance; independent of frontend work |
| Recovery attention and continuation | Idle unresolved work says Needs attention, explains uncertainty, refuses unsafe sends without side effects, and permits supported explicit follow-up after exact settlement | Accepted and integrated at `67b6b9ba9d` | Recheck affected behavior after latest upstream merge |
| Successful completion cleanup | Save result before closing the exact runtime, retain completed conversation, resume only on explicit follow-up | Primary directly implementing and validating the existing cleanup work order | Production integration follows recovery to reconcile shared lifecycle/resume code |
| Historical result duration | Recorded elapsed time in completed disclosure summary and expanded row, stable after reload and later child activity | This primary; [duration work order](task-01-result-duration.md): all source/browser reviews and checks accepted; committed and pushed as `8ed56d94ef` | Snapshot/parser/card can proceed independently; coordinate locale/browser ownership with recovery |

[Recovery work order](../errand-orphan-settlement/task-01-recovery-settlement.md) and [cleanup work order](../errand-completion-cleanup/task-01-completion-cleanup.md) remain the detailed execution contracts. No separate copies of their requirements are introduced. The [duration work order](task-01-result-duration.md) implements AC-TASKS-ERRANDS-010.7 using the owning design's historical result elapsed-time contract.

## Bounded packets

1. Finish recovery backend, localized presentation and refusal/resume acceptance in the existing subtask. Review with Era Sol Que Me Faltava; integrate only accepted changes.
2. Prepare cleanup RED integration tests and audit current cleanup seams without changing production code. Then implement durable success-close ownership, successor/cancellation guards, retry and completed-state projection as one coherent packet. Follow with explicit continuation and desktop/phone acceptance on the integrated recovery base.
3. Implement optional measured duration in the immutable delivery item using recorded creation-to-result-settlement elapsed time, matching the existing panel meaning and including queueing/continuation intervals. Preserve zero, omit unknown/invalid measurements and cover normal and cancellation producers. Never derive historical timing from the mutable live row or the parent's await duration. Implement snapshot/codec compatibility, localized shared finished-card display, reload/continuation stability and desktop/phone containment under the duration work order.
4. Review each coherent patch with the selected reviewer, assign any remediation back to Lunaldinho, run only affected final checks, commit and push integration checkpoints with truthful scope. The user also requires all accepted work on this parent branch and a fresh merge of `upstream/main`: fetch the latest upstream at integration time, resolve conflicts, verify affected behavior, push and confirm the remote commit. Report outstanding requirements until all three are accepted.

## Delegation and resources

One implementation writer per shared file set. Reviewers are read-only and must identify concrete correctness/specification issues with paths and severity. No recursive delegation from Errands. Every brief names owned files, exclusions, evidence, tests and terminal handoff. Existing authorized workers are not discarded merely to change profile; new implementation/review packets use the user's selected profiles.

Heavy checks across both worktrees use `/tmp/kandev-errands-repair-build.lock`, disk-backed owned scratch, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `-trimpath` and `-p 1`. Capture every process handle and collect terminal results. Check disk after each packet; remove owned scratch, never blindly clear active runtime or shared cache files. No live runtime/database mutation or deployment. PostgreSQL skips remain explicit.

## UI acceptance

Recovery uses the existing desktop popover and phone drawer/full-screen child. Cleanup reuses completed-session presentation. Duration reuses the shared historical disclosure, with no new management surface:

```text
Parent history (desktop and phone)
  > 1 errand finished · 2m 14s
  expanded: title     Done · 2m 14s     Open

Recovery row / opened child
  Needs attention
  Earlier work has an unresolved recovery outcome.
  Open / existing Stop; unsafe send receives an explanation.

Successful retained child
  Completed
  Retained transcript; explicit follow-up uses existing composer.
```

Illustrative wording must use supported locale keys. Phone retains its current scroll owner, safe areas, navigation and touch targets. Exact duration wording and plural formatting belong to the duration packet. Existing recovery and cleanup work orders own the detailed browser cases.

## Acceptance ledger

- [x] Diagnose and repair the separately observed [parent notification dispatch refusal](queue-notification-incident.md); the recovery attention patch does not cover ordinary parent terminal-identity binding.
- [x] Recovery attention, capacity parity, consistent reasons and stable refusal (combined acceptance passed).
- [x] Safe supported explicit continuation after exact interruption settlement (combined acceptance passed).
- [ ] Success-close ordering, durable retry, successor isolation and retained completed state.
- [x] Historical recorded duration, legacy compatibility and immutable history (accepted, committed and pushed as `8ed56d94ef`).
- [ ] All applicable translations, desktop/phone acceptance, review disposition and integrated delivery.

## Verification results

Recovery frontend worker `31e441fc` released ownership at 10:05:07 on 2026-11-11 according to the coordinator's independent transcript/handoff inspection. It reports 124 focused tests, typecheck/lint/i18n and desktop/phone E2E passes; browser cases inject attention/refusal and do not prove exact-settlement continuation. A subsequent completion-delivery-uncertain error left the Errand falsely projected running. The coordinator owns review of the saved patch; this worker must not be resumed as a writer. Its delivery symptom is recorded separately in the notification incident note, without assuming the parent terminal-identity fix covers it.

Era Sol review `075cfb6e` found no frontend production issues but confirmed the earlier browser runs mocked frames and disabled global setup. Those runs count only as presentation evidence, not integrated browser acceptance. The recovery coordinator now owns a test-only packet using its existing test fixture binary, a dedicated recovery helper, existing Errands desktop/phone specs, artifact freshness registration and passthrough-hook tests. It must prove real backend classification/refusal and exact-receipt follow-up with standard artifacts/global setup and the shared build lock. Parent terminal and duration files remain excluded; production recovery code is frozen while these tests establish acceptance. Final integration must reconcile the new fixture registration and run against the integrated source.

Cleanup regression preparation and synchronization corrections are accepted by Era Sol `4fa0283f`; two intentional missing-COMPLETED failures remain, while successor/cancellation controls pass. Production cleanup is unimplemented and follows recovery integration. Parent removed two redundant embedded selectors in the fixture without changing test logic; current file hash is `b83d51acd269cbb8d4c6aaef364186b91b1604a78021ebdeb271344ab051aa48`.

Parent terminal repair and its notification regression are semantically accepted by Era Sol `e4c08c5d` and `3b49fda5`. Focused final race evidence is nine top-level tests plus four notification/control tests; SQLite matched zero tests in the focused selector, while earlier broad checks recorded PostgreSQL skips. Extended lint and normal hooks passed; terminal checkpoint is `a96adeec38`. Duration production is accepted; phone/persistence browser evidence is accepted; keyboard reachability passed its targeted check and final review `952a4e39`. No outstanding feature is reported as fully integrated or pushed.
