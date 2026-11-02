---
created: 2026-10-30
status: draft
requirements:
  - REQ-TASKS-ERRAND-ORDER-001
  - REQ-TASKS-ERRAND-AWAIT-DURATION-001
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
  - REQ-TASKS-ERRANDS-CONTEXT-001
system_design:
  - ../../specs/tasks/system-design/errand-await-duration.md
  - ../../specs/tasks/system-design/errand-context-window.md
  - ../../specs/tasks/system-design/errand-panel-order.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
  - ../../specs/tasks/system-design/errands.md
legacy_specs: []
---

# Errands review repairs

## Implementation authorization

The user explicitly authorized implementation of all repair work orders and selected Option C for historical await entries. Implementation is active. Use Errands with bounded file ownership and TDD; no further planning approval is required. Completed checks must be recorded before marking any packet done.

## Outcome and current state

Repair every finding from the full review of `74a91cfc161916f0d5264b86762adae14e43119e..6400a265bb8a0a35fb702eced2aff1eeb81023d9`: nineteen correctness defects plus fallback identity and manifest recommendations. The checkout was clean at review completion. Implementation is active, with completed and partial repairs recorded in the work orders below. Original passing suites did not cover the reproduced failure windows; new receipts are recorded per packet. The original [Errands manifest](../errands/plan.md) retains historical receipts; this package owns new repair evidence.

The user requested all repairs using Errands and explicitly permits the configured Luna profile for implementation. The primary session owns integration, durable artifacts and shared schema contracts. The design checkpoint is complete and the user has explicitly requested implementation. Nothing here authorizes commits, push, a PR, shipped-default promotion or live-instance mutation.

## Evidence and Errand handoffs

The review reproduced cancellation overwrite and unclaimed await output with temporary Go overlays; transaction/race probes established partial claims and stale-token release. Real SQLite/orchestrator/executor diagnostics reproduced lost cascade recovery, failed-stop retry suppression, duplicate reserved-session creation attempts and role-read task failure. Five scratch frontend component/store cases reproduced stale reconnect/context state, preview navigation/menu and Threads announcement defects. The work orders name the permanent regressions and affected code; temporary probes are evidence, not accepted fix tests.

Terra designed transactional delivery and shared receipt boundaries; Sol designed lifecycle/workflow and UI/summary repairs. The follow-up Sol receipt audit uses the user's chosen uncertainty policy. Luna checked packet consistency and prepared the media execution handoff. Implementation may use the configured Luna profile (`188db592-1c5e-45e4-bb53-d84e6997f191`) for one bounded work order or explicitly listed subpass at a time, with its exact files, algorithm, RED/GREEN tests and predecessor receipts in the Errand prompt. The primary retains schema coordination and integration; Terra/Sol are appropriate for newly exposed cross-boundary design questions. Every writer receives the shared-checkout/no-revert/no-recursive-delegation rule.

## Chosen boundaries

The user selected preventing duplicates when await reception is uncertain. [The accepted ADR](../../decisions/2026-10-30-errand-await-delivery-uncertainty.md) qualifies AC004.4/004.8 and adds AC004.13. Automatically release only proven-unexposed outcomes with late effects fenced. Preserve unknown exposure and the transcript for explicit recovery; never infer receipt or abandonment from silence. Ordinary transactional failures remain automatically recoverable.

Keep the existing shared workspace, depth-one, five-live cap, fixed child tool catalog, role isolation and restart-required opt-in flag. SQL storage must remain SQLite/PostgreSQL compatible. Reuse existing runtime/session admission, cancellation and durable turn evidence rather than inventing parallel runtime ownership.

## Work orders and dependency order

The small packets separate independently testable storage, pure classification, runtime integration and UI outcomes. Execute dependencies before consumers; shared source files, migrations and browser specs have one writer. Commands in work orders define acceptance; only their Results sections establish completed evidence.

| Task | Outcome | Review finding | Depends on | Status |
| --- | --- | --- | --- | --- |
| 01 | [Token-fenced result release](task-01-token-fenced-release.md) | 10 | None | done |
| 02 | [Preserve cancellation against late settlement](task-02-cancellation-settlement.md) | 3 | 01 | done |
| 03 | [Atomic result claims and response snapshots](task-03-atomic-result-claims.md) | 1, 2 | 02 | done |
| 04 | [Fenced await outcomes and visible uncertainty](task-04-await-outcome-recovery.md) | 1 | 03 | done |
| 05 | [Fail closed on parent settlement errors](task-05-parent-settlement-guard.md) | 4 | None | done |
| 06 | [Preserve task state on unreadable session role](task-06-executor-role-guard.md) | 11 | None | done |
| 07 | [Capture results before successor admission](task-07-settle-before-successor.md) | 8 | 02, 03, 05 | done |
| 08 | [Keep created and recovered errands detached](task-08-created-errand-isolation.md) | 9 | 05, 07 | done |
| 09 | [Persist exact teardown obligations](task-09-teardown-intent-storage.md) | 6 | 04 | done |
| 10 | [Retry cancellation without stopping successors](task-10-retry-exact-teardown.md) | 6 | 09, 06 | done |
| 11 | [Reconstruct cancellation before admission](task-11-recover-lost-cascades.md) | 5 | 10 | done |
| 12 | [Persist and classify reserved launch authority](task-12-reserved-launch-authority.md) | 7 | 06, 09 | done |
| 13 | [Recover launch bookkeeping without replay](task-13-adopt-reserved-launches.md) | 7 | 08, 11, 12 | done |
| 14 | [Preserve Send Now sources and receipt identity](task-14-send-now-source-identity.md) | 18 | 04, 08 | in_progress |
| 15 | [Gate prepared results without losing user input](task-15-disabled-prepared-delivery.md) | 19 | 14 | pending |
| 16 | [Batch bounded errand summary observations](task-16-bounded-summary-aggregate.md) | 12 | None | done |
| 17 | [Repair missing and stale errand summaries](task-17-authoritative-summary-repair.md) | 12 | 16 | done |
| 18 | [Preserve role and parent in summary fallback](task-18-fallback-session-identity.md) | 20 | None | done |
| 19 | [Refresh each task across connection generations](task-19-reconnect-freshness.md) | 13 | None | done |
| 20 | [Converge authoritative context and cache](task-20-context-snapshot-freshness.md) | 14 | 19 | done |
| 21 | [Keep errand navigation inside its preview](task-21-preview-navigation.md) | 15 | None | done |
| 22 | [Hide ownership actions for retained errands](task-22-preview-menu-isolation.md) | 16 | 21 | done |
| 23 | [Announce rendered Threads status changes](task-23-visible-status-announcements.md) | 17 | 20 | done |
| 24 | [Verify mixed-parent ordering and transitions](task-24-mixed-parent-browser-acceptance.md) | task17 acceptance | 13, 15, 17, 18, 20, 22, 23 | pending |
| 25 | [Complete current dark desktop and phone evidence](task-25-current-dark-media.md) | task16 acceptance | 24 | pending |
| 26 | [Reconcile manifests and final repair evidence](task-26-documentation-reconciliation.md) | 21 | 25 | pending |
| 27 | [Compact historical await disclosure](task-27-history-disclosure.md) | User-reported historical spinner/visual weight | None | done |
| 28 | [Avoid notification guard re-entry](task-28-notification-guard.md) | Session-stall investigation | None | done |
| 29 | [Separate errand navigation and unify history](task-29-conversation-navigation-and-history.md) | Approved UI clarification | 27 | done |

## UI previews

