---
created: 2026-11-15
status: in_progress
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
  - REQ-TASKS-ERRAND-COMPLETION-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
legacy_specs: []
---

# Errands: remove obsolete state and clarify lifecycle integration

This is the first implementation package from the [quality audit](../errands-remaining/code-quality-roadmap.md#first-bounded-audit-2026-11-15), based on delivered HEAD `6f0053b803` and upstream `585824a5d0`. The user authorized behavior-preserving execution through the roadmap. Earlier delivery packages remain complete; this package does not reopen them or change their performance limits.

Current delivery checkpoint: Tasks01–05 are integrated in original branch HEAD `d08b07c5b3`, with an identical-tree review branch at `47f459b5cd` on upstream `f52fbebd9b`. This first package does not complete the broader [quality roadmap](../errands-remaining/code-quality-roadmap.md#broader-assessment-resumed-2026-11-15): architectural assessment, evidenced redesign and cross-cutting maintenance consolidation are continuing. Performance acceptance remains open.

## Selected design

Remove the unused private complete-list browser loader instead of maintaining two loader models. Keep the complete public/API inventory contract and operational live-row map. Bounded summary/history/inspection ownership continues to serve all browser consumers. Generic legacy plugin store access stays unchanged; no published SDK field is removed.

Make all lifecycle methods required of an installed `ErrandController` instead of probing a production service for capabilities it always provides. Keep nil for a genuinely unwired orchestrator. Retain Errands service policy, SQLite transactional decisions and orchestrator runtime ownership in their existing modules. Add missing contract coverage before this refactor; do not move lock boundaries, runtime calls, callbacks or settlement authority.

That contract coverage exposed an existing cleanup defect for legacy-provider user follow-ups. Task02 now includes a distinct repair phase before its controller refactor: use the already-committed exact successful decision when submission inventory provides no identity, retaining every ownership/conflict check and cancellation precedence. The requirement is unchanged; the completed user turn must close just as a completed parent turn does. The regression and repair are reviewed separately from the structural simplification.

Require one ownership value for internal await calls rather than supporting an ownerless branch and duplicate token/parent fields. The production MCP handler already validates that value. Keep public invocation-metadata compatibility and repository recovery; remove only internal service alternatives without real consumers.

Keeping the current dead loader and optional interfaces costs extra state and unsupported branches. A whole lifecycle rewrite would move proven safeguards without a demonstrated benefit. These local changes remove the identified costs while leaving the working product intact.

## Order and ownership

| Work order                                                               | Result                                                                                                  | Dependency                                                                    |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| [01: private browser state](task-01-private-browser-state.md)            | Remove obsolete inventory state and preserve real consumers                                             | Accepted                                                                      |
| [02: lifecycle contract and controller](task-02-lifecycle-controller.md) | Strengthen boundary tests, repair legacy completion identity, then make installed capabilities explicit | After 01; accept the repair before the structural refactor                    |
| [03: owned await requests](task-03-owned-await.md)                       | Remove duplicate internal identity and ownerless service paths                                          | After 02                                                                      |
| [04: ordinary subscriptions](task-04-ordinary-header-subscriptions.md)   | Bound view-model listeners while preserving child recognition and ownership                             | After 03; source review before final browser/performance checks               |
| [05: definite non-delivery](task-05-definite-non-delivery.md)            | Preserve exact upstream unsent-prompt proof through Errand admission, claims and recovery               | Integrate upstream f52fbebd9b; independent of the open performance decision   |
| [06: prompt outcome owner](task-06-prompt-outcome-owner.md)              | Resolve admission evidence once and remove downstream error reclassification                            | After 05; preserve runtime/SQL evidence producers and existing cleanup guards |
| [07: browser read owner](task-07-browser-read-owner.md)                  | Own concrete reads and retained-data lifetime; remove unreachable aggregate loaders                     | After 06 and upstream receipt integration; preserve ordinary/child navigation |
| [08: current docs and evidence](task-08-current-docs-and-evidence.md)    | Consolidate current authority and archive exact historical evidence before pruning                      | After 07 and an immutable published checkpoint                                |

Task06 is accepted after 19 focused top-level race tests (66 entries, zero skips/failures), normally optimized compilation, scoped lint and two no-findings Sol reviews. It centralizes attempt evidence and removes parallel error classifiers and duplicate acceptance state; production grows by 96 lines to make the evidence and bookkeeping boundary explicit. The fixed five-minute lint wrapper timed out twice; identical package/base/rules passed with a ten-minute allowance. Task07's reviewed design deepens the existing browser reader and conversation-cache ownership. Task08 consolidates current documentation and preserves exact evidence before pruning. The persisted terminal-decision representation stays: captured-execution and legacy-source bindings are distinct supported evidence, and the audit did not establish a safe conversion or a benefit warranting a new persistence migration. Distinct concurrency and black-box tests remain. The [quality roadmap](../errands-remaining/code-quality-roadmap.md#broader-assessment-resumed-2026-11-15) records inspected and unassessed boundaries.

Task01 is accepted: 100 focused tests, desktop 3/3 and phone 3/3, no-findings review and unchanged frozen source hashes. Task02's test phase exposed a supported legacy-provider cleanup defect; the bounded repair is accepted after 54 orchestrator tests, four Errands controls, an unchanged GREEN public user-continuation oracle and no-findings review. The controller refactor is also accepted: 98 orchestrator tests, four Errands controls, normal optimized compilation and completed scoped lint passed, with no review findings. Task03 is accepted after 83 service, handler, SQLite/PostgreSQL repository and orchestrator tests, scoped lint, unchanged source hashes and no-findings review. Five-minute lint wrapper timeouts remain recorded separately from identical-scope longer runs that passed. All three implementation owners released the checkout and their checkpoints were delivered. Task04's final controls and measurements are recorded below; performance acceptance remains open. The original and separate review branches are published at the identical-tree checkpoint above.

The functional controls at `77b155597c` passed, but its matched ordinary comparison raised a desktop return-navigation warning. Task04's source is now accepted after 47 focused tests, typecheck, lint, formatting and independent review. Its real-store tests show twelve extra ordinary listeners/callbacks reduced to one, with both callers and ownership effects unchanged. The single production hook grows from 201 to 263 lines to make the selected inputs explicit. Profiling did not establish that those subscriptions cause the whole timing difference; fresh desktop 3/3, phone 3/3 and 28 comparison scenarios now pass. The unchanged numerical gates retain a phone switch-away warning, roughly one MiB fixed heap cost and measurable desktop switching cost. Performance acceptance remains open; the [final receipt](ordinary-header-verification.md) preserves all values and independent review. [Source handoff](ordinary-header-subscriptions-handoff.md); [original warning](final-verification-handoff.md).

The final upstream check added `f52fbebd9b`, which settles definitely-unsent prompts. Its clean merge exposed a concrete integration gap: Errand admission could preserve an uncertainty wrapper after the exact non-delivery receipt was committed. Task05 is accepted after independent source review, public caller and ownership regressions, 130 integration tests, actual SQLite/PostgreSQL conformance, normal optimized compilation and lint, and desktop 3/3 plus phone 3/3. The [handoff](definite-non-delivery-handoff.md) records counts, source hashes and limits. It reuses existing receipt, admission and claim owners without broadening uncertain-work recovery. The parent integrated and published Task05 at the checkpoint above; the frozen performance gate remains open.

The task plan records actual child IDs, branch/head and active ownership. One writer owns this shared checkout at a time, including documentation changes. Read-only review may run concurrently. Writers must not stage, commit, push, start live agents or change runtime data. Parent owns review, integrated verification and delivery. Missing Specialist workflow is recorded in the task plan; native children are not persistent platform subtasks.

## Verification and limits

Task01 preserves history criteria .1–.8 through real store/hook/event consumers and existing desktop/phone demand-history, ordinary-loading and retention scenarios. No markup, controls, navigation design or viewport composition changes are intended, so no new ASCII preview is needed. The existing phone drawer, full-screen child and Back behavior remain authoritative. Task02 protects completion criteria .1–.4 through real SQLite, production parent delivery and controlled provider boundaries, retaining the separate cancellation, successor, receipt and projection-failure tests.

Record the baseline before changing tests. A behavior-preserving deletion does not need an artificial RED failure. If an assertion is replaced, prove the retained oracle catches its behavioral defect; restore any fault injection exactly before final checks. Preserve source hashes and existing failed benchmark observations. Current tests establish their exercised environments, not universal provider or OS support.

Heavy checks run sequentially under `/tmp/kandev-errands-repair-build.lock`; Go uses two workers and browser runs one shard. Retain process handles and evidence, monitor disk, and remove only inactive owned caches. This host's default pnpm is newer than the repository pin: prepend `/tmp/kandev-history-baseline-bin` to PATH or invoke `corepack pnpm@9.15.9`; do not relax the package-manager check. Work-order commands are scoped checks. Parent runs the affected integrated browser/performance controls against the final tree and matching upstream, using the existing protocol and unchanged thresholds, before claiming roadmap delivery.

## Results and follow-up

Task01 removed 184 net production lines across four files; seven tests were repaired or pruned against retained contracts. Its work order records current checks and their limits. Task02 is accepted: its 21-line repair restores legacy user-follow-up cleanup, and its controller refactor removes 38 net production lines. Task03 removed 50 net production lines across two files while preserving public await compatibility and durable recovery. The [quality roadmap](../errands-remaining/code-quality-roadmap.md) retains broader architectural assessment, maintenance consolidation and performance acceptance. This first package corrected its scoped stale documentation and preserved its evidence; that does not establish a complete cross-cutting documentation or evidence audit. The [bounded-use pilot](../errands-remaining/bounded-pilot-receipt.md) ran on a disposable current-build instance with isolated data, profiles and repository. It records the initial model tool-use failure, useful review/implementation, cancellation and truthful unresolved crash recovery with an explicit operator preflight limitation. The instance is stopped and owned processes/listeners are gone. The main instance at `027432d2e0` was unchanged.
