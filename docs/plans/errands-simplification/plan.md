---
created: 2026-11-15
status: in_progress
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
  - REQ-TASKS-ERRAND-COMPLETION-001
  - REQ-TASKS-ERRANDS-004
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
legacy_specs: []
---

# Errands: remove obsolete state and clarify lifecycle integration

This is the first implementation package from the [quality audit](../errands-remaining/code-quality-roadmap.md#first-bounded-audit-2026-11-15), based on delivered HEAD `6f0053b803` and upstream `585824a5d0`. The user authorized behavior-preserving execution through the roadmap. Earlier delivery packages remain complete; this package does not reopen them or change their performance limits.

## Selected design

Remove the unused private complete-list browser loader instead of maintaining two loader models. Keep the complete public/API inventory contract and operational live-row map. Bounded summary/history/inspection ownership continues to serve all browser consumers. Generic legacy plugin store access stays unchanged; no published SDK field is removed.

Make all lifecycle methods required of an installed `ErrandController` instead of probing a production service for capabilities it always provides. Keep nil for a genuinely unwired orchestrator. Retain Errands service policy, SQLite transactional decisions and orchestrator runtime ownership in their existing modules. Add missing contract coverage before this refactor; do not move lock boundaries, runtime calls, callbacks or settlement authority.

That contract coverage exposed an existing cleanup defect for legacy-provider user follow-ups. Task02 now includes a distinct repair phase before its controller refactor: use the already-committed exact successful decision when submission inventory provides no identity, retaining every ownership/conflict check and cancellation precedence. The requirement is unchanged; the completed user turn must close just as a completed parent turn does. The regression and repair are reviewed separately from the structural simplification.

Require one ownership value for internal await calls rather than supporting an ownerless branch and duplicate token/parent fields. The production MCP handler already validates that value. Keep public invocation-metadata compatibility and repository recovery; remove only internal service alternatives without real consumers.

Keeping the current dead loader and optional interfaces costs extra state and unsupported branches. A whole lifecycle rewrite would move proven safeguards without a demonstrated benefit. These local changes remove the identified costs while leaving the working product intact.

## Order and ownership

| Work order                                                               | Result                                                                                                  | Dependency                                                 |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| [01: private browser state](task-01-private-browser-state.md)            | Remove obsolete inventory state and preserve real consumers                                             | Accepted                                                   |
| [02: lifecycle contract and controller](task-02-lifecycle-controller.md) | Strengthen boundary tests, repair legacy completion identity, then make installed capabilities explicit | After 01; accept the repair before the structural refactor |
| [03: owned await requests](task-03-owned-await.md)                       | Remove duplicate internal identity and ownerless service paths                                          | After 02                                                   |

Task01 is accepted: 100 focused tests, desktop 3/3 and phone 3/3, no-findings review and unchanged frozen source hashes. Task02's test phase exposed a supported legacy-provider cleanup defect; the bounded repair is accepted after 54 orchestrator tests, four Errands controls, an unchanged GREEN public user-continuation oracle and no-findings review. The controller refactor is also accepted: 98 orchestrator tests, four Errands controls, normal optimized compilation and completed scoped lint passed, with no review findings. The five-minute lint wrapper timeout remains recorded separately from the identical-scope longer run that passed. Task03 is next under the same single-writer rule.

The task plan records actual child IDs, branch/head and active ownership. One writer owns this shared checkout at a time, including documentation changes. Read-only review may run concurrently. Writers must not stage, commit, push, start live agents or change runtime data. Parent owns review, integrated verification and delivery. Missing Specialist workflow is recorded in the task plan; native children are not persistent platform subtasks.

## Verification and limits

Task01 preserves history criteria .1–.8 through real store/hook/event consumers and existing desktop/phone demand-history, ordinary-loading and retention scenarios. No markup, controls, navigation design or viewport composition changes are intended, so no new ASCII preview is needed. The existing phone drawer, full-screen child and Back behavior remain authoritative. Task02 protects completion criteria .1–.4 through real SQLite, production parent delivery and controlled provider boundaries, retaining the separate cancellation, successor, receipt and projection-failure tests.

Record the baseline before changing tests. A behavior-preserving deletion does not need an artificial RED failure. If an assertion is replaced, prove the retained oracle catches its behavioral defect; restore any fault injection exactly before final checks. Preserve source hashes and existing failed benchmark observations. Current tests establish their exercised environments, not universal provider or OS support.

Heavy checks run sequentially under `/tmp/kandev-errands-repair-build.lock`; Go uses two workers and browser runs one shard. Retain process handles and evidence, monitor disk, and remove only inactive owned caches. This host's default pnpm is newer than the repository pin: prepend `/tmp/kandev-history-baseline-bin` to PATH or invoke `corepack pnpm@9.15.9`; do not relax the package-manager check. Work-order commands are scoped checks. Parent runs the affected integrated browser/performance controls against the final tree and matching upstream, using the existing protocol and unchanged thresholds, before claiming roadmap delivery.

## Results and follow-up

Task01 removed 184 net production lines across four files; seven tests were repaired or pruned against retained contracts. Its work order records current checks and their limits. Task02 is accepted: its 21-line repair restores legacy user-follow-up cleanup, and its controller refactor removes 38 net production lines. Task03 remains open. The [quality roadmap](../errands-remaining/code-quality-roadmap.md) retains final verification and review-history work; stale documentation and evidence retention have been reconciled. The separate bounded-use pilot will use a disposable current-build instance with isolated data, profiles and repository. The main instance reports `027432d2e0`; no main-server change is implied or required.