UI-PREVIEW-01 and UI-PREVIEW-02: desktop Kanban preview owns its selection; phone uses the existing full-screen task route and bottom drawer. Open/Back never transfer workflow ownership.

```text
Desktop task preview                 Phone task
Parent [Errands] -> Child [Open]      Parent [Errands] -> bottom drawer
Child transcript [Back to parent]    Tap child -> full-screen chat [Back]
Child menu: Rename / Stop / Delete   Visible actions; no Primary/Handoff
```

UI-ORDER-01: scope the panel to the rendered parent, with active errands followed by initially collapsed completed history. Switching parents changes the panel's scope; Stop all remains task-wide. Phone has one internal list scroller, a fixed safe-area Stop all footer and 44px row targets. Context clear removes old counts/bar without opening the child. UI-ANNOUNCE-01 uses one polite region per visible detail view, independent of read-cursor acknowledgement.

```text
Current parent's errands            Delivery uncertainty
Active                              Child: Done
  Review API: Running, 68k / 100k    Result delivery uncertain [Open]
  Check tests: Running              Open retained transcript; deliberate
[Completed history]                 recovery through existing composer
[Stop all errands in task]          No automatic replay
```

Labels are illustrative and must use all supported locales. Uncertainty is separate from lifecycle, not a new failed/running status. Phone retains the existing drawer and full-screen transcript; no new nested control inside a whole-row Open target.

## Verification and acceptance

Each packet names exact red/green regressions and commands. Race tests cover ownership interleavings with barriers. SQLite integration exercises actual services/stores; PostgreSQL must execute with a disposable test DSN rather than skip. Run browser suites sequentially through the managed runner after rebuild, using actual preview/Threads/drawer behavior; mock callback assertions alone do not prove navigation. Task 24 closes the original Task 17 mixed-parent ordering/transition gap. Task 25 closes original Task 16 dark current-source media, with synthetic telemetry disclosed and isolated data.

Do not rerun broad suites without a relevant edit or failure. Preserve every process handle, inspect failures, and record final tests after the last source change. Existing review diagnostics prove bugs, not fixes. No live crash, provider teardown or PostgreSQL certification is inferred from mocked runtime tests.

## Risks and compatibility

Uncertain await delivery cannot be made exactly-once by a server write or timeout. Retry/cancellation must never act on a successor turn or new runtime. Legacy unknown identity/provenance remains preserved and blocked rather than fabricated. CAS conflicts require fresh authoritative observations; transactional errors must not become empty success. UI cache omission and explicit clear are distinct. The release flag must suppress previously prepared output while preserving folded user input.

## Design validation

Design checkpoint completed on 2026-10-30. `python3 scripts/list-docs.py validate` passed with 422 decisions and 1,861 specifications. `python3 scripts/lint-spec-files.py --all` passed. A local call to `.github/scripts/pr-docs.cjs` `validateCoverage` passed for the full reviewed feature range plus working/untracked documents: 489 changed paths, 52 work orders and 87 referenced documents. Repair-package checks resolved all 26 work-order IDs/dependencies, found no cycles and verified Markdown links. `git diff --check` passed; new documents also passed explicit trailing-whitespace/EOF checks. Logs are under `/tmp/errands-repair-{catalog,spec-lint,coverage}.*`.

The only repository edits at this checkpoint are documentation. No production or permanent regression-test changes, runtime launches, product tests, capture, commit, push or PR occurred. All 26 implementation/acceptance work orders remain pending; Task 26's manifest/index/status corrections are already applied, with its public recovery copy and final receipts still pending. The required implementation handoff begins with Task 01 and follows the table's dependency order, using the authorized Errands and bounded Luna briefs.

## Active implementation errands

- Task 01, Luna: `9aa87b26-ca9e-4811-ac7b-c50498f55d9c`.
- Task 05, Luna: `dd01b2c7-9a3a-4e2e-91ee-26a1dd1443c7`.
- Task 06, Luna: `34625d61-fd9f-4ba5-bdba-5f9c0469fd16`.
- Task 27, Sol: `5f24c2df-259f-4363-ad23-2b62060b6623`.

These four grants are disjoint production ownership; all later shared-file packets remain serialized. Continue with Task 02 after Task 01, and the independent summary/frontend packets as slots and file ownership permit. Task 04 presentation and Task 24 browser acceptance must preserve Task 27. No implementation is yet claimed accepted.

## Recovery of incomplete implementation handoffs

The first four implementation errands terminated with incomplete output. Tasks 01 and 05 have regression edits but no corresponding production repair in the inspected diff; Task 06 has executor changes and Task 27 has frontend changes, without complete acceptance receipts. None is accepted yet. Two warnings concerned Codex diagnostic persistence; the cause is not established. Disk capacity was not exhausted (39 GiB available at inspection).

- Backend recovery, Sol `5be2c3a9-7287-4bf4-83e6-9a2a9875ea20`: finish Tasks 01/05/06 sequentially, preserve partial work, record checks in `/tmp/errands-backend-recovery-progress.md` and own workorders.
- Option C recovery, Sol `3e419ddb-75f5-40d6-b6c5-9791bace92b2`: complete Task 27 and targeted browser acceptance; sole frontend/browser writer, evidence `/tmp/errands-option-c-progress.md`.
- Resource audit recovery, Luna `46ef6862-a84b-4fa5-a3da-73e9f5d09166`: read-only evidence report `/tmp/errands-resource-impact.md`.

The first resource audit reported 19 completed errands retaining ready executions and Codex processes, about 3 GiB combined RSS, with idle suspension disabled. Its full report was not saved; corroboration and retention-path analysis remain pending. No live cleanup or setting changes are authorized by this audit. Later implementation packets remain pending until prerequisites are accepted.

## Verified progress and resource-audit correction

Task 01 is done: full Errands race suite passed (84 top-level tests, 27 subcases, no skips), changed-Go lint passed. Exact receipts are in its workorder and `/tmp/errands-backend-recovery-progress.md`. Tasks 05/06 remain with backend recovery. Option C passed three targeted desktop and three native-phone browser cases without retries; final acceptance is still with its owner. Task 02 cancellation-safe settlement and Task 16 bounded aggregate query implementation have started with disjoint file ownership.

The saved resource audit (`/tmp/errands-resource-impact.md`, 2026-10-30 19:23 UTC) supersedes the unsupported inference that all completed errands retain provider processes. Of 27 rows, 24 completed sessions had stopped executor records with resume metadata, and three were running. All reference one workspace/worktree. The completed sessions contain 2,282 messages and 780,359 UTF-8 content bytes; this is not total physical disk size. The earlier 19-row/3-GiB sample is unverifiable. Shared parent/active-agent process sampling cannot establish per-errand memory cost or historical stop cause.

Confirmed retention concerns: the five-live cap does not bound completed history, inspected Errands paths lack TTL, per-row session lookups and startup reconciliation grow with retained rows. Idle suspension is disabled here and would preserve history even if enabled. No cleanup/settings changes were made. The audit cited original-plan workorders when discussing mitigation; those are distinct from this repair package. Repair Tasks 10/11 repair teardown/cascade correctness, while Task 04 explicitly retains attempt evidence until task deletion. Neither constitutes a total-history retention policy. Automatic history deletion is not part of the authorized correctness fixes.

## Delayed result handoff investigation

Received completed implementation receipts after a long period without automatic primary continuation. Tasks 01/02/05/06 and 27 are done per their saved workorder checks. Task 16 has SQLite/race/lint/sqlguard evidence but remains in_progress pending actual PostgreSQL execution; two skipped PostgreSQL tests are not passes. Current user request is read-only investigation of the stalled parent handoff, preserving the remaining implementation objective. Option C passed 52 unit tests and three desktop plus three phone cases. Saved report final timestamps precede the observed handoff by hours; runtime/queue evidence is being checked before attributing cause.

### Incident evidence: parent wake-up gap

Read-only SQL/log investigation found all four results settled between 19:25:16 and 20:00:59 UTC on 2026-10-30, but only delivered at 22:33:26 via turn_context on the user's new prompt. Parent `1f764ecc-824e-4065-a31d-96b2db9ee9b0` had Auto-run enabled. Notification `6e50cca0-0022-4294-914d-9aefe344ba58` was queued at 19:26:30, yet the next parent turn started at 22:33:22. Backend restarted at 22:32; the Errands override was enabled, so missing tools alone was not evidence of a disabled feature.

The preceding parent submission `prompt:queue:76d7d1c4-9927-40c6-bce0-f19247a8009d` has completed outcome but lacks fully_settled_at and terminal_turn_id. This is a terminal-settlement/queue-handoff lead, not proof of a particular lock deadlock. Later cancel deadline errors and notification ensure-session-running timeout are observed symptoms. Before claiming the repair package fixes this incident, reproduce the ended-parent/unsettled-submission/queued-notification window with Auto-run enabled and prove eventual exactly-once delivery without another user prompt. Preserve this as an explicit integration gate alongside Tasks 05/07/14; no live restart or queue mutation was performed during diagnosis.

## Resumed active native implementation

Errands start/await tools are absent from the current tool catalog. Authorized native workers are active instead: `atomic_delivery` owns Task03 plus coordinated admission integration; `summary_repair` owns Tasks17/18; `frontend_repairs` owns Tasks19-23 sequentially. Primary owns new [Task28](task-28-notification-guard.md) and isolated PostgreSQL verification. The primary remains active to integrate progress instead of relying on the broken Errand wake-up path. No prior implementation authorization is being re-requested.


## Resource-constrained resumption (2026-10-31)

The user reported resource exhaustion from concurrent Go and TypeScript builds. Resume the already-authorized implementation sequentially: at most one active implementation errand and one heavy validation command. Use Luna for bounded designed packets and Sol when complexity requires it. Every heavy command must acquire `/tmp/kandev-errands-repair-build.lock`; Go commands use `GOMAXPROCS=2` and `-p 1`. The primary must not run builds while an errand owns validation. Preserve prior partial edits; recover and verify them rather than assuming interrupted commands passed.

Current errand: `2521c824-cf2e-469a-93c9-693335a80ff0` (Lunaldinho), Task 28 notification guard repair. After its receipt, integrate results and continue the remaining work orders in dependency order, recovering Task 03 and the summary/frontend partial edits. Tasks 01, 02, 05, 06, and 27 retain their completed receipts. Task 16 still needs its PostgreSQL acceptance. No commits, pushes, or live-instance changes are authorized by this resumption.


### Sequential handoff after Task 28

Task 28 is done: exact-parent-session guard detection prevents notification readiness re-entry; both same-session regressions reproduced RED, all six focused race cases passed GREEN, and changed-file lint reported zero issues. No broad suite was run. Task 03 atomic claims is the next sole active implementation errand using the Sol medium profile; it owns the build lock and must recover interrupted changes, complete transaction/admission integration, and execute actual PostgreSQL checks. The primary runs no concurrent builds. Hiding errand conversations from regular session tabs remains a proposal from the discussion, not an approved implementation change.


### Task 03 revision checkpoint

Errand `33aee92f-c455-4198-a7c0-360d206bd7f1` finished a partial foundation: persisted result revisions and stable backfill, six focused tests including actual PostgreSQL, changed-Go lint, SQL guard, and whitespace checks passed. Task 03 remains in progress; admission fences, receipts/release recovery, ambiguous commits, independent-connection races, and conformance are still open. The next sole Sol errand implements the durable admission milestone; builds remain serialized. PostgreSQL from the completed errand was stopped.


### Task 03 admission packet narrowed

Errand `81f641a6-02e0-4256-8ad2-567f7764a41e` produced only an integration trace; it made no production changes and ran no new tests. It identified enqueue-time supersession, legacy dispatch bypass, and missing release fences. The next sole Sol errand is narrowed to repository admission/reservation and exact claim-release transactions with tests. Real orchestrator/MCP admission wiring remains a subsequent required milestone; neither this trace nor repository-only work completes Task 03.


### Task 03 repository milestone and next integration

Repository admission generations/reservations, idempotent decisions, immutable claim ownership, exact releases, deferred abort retry, and await tombstones are implemented. Errand `b2a041ba-13b3-48bc-9767-63a031c0754b` recorded 52 focused race tests (27 actual PostgreSQL, no skips), 14 SQLite/PostgreSQL conformance scenarios, caller compilation, SQL guard, lint and whitespace checks. Task 03 remains in progress because real dispatch integration is outstanding. The next sole Sol errand owns that integration with serialized builds.

### User clarification: consistent historical errand markers

The user clarified that Option C applies consistently to all errand chat markers, not just completed waits. Remaining UI scope includes compact, subtle expandable start/wait/completion events; collapsed summaries omit repeated titles, expanded content exposes titles and outcomes, and completed historical events use static icons and historical wording. Live status belongs in the Errands panel. Preserve completed Option C behavior and cover desktop/mobile parity. This clarification must be incorporated into the UI specification/work order before that sequential implementation packet; it is not yet implemented. The user subsequently approved removing errands from regular session tabs; see the UI priority clarification below.


### Approved UI priority: sessions versus errands

The user explicitly approved the proposed tab simplification and clarified ownership: normal sessions are conversations within a task; errands are delegated work inside a parent session. Immediately after the currently running backend integration errand, prioritize a sequential UI packet covering this distinction together with consistent compact historical markers. Update the relevant requirements/design and work order to reflect this approved behavior before implementation.

Normal conversations remain in the regular session tabs. Errands are accessed through their parent session's Errands panel, with active work visible and completed history collapsed. Opening an errand displays its conversation temporarily with an explicit Back to parent action and does not permanently add a regular tab. Preserve transcripts and direct access; hiding tabs is not deletion or runtime cleanup. Apply desktop/native-mobile parity, reload/direct-navigation handling, and existing role/ownership action restrictions. Start, wait, and completion chat events share the subtle expandable historical presentation already selected for waits. No additional implementation errand or build runs alongside the current backend packet.


### Authorized cache cleanup at the next idle boundary

The user requested cache cleanup because disk usage is high. Measurement: root filesystem 91% used, approximately 11 GiB available; `/home/luancm/.cache/go-build` alone occupies 35 GiB, Go modules 542 MiB, Playwright browsers 646 MiB, disposable PostgreSQL 375 MiB. The current integration errand has a focused race run active, so do not remove its caches while it builds. Before starting the next UI errand, acquire the shared build lock and reclaim expendable build caches, prioritizing Go build cache; inspect TypeScript/Vite/pnpm cache locations and remove only identified disposable caches. Preserve source, node_modules dependencies, live databases, transcripts, and test evidence. Report measured reclaimed space. Cleanup is authorized and pending, not completed.


### Dispatch integration receipt and cache cleanup

Errand `6a7fd5a8-aee1-44d7-bfcc-a9d686b17e4d` implemented durable admissions across initial/direct/FIFO/Send Now/fallback legacy and V1 dispatch, removed enqueue-time supersession, and wired exact releases/scoped AbortAwait. Receipts: 123 boundary race tests, 29 PostgreSQL cases, 14 conformance scenarios, zero lint issues, SQL guard/build/docs checks. Task 03 remains in progress for inventory serialization, ambiguous-commit readback, stale-snapshot retry and broader fault coverage; Task 04 native receipt/offer work remains pending.

After all worker commands and disposable PostgreSQL stopped, authorized cleanup ran under the shared build lock: `go clean -cache -testcache`, plus the identified Vite cache and TypeScript incremental build metadata. Root filesystem usage fell from 91% (11 GiB available) to 61% (45 GiB available) after Go cleanup, approximately 34 GiB reclaimed. Dependencies, source, databases, transcripts and test evidence were preserved. Subsequent builds start with a cold Go cache and must remain sequential. The next priority is the approved session-versus-errand UI and consistent historical markers.


### Task 29 clarification: normal-session menus and architectural simplicity

The user highlighted the desktop Agents/session reopen menu as another source of errand clutter and requested a simple architecture. This is the same approved navigation boundary, not a separate feature: all ordinary conversation tabs, reopen menus, normal-session pickers, counts and ordinals should use the existing session role to exclude errands. Preserve the complete session collection for transcripts, direct links, runtime state and parent-owned errand navigation. Reuse a shared existing navigation selector/helper where practical; do not change the global store/API to discard errands, add a new database model, migrate transcript/runtime storage, or duplicate the session lifecycle just to change navigation.

Inspection during active Task29 found `apps/web/components/task/session-reopen-menu.tsx` still sorting all `useTaskSessions(taskId).sessions` without a role filter. Before accepting Task29, include that actual desktop +/Agents menu in the implementation and browser checks: mixed normal/errand sessions must show only normal conversations with consistent ordinals, including when the active view is an errand. Errands remain reachable from the parent panel and direct links. This gap is recorded for integration; no concurrent UI edits or additional worker was launched.


### Task 29 UI handoff and final integration

Errand `12b81a34-5ed2-493e-8ab8-ed857bbde1f5` implemented normal-only tabs, temporary child conversation views with Back, parent-scoped panels, collapsed completed history, and START/WAIT/COMPLETION disclosures, preserving parent reading state. Receipts: 571 focused tests, desktop 11/11, native phone 8/8, scoped lint/format/i18n/spec/whitespace checks and inspected current screenshots. Typecheck still reports 17 missing connection.generation properties in nine inherited fixture files. The desktop Agents reopen menu also remains unfiltered in the handoff; its explicit user-reported gap is not covered by the existing normal-tab proof. Task 29 is therefore still in progress.

The sole next errand `8838dd25-ddb6-41cb-a729-522a98d23a6a` (Luna) owns the narrow menu/filter regression and fixture compatibility fixes, with serialized builds. The parent updated public coordination guidance for parent-owned navigation and expandable historical markers, preserving the existing task-wide Stop-all scope; public-doc validation remains pending until the worker releases validation ownership. Task21/22 reconciliation and Task25 dark-media acceptance remain open; do not infer their completion from the Task29 headline counts.


### Current UI integration status

The +/Agents menu now excludes errands and keeps normal-session numbering, including after the feature is disabled; the follow-up passed 161 focused tests, full web typecheck, scoped lint/format and 2/2 desktop browser checks. Public coordination docs are reconciled and validated (63/63 tests, 54 pages). Original Task21/22 preview navigation and ownership-action acceptance is the next bounded sequential packet, adapting only obsolete child-tab/phone-picker selectors to the approved temporary-view design. Task29 stays in progress pending that reconciliation. Task25 media remains separately pending; no runtime/database redesign was introduced for navigation.


### UI implementation accepted; reconnect repair resumes

Tasks21, 22 and 29 are done. The approved normal-session/errand distinction now covers tabs, the desktop +/Agents menu and phone selectors; Open/Back uses temporary child views, and START/WAIT/COMPLETION history is compact and expandable. Acceptance includes the final menu follow-up, full web typecheck, current desktop/phone browser receipts, preview/role assertions and public-doc validation. Task25 curated media remains a separate pending gate; no overall feature acceptance is inferred.

Next sole implementation packet: Task19 reconnect freshness, recovering the partial hook/store changes left by the earlier interruption. Builds/tests remain serialized under the shared lock; no additional worker runs alongside it.


### Task19 complete; Task20 starts in bounded suborders

Task19 reconnect freshness is done: 147 tests across ten files, scoped lint/format and full web typecheck passed. Regressions cover unmounted reconnects, task/auth-scope switching and hydration that retains cached rows while resetting runtime freshness maps. Task29 changes remain intact.

Task20 now proceeds sequentially through its existing UI-F2a/b/c suborders. The next sole errand owns UI-F2a authoritative context-property/cache ingestion: explicit null must clear cached context, accepted values replace it, omission preserves it, and the hook remains passive. Context epochs, stale transport ownership and the missed-event browser acceptance remain later gates; no early completion claim is permitted.


### Resource-retention concern and proposed actions (not yet implemented)

The user supplied a newer read-only installation snapshot for task `29d30fd5-4243-4b3a-99b5-2a76931fa7ef`: 38 finished errands, nine retained ready runtimes, approximately 1.2 GiB proportional resident memory plus 362 MiB swapped for completed-errand process trees, including about 215 MiB for the video errand. Workspace idle suspension was disabled. These are user-supplied observations from one installation, not independently remeasured in this turn or universal per-errand costs; they do not change the provenance limits of earlier snapshots. The five-live cap cannot bound resident completed processes.

Read-only inspection confirms existing executor idle parking already provides exact-execution suspension and durable resume provenance, but its workspace policy defaults off and ordinary focus resumes parked sessions. The proposed errand policy should reuse those mechanisms with role-specific eligibility and wake rules rather than introducing another runtime manager or database model. Proposed behavior for review: eagerly park fully settled completed errands once their result and required recovery evidence are durable, independent of the general workspace idle switch and without waiting for parent result consumption; preserve session/transcript/workspace and logical outcome; inspecting history does not wake a process, while a new accepted follow-up resumes through existing admission/recovery. Pending turns, dispatch, permissions/questions and known work remain protected, and stale cleanup cannot stop a successor execution. Failed stops remain visible and retryable through existing lifecycle maintenance/reconciliation. Unsupported safe restore remains a diagnosed exception, never silent conversation replacement.

The remaining lifecycle package should evaluate this as a priority resource contract after user feedback. Validate sequential completed-errand bursts with real process-tree/PSS accounting and plateau behavior, repeated Open/Back without wake, exact-once follow-up resume, successor races, failed-stop retry and restart, and preserved result delivery/parent workspace. Keep active runtime admission, idle completed runtime retention and history/artifact disk growth distinct. Normal runtime caps should account for resident ready processes; no per-errand automatic compiler-cache deletion, transcript TTL, separate database model or parallel cleanup service is proposed. This turn changed no live runtime/settings or implementation; the existing UI-F2a errand continues alone.


### Task20 ingestion complete; snapshot fencing active

UI-F2a completed with 39 focused tests, 119 adjacent tests, scoped lint/format and full web typecheck. Explicit context clears/replacements now atomically update session metadata, cache and task mirrors; the hook is passive and cannot resurrect cleared metadata. The initial default-heap typecheck exhausted Node's 2 GiB heap; the sequential 8 GiB-heap rerun passed.

The next sole Luna errand implements UI-F2b context epochs and hydration fencing, preserving newer live updates against delayed snapshots. UI-F2c connection-generation response ownership and final missed-event desktop/phone acceptance remain open, so Task20 stays in progress. The completed-errand resource-retention policy remains a recorded proposal pending user feedback; no implementation or live cleanup was inferred from that discussion.


### Task20 snapshot fencing complete; final reconnect/browser packet

UI-F2b completed with 104 tests across nine files, scoped lint/format, full web typecheck with an 8 GiB heap, and whitespace checks. Context-specific epochs now preserve newer live/model-reset values across deferred snapshots, route/list hydration and remounts.

The sole next Luna errand implements UI-F2c full-session read ownership across connection generations and then executes the specified missed-event desktop/native-phone browser acceptance. Task20 remains in progress until those gates pass. Curated dark media and resource-retention policy remain distinct from this acceptance; no parallel builds or errands are started.


### Task20 accepted; Threads announcement repair next

Task20 is done. Its final connection-generation packet passed 42 focused tests, typecheck, scoped lint/format, and both desktop Chromium and native Pixel5 missed-event cases on current assets. Browser evidence proves reconnect clears the context row without reload, keeps sibling context, avoids child detail activation, remains clear after reload and has no horizontal overflow. UI-F2a/b receipts remain recorded in the work order.

The sole next Luna errand owns Task23: visible Threads status announcements with unchanged unread behavior and hidden-view silence. All checks remain serialized. Task25 curated media and remaining backend repairs stay open.


### Frontend repair packets complete; backend atomic-delivery acceptance resumes

Task23 is done: real desktop Threads 2/2, native-phone Threads 2/2, adjacent unit tests 67/67, scoped lint/format, frontend build and typecheck passed. Visible transcript activation now drives announcements while Threads preserves its unread policy and covered/offscreen views stay silent. Tasks19–23 and the approved Task27/29 UI are complete; Task24 aggregate acceptance and Task25 curated media remain separate open gates.

Resume Task03's remaining backend transaction work with one Sol errand: complete parent-inventory serialization, retained-token ambiguous-commit readback, bounded stale-snapshot retry and missing real service fault/concurrency coverage. The prior durable admission integration receipts remain valid for their tested scope and must be preserved. Task04 native offer/receipt work and downstream lifecycle repairs remain pending. Disk currently has approximately 41 GiB free (65% used); no cache cleanup is needed at this checkpoint.


### Task03 complete; Task04 begins with offer/abort outcomes

Task03 is done. Final acceptance includes 664 top-level race-test passes across service/repository, dispatch/MCP/executor and queue boundaries, actual PostgreSQL, 14 task conformance scenarios, both-engine upgrade checks, SQL guard, build, zero lint issues, docs validation and clean whitespace. Immutable snapshots, full-inventory serialization, retained-token ambiguous-commit handling and bounded retries are implemented; PostgreSQL stopped and the writer released.

Task04 proceeds through its approved 04A/04B/04C passes sequentially. The next sole Sol errand owns 04A: trusted attempt ownership, durable offered/error disposition and journal-backed abort recovery. Native receipt extraction/inbox reduction and revision-bound presentation remain later passes. The user's duplicate-prevention policy remains authoritative: uncertainty never authorizes automatic replay.


### Task04A accepted; native receipt integration active

The 04A errand's displayed result stopped before its final handoff, but the saved work order and progress report confirm final acceptance and writer release. Final-source checks passed 104 boundary race tests (38 actual PostgreSQL), all 87 journal tests, 14 conformance scenarios, SQL guard, build, zero-issue lint, docs/spec validation and whitespace checks. All handles were consumed and disposable PostgreSQL stopped. Durable offer/error disposition, trusted ownership and journal-backed abort replay are implemented.

The next sole Sol errand owns 04B supported native receipt extraction, bounded journal/event preservation and atomic authenticated inbox/receipt replay reduction. Offered success remains distinct from positive reception. Task04 stays in progress for 04B and the later 04C presentation pass; no automatic replay or resource-retention change is included.


### Task04B accepted; revision-bound projection next

04B is done: both supported Codex native receipt paths preserve evidence through display truncation and journal replay, then reduce atomically with authenticated inbox/cursor acceptance. Final verification passed 85 boundary race tests (43 actual PostgreSQL), 716 native adapter tests, 87 journal tests, 14 conformance scenarios, lint, SQL guard, build and docs checks. One unrelated opt-in installed OpenCode compatibility test was skipped; neither supported receipt path was skipped. PostgreSQL stopped and writer released.

04C is split sequentially to keep ownership and cost bounded: the next sole Sol errand implements backend uncertainty classification and revision-bound DTO/list/event projection; a subsequent cheaper UI packet will consume that exact contract and preserve the compact historical presentation. No warning is inferred from a live await, omitted data, zero-item waits or an unrelated turn. Task04 remains in progress; uncertainty never permits automatic replay.


### Parent-turn completion identity mismatch: open regression gate

User reported restart at04:04:44UTC and parent remaining working until Pause. Read-only evidence in `/tmp/errands-restart-stall-2026-11-01.md` confirms running binary is clean baseline6400a265 (repairs not deployed). Native end_turn at04:06:15 retained an older synthetic turn identity; the backend rejected settlement against a newer active turn. Child9e38064f finished05:07:42, but notification was delivered07:20:31 after Pause closed the parent at07:20:14. This is distinct evidence from Task28's guard re-entry defect and is not yet verified fixed in current source.

After the active04C backend errand releases ownership, prioritize a bounded wakeup/terminal-correlation regression against the patched checkout, starting without restart and adding startup-replay coverage only where the evidence requires it. Preserve exact successor protection; do not repair this by completing arbitrary latest turns. The active errand remains undisturbed, and no extra build/worker or live mutation was introduced. This open gate must be resolved before declaring full repair completion.


The user clarified that the parent already resumed at04:05 after the restart and then started04B. Recovery resumed work successfully; the observed stall is completion of that resumed parent turn, not failure to resume after restart. The stale captured turn identity remains confirmed, but restart as a necessary trigger is unproven. The investigation report and next regression scope reflect this distinction.


### Limited parallel preparation authorized

The user permits additional parallel work when it does not require builds. Keep exactly one implementation/build/test owner, currently04C backend. A second lightweight Luna errand may perform read-only source tracing for the parent completion identity mismatch and write only a scratch regression handoff. It must not compile, run tests/linters/browser tools, launch services, change source/workorders, or touch the live instance. This updates the earlier blanket one-errand limit while preserving the one-heavy-command constraint; no additional implementation writer is authorized by this scheduling change.


### Read-only mismatch trace completed

The source trace is saved in `/tmp/errands-turn-mismatch-trace.md`. The old incident ID is the durable projector's fallback Kandev turn identity, distinct from agentctl ControlTurnID. Exact producer attribution remains unproven; wakeup-only and direct-message association paths require a real no-restart regression without injecting a desired turn ID. Existing successor protection is intentional and must remain. The next test should observe the actual prompt-generation/submission/turn binding and terminal projection, with an independent successor positive-protection control. No tests/builds or source edits occurred in this trace.

While04C remains the sole implementation/build owner, a lightweight Luna errand prepares the remaining work-order/evidence reconciliation for Task26 in a scratch report only. Task26 itself remains pending for its acceptance dependencies; no second build or source writer is introduced.


### Remaining-work reconciliation prepared

Read-only handoff `/tmp/errands-remaining-work-handoff.md` records remaining dependency order and receipts. Task16 still needs actual PostgreSQL coverage including the >499-ID boundary; Tasks17/18 lack accepted completion receipts. Historical active-owner/design checkpoints remain history, not current status. The active implementation remains04C; both read-only errands have finished.

The handoff repeated stale Task24 wording about a task-wide grouped panel. Primary corrected Task24 to the approved Task29 parent-owned UI: per-parent active/completed ordering, initially collapsed history, isolation and switching between parents, with task-wide Stop-all preserved. Task29 is now an explicit dependency/design reference. This is a work-order correction, not a product implementation or acceptance claim. Task24/25/26 remain pending; no tests/builds were run for this preparation.


### Current verification resource policy (2026-11-01)

Two independent work orders may progress with disjoint ownership, while all heavy validation remains serialized under `/tmp/kandev-errands-repair-build.lock`. Go commands use shared `/home/luancm/.cache/go-build` and `/home/luancm/go/pkg/mod` caches, `GOMAXPROCS=2`, `GOFLAGS=-p=1`, explicit test `-p 1 -parallel 1`, and lint concurrency 1. Use a 3GiB Go soft memory target for light checks/lint, retaining the proven 8GiB setting for large compiler/export steps under the exclusive lock. These are concurrency and soft-memory controls, not per-build hard caps. The existing service cgroup remains unchanged. Full command guidance is in `/tmp/errands-verification-resource-policy.md`; include it in subsequent errand handoffs. No per-errand cache copies or concurrent cache cleanup. This supersedes the earlier one-active-implementation restriction while preserving one heavy validation owner.


### Second independent work order admitted (2026-11-01)

Task18 fallback session identity is in progress alongside Task04C backend projection. Task18 owns only `internal/task/service/service_sessions.go`, its session-summary projection test and its work order; these are disjoint from the recorded Task04C inventory. Both workers share Go caches and the exclusive verification lock. Current work-order totals are 13 done, 3 in progress and 13 pending; Task16 is awaiting PostgreSQL validation, not an active worker.


### Task04C backend accepted; frontend handoff (2026-11-01)

Backend04C is verified with 35 focused race passes (12 actual PostgreSQL), 14 conformance scenarios, DTO contracts, SQLguard, build and lint. Exact contract and receipts: `/tmp/errands-task04c-backend-handoff.md`. Task04 remains in progress for frontend/localization; that packet owns only web files and may run beside Task18 under the shared verification lock. Task18's own regression passed; its broader selector exposed unrelated pending Task17 CAS-summary failures, which remain open rather than counted as Task18 acceptance failures or silently fixed. A single additional DTO regression file is permitted for Task18 if existing tests do not cover the converter contract. The mobile session-tree refinement discussed with the user remains a proposal, separate from this uncertainty packet.


### Task18 accepted; Task16 validation resumed (2026-11-01)

Task18 is done with four focused race tests covering real list/batch fallback through the DTO converter, formatting and patch-scoped lint. Its broader selector still fails pending Task17 CAS-summary regressions, and the package-level lint reports Task17 complexity at `service_status_summary_reconcile.go:56`; neither is counted as fixed. Current totals: 14 done, 2 in progress, 13 pending. Active packets are Task04C frontend/localization and Task16 actual PostgreSQL acceptance. They have disjoint ownership and serialize all heavy checks under the shared lock. Task16 may start and stop only the established disposable PostgreSQL fixture for its missing >499-ID boundary and dialect checks.


### Task16 accepted; Task17 resumed (2026-11-01)

Task16 is done: six focused race tests passed, including both actual PostgreSQL tests without skips and the 500-ID boundary spanning two 499-ID chunks. No Go changes were needed; the disposable server is stopped and an unattributed pre-existing scratch schema was preserved. Task17 now owns recovery of the partial authoritative-summary repair, including CAS re-observation failures and the summary-reconcile complexity finding exposed by Task18. Active workers: Task17 backend and Task04C frontend, with one shared heavy-validation lock. Totals: 15 done, 2 in progress, 12 pending.


### Task17 accepted; Task07 started (2026-11-01)

Task17 is done: fresh CAS observations and missing-row winner reconciliation passed 37 tests and 13 subtests under race detection, without skips; scoped lint and backend composition build passed. This closes the Task17 test/complexity findings exposed by Task18. Task07 now owns exact-turn settlement before successor reservation, including failure/retry and actual queue admission wiring under the existing task guard. Sol handles this higher-risk transactional integration; Luna continues frontend04C. Two disjoint workers, one heavy validation lock. Totals: 16 done, 2 in progress, 11 pending. The separately observed parent terminal identity mismatch still needs its own producer-path regression; Task07 completion alone must not be reported as fixing it.


### Task04C UI implemented; rendered acceptance follow-up (2026-11-01)

The uncertainty UI and localized copy passed 86 focused tests, ESLint, Prettier, i18n checks/ratchet, full web typecheck and E2E Vite build. Managed desktop/phone cases stopped before execution because the mock-agent fixture predates Task17 source. Task04 remains in progress. A bounded UI follow-up may rebuild required backend/helper fixture artifacts under the shared lock, verify source freshness and run only the uncertainty desktop/phone cases; no freshness bypass or live-instance mutation. Task07 remains the sole backend source writer. Final Task24 combined acceptance remains separate.


### Emergency cache cleanup completed (2026-11-01)

At the user's request, cleared the 45GiB shared Go build/test cache after verification released the shared lock and no compiler/test/lint process remained. Filesystem usage fell from 99% (1.3GiB free) to 60% (46GiB free). Source, dependencies, databases, transcripts, logs/evidence and current built artifacts were preserved. Workers may resume serialized checks with a cold cache; accepted checks do not need repeating. Receipt: `/tmp/errands-storage-cleanup-status.txt`.


### Task07 accepted; backend frozen for browser acceptance (2026-11-01)

Task07 is done with exact durable terminal settlement and successor-admission fencing, including failure/recovery and stale event guards. Its work order records final race/actual PostgreSQL/conformance/build/lint receipts. The parent-terminal producer mismatch remains separate and unproven. Backend source is frozen while Task04C browser verification rebuilds a coherent current backend and required helpers; no new backend writer is admitted yet. The older Task17 backend must not be mixed with current helpers as current-source acceptance. Snapshot instructions: `/tmp/errands-backend-browser-checkpoint.md`. Totals: 17 done, 1 in progress, 11 pending.


### Task04 accepted; Task08 started (2026-11-01)

Task04 A/B/C are done: rebuilt coherent current backend/helpers, desktop uncertainty 1/1 and native-phone 1/1 on diagnostic retry, fresh captures inspected, final scoped lint/format/diff clean. The intermediate phone Task unavailable route failure remains unexplained and is recorded for Task24, not silently counted as fixed. Backend browser freeze is released. Task08 now owns created/recovered errand workflow isolation; a separate lightweight read-only phone-route trace may overlap, with no builds or source edits. Totals: 18 done, 1 in progress, 10 pending.


### Phone route trace incorporated (2026-11-01)

The lightweight phone-route trace is complete. Implicit primary/first-session selection versus the test's explicit parent expectation is a candidate, not a confirmed cause. Task24 now requires complete pre-navigation identity evidence and uniquely retained failure screenshots/route hydration state before retry. No code, builds or browser runs occurred. Task08 remains the active implementation; completion totals stay 18 done, 1 in progress, 10 pending.


### Task08 accepted; Task09 started (2026-11-01)

Task08 is done: created/recovered errand launches retain profile and role/MCP mode, avoid workflow ownership effects, preserve accepted prompt context and refuse terminal redirection while retaining archive eligibility checks. Targeted orchestrator and executor race commands passed; selectors matched 49 and 25 top-level tests respectively. Scoped lint and formatting passed. Task09 is the sole schema/repository writer for durable exact teardown intent; Task10 will consume it for physical-stop retries. Totals: 19 done, 1 in progress, 9 pending. No new concurrent heavy work is admitted.


### Task09 accepted; Task10A started (2026-11-01)

Task09 is done: private immutable teardown intents, replayable one-column storage and exact-ID/version transitions passed six model tests, eight SQLite and eight actual PostgreSQL repository tests, 14 task-conformance scenarios, both upgrade dialects, SQLguard and lint. No physical-stop implementation is claimed yet. Task10 is split as designed: Sol first owns 10A exact-target adapter and guard/acknowledgement behavior; 10B later integrates lifecycle cancellation and retries. No other backend writer or heavy validation runs concurrently. Totals: 20 done, 1 in progress, 8 pending.


### Task10A accepted; Task10B started (2026-11-01)

The exact-target adapter is verified: 25 required race tests, seven compatibility tests, build, scoped lint and formatting passed. Existing synchronous StopExecution was sufficient; no executor changes were needed. Task10B now owns migration of cancellation callers, durable pending-intent recovery, generic-stop routing and reserved-launch cancellation fences. Physical provider behavior remains mocked-boundary evidence; automatic parking of successful errands remains a separate proposal. Totals unchanged: 20 done, 1 in progress, 8 pending. One backend writer and serialized heavy verification.


### Task10 accepted; Task11 started (2026-11-01)

Task10 is done with exact-target cancellation, durable retries and initial launch/deletion fences. Final verification passed 124 top-level race tests (44 errands, 59 orchestrator, 21 executor), gateway caller regression, build, lint and formatting. Real provider behavior remains mocked. Task11 now reconstructs lost cascades from durable task/parent state before queued admission and through existing startup/sweep recovery. Single backend writer, serialized heavy checks. Totals: 21 done, 1 in progress, 7 pending.


### Cache cleanup before Task11 validation (2026-11-01)

User-requested cleanup reclaimed about 42GiB from the shared Go build/test cache under the exclusive repair lock after confirming no active Go tools. Disk usage fell from 96% (4.7GiB free) to 59% (47GiB free). All source, dependencies, binaries, databases and test evidence were preserved. Task11 may proceed with a cold cache; accepted earlier checks remain valid.


### Task11 accepted; Task12 started (2026-11-01)

Task11 is done: 51 errands and 71 orchestrator top-level race tests passed without skips or race reports, plus caller build, lint, formatting and whitespace. Final evidence is `/tmp/errands-task11-verification.json`; all handles consumed. Task12 now owns immutable reserved-launch provenance and a read-only recovery classifier. Runtime adoption remains Task13, with Task10 cancellation and Task11 ownership fences preserved. Totals: 22 done, 1 in progress, 6 pending. Single backend writer and serialized heavy validation.


### Task12 accepted; Task13 started (2026-11-01)

Task12 is done: immutable launch authority is written with initial session insertion, and the pure classifier rejects incomplete/conflicting evidence. Eighteen top-level race tests and 124 subcases, caller build, scoped lint and formatting passed. Task13 now consumes the classifier under existing admission guards for adoption/continuation without replay, retaining Task10/11 cancellation and ownership fences. Totals: 23 done, 1 in progress, 5 pending. Single backend writer; heavy checks remain serialized.


### Task13 accepted; Task14 started (2026-11-01)

Task13 is done: 98 race tests and 135 compatibility tests passed, plus caller build, lint and formatting. Twenty-nine optional PostgreSQL compatibility cases skipped without a DSN; those are not passes. No SQL/schema/dialect production behavior changed in this packet, so earlier storage acceptance remains applicable. Proven launches adopt without replay and prepared launches retain exact identities. Task14 now owns Send Now source-aware composition and durable receipt/transcript identity. Totals: 24 done, 1 in progress, 4 pending. One backend writer and serialized heavy validation.


### Task14 recovery-reader ownership resolved

Task14 discovery found a necessary scalar-token recovery reader outside its initial inventory. Ownership now includes backendapp/errands_transcript.go and its focused tests, so all source tokens represented in a persisted transcript remain recognized during reconciliation. This completes the existing multi-source receipt contract without a new public API. No user decision or architectural change is required; Task14 resumes with the expanded narrow boundary.

### Task14 interrupted; bounded continuation

Errand `af0ad700-c38e-4c48-a062-1ab9e6e75d30` exited with code `-1` and no acceptance result. Saved work includes four Send Now regression functions and one extended recovery test; its RED log is empty, and no old validation process or lock holder remains. The npm notices do not establish the process-exit cause. Task14 stays `in_progress`; totals remain 24 done, 1 in progress and 4 pending.

The next sole implementation errand handles Task14A: the internal structured receipt/source codec and the scalar-compatible persisted-transcript recovery reader. Task14B then integrates the queue dispatch, durable claims, source admission and transcript updates and runs the complete acceptance suite. The work order records the additional narrow producer ownership and per-pass checks. Use the same configured Sol profile, one writer, and the shared heavy-validation lock. Preserve all existing changes; no commit, push, deployment or live-instance mutation is authorized.

Task14A is running as errand `0c63b138-8b44-4b4f-b591-e5bb359fc733`, profile `277af684-cf04-4853-b057-a266dd75b44f` (Era Sol Que Me Faltava Mesmo). Its result must include `/tmp/errands-task14a-handoff.md` and exact RED/GREEN evidence. Task14B has no active writer until this receipt is integrated.

### Scope and commit checkpoint requested

The user requested coherent commits and raised concern about scattered or unnecessary changes. Finish the already-running bounded Task14A pass, then assess working-tree scope against accepted repairs and commit completed, dependency-coherent groups before starting Task14B. Read-only backend and web scope reviews and a commit-boundary inventory are active. Commit authorization now covers these local checkpoints; no push, PR, deployment or live-instance mutation is authorized. Do not infer overengineering from file count alone or undertake a broad cleanup without concrete findings. Unfinished tests and active partial implementation must retain explicit status and remain outside accepted repair commits.

The review snapshot over `6400a265bb8a0a35fb702eced2aff1eeb81023d9` contains 368 paths: 158 production, 144 test, 50 documentation and 16 locale files, with 30,497 additions and 2,150 deletions including untracked files. Most breadth maps to the approved uncertainty, exact-settlement, recovery and navigation contracts; the review found no broad unrelated web production churn. This was a focused scope/architecture review, not exhaustive correctness certification.

Task19 is reopened for a confirmed equal-timestamp list/live delivery-state race at `apps/web/lib/state/slices/errands/errands-slice.ts`; its work order records the failure schedule and focused regression. Current totals are 23 done, 2 in progress and 4 pending. Task14 and Task19 remain unfinished even after local checkpoints.

Concrete consolidation candidates remain to be resolved before claiming the architecture review clean:

- `internal/errands/{reconcile,results}.go` retains a latest-turn settlement fallback alongside the exact terminal-decision path, and the in-memory store still exercises the fallback. Align the required storage capability and behavioral tests before removing the obsolete algorithm.
- `internal/errands/service.go` retains the old `SessionStopper` dependency even though cancellation uses `ErrandTeardown`. Remove that obsolete seam with its adapter and compatibility-only tests once callers are reconciled.
- `internal/orchestrator/{errand_launch_recovery,errand_teardown}.go` duplicates runtime-absence classification and differs on nil persisted inventory. Reconcile the intended authority and share the decision where semantics are identical; do not blindly merge different safety contracts.
- Mobile/Threads normal-session selectors retain unreachable nested-errand branches and unused row/nesting helpers after Task29. Remove those branches/modules and their obsolete tests while preserving the approved separate Errands navigation.
- Historical WAIT repeats the shared disclosure UI in `errand-await.tsx`. Reuse `ErrandHistoryDisclosure` while retaining WAIT-specific duration, reason and recorded rows.

The list-only context hydration watermark is an additional unconfirmed reachability concern, not an established product defect. The local commit checkpoints preserve reviewed progress and explicit open findings; they are not release acceptance. No cleanup implementation or new worker was started from these findings.

The checkpoint sequence is: repair contracts and evidence (50 documentation paths), backend recovery and summary projections (206 backend paths), and conversation/cache UI (110 frontend/locale paths). The 316 candidate application paths match the preserved Task13 completion snapshot; this proves the commits retain the previously tested bytes, not that the new review findings are resolved. Per-work-order splitting would reconstruct untested intermediate versions because the repairs share schema, interfaces and callers. Keep the five unfinished Task14 source/test paths outside these commits. Exact path inventories and prior check receipts are retained in `/tmp/errands-commit-boundaries.json`; commit hashes and hook outcomes belong in the live coordination plan after each checkpoint succeeds.

The first backend-only hook attempt exposed an unsafe staged snapshot: pre-commit stashed the existing summary caller changes while retaining the untracked summary helpers, causing a missing `statusSummaryErrands` field. Recovery and summaries therefore travel in one backend checkpoint, with this manifest correction included. No production change or hook bypass was used to repair the split; the failed attempt is retained at `/tmp/errands-commit-02-core.log`.

### Authorized consolidation continuation

The user explicitly requested proceeding after the checkpoint and scope findings. First finish the reopened Task19 list/live race within its existing work order, verify and commit it independently. Then address the recorded obsolete UI/disclosure code and backend duplicate ownership paths in bounded, sequential changes, preserving their existing behavior and safety contracts. Do not combine these into another accumulated repair diff or start Task14B before the consolidation findings are resolved. The five unfinished Task14 files remain preserved and excluded from unrelated commits. One implementation errand and one heavy validation command may run at a time; retain the shared build lock and resource limits. No push, deployment, live-instance mutation or broader redesign is authorized.

Active sole writer: Task19 errand `49a0f0c3-8d75-47f7-98a3-f78e59e99089` on the configured Sol profile. Progress: `/tmp/errands-task19-race-progress.md`; required final handoff: `/tmp/errands-task19-race-handoff.md`. It owns the existing Errands slice/request-boundary files and their tests only. The primary integrates its result and makes the focused commit before granting the next consolidation packet.

### Task19 race correction accepted

Task19 is done again: the deferred equal-timestamp delivery-state regressions reproduced three behavioral failures, then the final six-file suite passed all 63 tests after the row-specific fence. Scoped lint, formatting and whitespace checks passed. Existing task/request ownership remains intact; unrelated rows still accept their list updates. The five Task14 files are unchanged. Totals return to 24 done, 1 in progress and 4 pending. Handoff and exact receipts: `/tmp/errands-task19-race-handoff.md`. Commit this correction before granting the next consolidation packet.

### Historical WAIT disclosure consolidation accepted

Historical WAIT reuses the existing `ErrandHistoryDisclosure`, retaining its duration/reason summary, recorded rows, test IDs, spacing and interactions. Three frontend files changed; production code decreased by ten lines. The same 59 focused tests passed before and after reuse; scoped ESLint, Prettier and whitespace checks passed. Existing keyboard/phone browser coverage was preserved but not rerun because interaction and responsive rules are unchanged. Receipt: `/tmp/errands-wait-disclosure-handoff.md`. This closes the shared-disclosure finding; obsolete selector branches and backend duplication remain open. All five Task14 files are unchanged.

### Obsolete selector nesting consolidation accepted

Removed unreachable nested-errand branches from normal mobile/Threads selectors and deleted the uncalled errand row/nesting modules with their dedicated tests. Production code decreased by 100 lines. The strengthened pre-refactor suite passed 92 tests; final focused coverage passed 88 tests, with the four-test difference solely the removed orphan helper suite. Scoped lint, formatting and whitespace checks passed. Receipt: `/tmp/errands-selector-cleanup-handoff.md`.

The adjacent unchanged `mobile-session-picker-row.test.tsx` flag-off case failed before and after the packet: it expects a persisted-role errand in the normal picker, while the current role-based wrapper hides it. This remains an explicit test/contract reconciliation item, not a passing check or a new selector regression. Browser specifications were preserved but not rerun because behavior and geometry are unchanged. All five Task14 files remain unchanged. Backend stop-interface and settlement/runtime-authority consolidation remain open.

### Obsolete stop-interface consolidation accepted

Removed `SessionStopper`/`StopErrandSession` and required the existing identity-aware `ErrandTeardown` directly, retaining the `Stopper` field name. Cancellation, persistence and runtime-absence algorithms are unchanged. Ten files changed; production code decreased by 39 lines. The baseline passed 124 tests; final targeted race checks passed 125 tests with no skips or race reports. Caller build, scoped lint, formatting and whitespace checks passed. Exact selectors and limits: `/tmp/errands-stopper-cleanup-handoff.md`. The known unfinished Task14 queue test was isolated and restored during affected validation; all five Task14 files retain their hashes. These are isolated targeted passes, not full-worktree acceptance. Duplicate settlement paths and runtime-absence authority remain open.

### Exact settlement consolidation accepted

Made the existing exact terminal-decision capability mandatory on Store and removed the legacy latest-turn settlement/reconciliation fallback. The sole production SQLite store already implements this capability; its settlement algorithm and cancellation policy are unchanged. Four production files and fourteen test files changed, with no new repository files: production decreased by 184 lines and tests increased by 125 lines to exercise actual persistence evidence. All 56 tests in changed test files and their 36 subtests passed with race detection; caller build, scoped lint, formatting and whitespace checks passed. All five Task14 files retain their hashes. Evidence: `/tmp/errands-settlement-cleanup-handoff.md`.

Two orchestrator parent-settlement fixture tests (15 subcases) fail identically before and after cleanup because reserved launch execution evidence is missing. An unchanged await-recovery test also has a pre-existing teardown fake pointing at an empty memory store. These remain explicit fixture reconciliation items alongside the mobile flag-off test; no PostgreSQL execution or full-worktree acceptance is claimed. Runtime-absence authority remains the next consolidation boundary.

### Runtime-absence consolidation accepted

Launch recovery and exact teardown now share only their matching positive non-nil inventory predicate: completed exact cleanup, stopped without a PID, or confirmed dead process. Caller-specific errors, pending authenticated recovery fences and nil handling are preserved. Launch rejects nil inventory; teardown retains its existing compatibility branch, which the production SQLite reader does not produce. Three existing source/test files changed, production decreased by three lines and a 13-scenario matrix added 96 test lines. The isolated targeted race suite passed 62 tests and 202 subcase nodes; caller build, scoped lint, formatting and whitespace checks passed. Evidence and contract rationale: `/tmp/errands-runtime-absence-handoff.md`.

The same two parent-settlement fixture failures and held Task14 queue compilation blocker remain unchanged. All five Task14 hashes match. This closes the recorded runtime-absence duplication finding without changing safety policy; the previously recorded caller/await-recovery/mobile fixture gaps still require reconciliation before combined acceptance.
