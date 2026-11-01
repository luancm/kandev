---
id: "29-conversation-navigation-and-history"
title: "Separate errand navigation and unify compact chat markers"
status: done
wave: 29
depends_on:
  - "27-history-disclosure"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-ORDER-001
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-011.1
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.7
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-await-duration.md
  - ../../specs/tasks/system-design/errand-conversation-navigation.md
  - ../../specs/tasks/system-design/errand-panel-order.md
---

# Task29: Conversations, delegated work and quiet history

## Approved outcome and boundaries

The user approved implementation after clarifying normal conversations versus parent-owned errands and Option C for every historical marker. Reconcile existing owning requirements/design first, then implement in this same packet without a new approval checkpoint. Preserve Task27's completed WAIT behavior, Task19's partial reconnect work, backend edits and all untracked files. No backend production changes, shared manifest changes, commits, push, live instance changes or delegation.

Own frontend conversation navigation (Dockview, preview, Threads and phone), Errands panel/history renderers, related locale keys, focused tests/E2E and this work order. Task21 preview navigation and Task22 role isolation may be repaired here; their original exact browser acceptance remains a separate gate unless explicitly demonstrated. Their former child-tab affordances are superseded by the approved temporary conversation contract; do not invent a child tab to satisfy an obsolete selector.

## Navigation state and ownership

Normal session IDs own regular tabs. The task route/store continues selecting the actual displayed conversation, including a child, so direct child URLs and reload retain transcript access. Desktop resolves that selected child's parent as the tab host and renders the child only inside that host; regular-tab selection returns to a normal conversation. Remove restored child panels only after authoritative session hydration. Keep retained role restrictions with flag on and off. Missing parent must retain direct transcript access with a safe fallback rather than promoting an errand.

Keep the normal parent's mounted conversation region and viewport while a temporary child covers it. On phones the retained region includes the Sessions picker and chat together, preserving the covered transcript's geometry. The covered view is invisible and inert, with read tracking, rich detail demand, recovery, announcements and workbench shortcuts paused. Back reactivates the same parent view and preserves its reader position and local state. This shared composition serves desktop, phone, preview and Threads; it does not retain unrelated or invalid parent conversations.

Preview owns its controlled selected session and session/Plan mode through `TaskSessionNavigationContext`; foreign-task targets or absent callbacks do nothing. Threads keeps column-local selection. Neither path may select an unrelated workbench or transfer primary/workflow ownership. The parent chat chip filters its children from the existing shared task cache; active work is visible, completed history is initially collapsed. Task-wide listing summaries and Stop-all semantics remain as established.

START, completed WAIT and COMPLETION are historical semantic disclosures. Collapsed event/count lines omit repeated titles; recorded duration/end reason remain where available. Expanded details retain titles, objectives/profiles, snapshot statuses/results and Open. Completed events never acquire a live spinner or today's status. In-flight wait behavior is retained from Task27.

## Desktop and phone previews

UI-T29-01, desktop (normal conversations remain tabs; child is temporary content):

```text
[Main conversation] [Review conversation] [Plan]
Main chat                              [Errands: 2 active]
> Started 1 errand
> Waited 30s for 1 errand · Timed out
> 1 errand finished

Errands panel for Main
Active
  Audit · Running · context               [Open]
> Completed (12)

Open Audit, same Main tab:
[Back to parent] Audit                    [Stop]
Full child transcript + composer
Back returns Main transcript; Review remains independently selectable.
```

UI-T29-02, phone (exemplar: current Errands drawer and mobile session picker):

```text
Task / [Sessions: Main]
Main transcript (one scroller)
> Started 1 errand             (44px disclosure)
> Waited 30s for 1 errand
> 1 errand finished
Composer                      [Errands]

Inset bottom Errands drawer, fixed title
Active
  Audit · Running             (44px Open row)
> Completed (12)               (44px disclosure)
Internally scrolling list
[Stop all errands]            (fixed, safe-area aware)

Tap Audit -> full-screen child chat
[Back to parent] Audit        [Actions]
Child transcript + composer
```

Share session selection, role classification, filtering and historical model across desktop/phone. Phone presentation uses bottom drawers for temporary choices and full-screen chat for dense content, keeping one main scroller, `dvh` containment, safe areas and no horizontal overflow. Regular Sessions lists normal conversations; the Errands drawer retains every child including completed ones. Use the canonical responsive hook and 44px phone/coarse-pointer hitboxes; ordinary desktop actions remain compact. Preserve desktop preferences.

## TDD and validation

Primary unit contracts: child Open selects the actual child without creating its tab; parent tab host resolution does not alter ownership; legacy restored child panels disappear without deleting sessions; ordinary siblings remain; direct/retained/orphan child resolution is deterministic; preview owner scopes Open/Back; Threads/mobile normal selectors omit errands; historical START never follows live completion. Extend existing tests for distinct boundaries, rather than markup-only new tests.

Browser contracts: real MCP-created errands and multiple normal conversations; panel active work plus many completed rows; Open/Back and parent switching; child direct URL/reload; restored legacy child tabs; preview and Threads local selection; role guards with enabled/disabled flag; all three historical markers collapsed/expanded/static after reload. Native Pixel5 proves real touch, 44px targets, drawer internal scroll and containment, full-screen child/back and no document overflow. Inspect fresh screenshots. Preserve Task27 await cases and real delivery/scroll checks.

Run every heavy command serially with `flock /tmp/kandev-errands-repair-build.lock`. Use `VITEST_MAX_WORKERS=1`, `NODE_OPTIONS=--max-old-space-size=8192`; Go build cold-cache constraints `GOMAXPROCS=2 GOFLAGS=-p=1`. No parallel builds or browser runs. Retain and poll all command handles.

Commands (from `apps/web` unless stated):

- `pnpm exec vitest run <exact changed navigation/parser/renderer suites>`; capture RED/GREEN and final test counts.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`.
- `pnpm exec eslint --max-warnings 0 <exact changed source/test/E2E files>`; exact-file Prettier.
- `pnpm run i18n:zh-hant --namespace chat` (and task if needed), preserving unrelated inherited catalog changes; `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- `../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --grep '<task29 and preserved history scenarios>' --retries=0`; exact preview/Threads specs when those gates are attempted.
- Same managed command with `--project mobile-chrome e2e/tests/session/mobile-errands.spec.ts` and focused grep. Build current source sequentially before acceptance.
- Repository root: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `git diff --check`.

## Results

Approved UI changes are implemented and current desktop/phone acceptance passes. The inherited global typecheck gate is now closed; the order remains `in_progress` for parent integration and the separate Task25 media gate. Normal tabs, the desktop Agents reopen menu and mobile session selectors exclude errands, while parent-owned temporary child views preserve full transcripts, explicit Back, direct URLs and reload. The reopen menu filters by the shared normal-session role predicate before sorting and numbering, so retained errands stay excluded with the feature off and the New Agent action remains available when no normal sessions exist. Legacy child panels are pruned after authoritative hydration. Preview/Threads navigation stays local, and persisted role prevents automatic child adoption with the feature on or off. The panel scopes its list to the rendered parent and initially collapses completed history while preserving task-wide Stop-all. START, completed WAIT and COMPLETION share compact historical disclosures; expanded details retain titles, outcomes, recorded results and Open. Task27's WAIT semantics are preserved.

`TaskConversationPanel` and `TaskConversationView` retain the complete parent conversation region, with inactive detail/read/recovery/shortcut demand suppressed. Phone retains the picker and chat together; preview and Threads retain their conversation body by task. `useRetainedReaderPosition` preserves an above-bottom reader's visible message anchor through late browser correction and content reflow, releasing its CSS/listeners/observers for user input, explicit host/local targets, history reset, a hidden normal view or unmount. It yields to bottom following and does not capture an unseen parent behind a direct child URL. Native scroll modules are unchanged. Browser tests use the actual Jump-to-latest control to prove intentional user navigation releases this owner.

Exact ownership inventory: `/tmp/errands-task29-files.txt` (86 paths: 62 frontend source/test/E2E files, 16 locale files and eight documents). `/tmp/errands-task29-source-files.txt` lists the frontend paths relative to `apps/web`. New modules are `lib/task-conversation-navigation.ts`, `components/task/task-conversation-view.tsx`, `components/task/chat/use-retained-reader-position.ts` and `components/task/chat/errands/errand-history-disclosure.tsx`, with adjacent meaningful navigation/state tests. This final integration packet also owns the desktop Agents reopen menu and nine existing connection-state test fixtures. Existing Dockview, preview, Threads, mobile composition/picker, chip/panel, parser/renderers, header, role-adoption handler and focused browser suites were updated. No backend production changes or shared manifest edits were made.

### Verification receipts

- Current focused navigation, ownership, history, role-adoption, retained-view, native scrolling and viewport checks: 571 tests in 26 files passed, `/tmp/errands-task29-unit-current.log`, including 12 reader ownership cases.
- Exact 51-file ESLint passed with zero warnings, `/tmp/errands-task29-lint-current.log`; Prettier completed, `/tmp/errands-task29-prettier-current.log`. Current web production build passed, `/tmp/errands-task29-build-web-current.log`. Host runtime/mock-agent/delivery/plugin fixture builds passed earlier, `/tmp/errands-task29-build-host-final.log`; no backend source changed afterward.
- The final focused integration/helper-consumer run passed 161 tests in 15 files, `/tmp/errands-task29-final-fixture-tests.log`. It covers the menu, shared conversation-role predicate, all nine corrected fixtures and consumers of the shared session-read fixture.
- The earlier typecheck log recorded 17 missing `connection.generation` fields across nine fixtures. Those fixtures now provide generation zero for initial state, while reconnect tests use `setConnectionStatus` so the generation reflects the transition. Full web typecheck passed, `/tmp/errands-task29-final-typecheck.log`.
- The real-browser menu regression reproduced 10 rows for the 3-normal/7-errand fixture before the fix, `/tmp/errands-task29-menu-red.log`; the mounted unit regression also failed on the child row, `/tmp/errands-task29-menu-unit-red.log`. After the fix, the fresh-assets desktop browser run passed both mixed-parent and feature-off retained-child scenarios, `/tmp/errands-task29-final-menu-e2e.log`. It asserts normal rows numbered 1–3 while a child is selected and a role-persisted child omitted after the feature is disabled.
- Final web assets built once after the production change with `pnpm run build:vite`, `/tmp/errands-task29-final-build-web.log`; the managed browser rerun used `--no-build` with the backend/plugin artifacts built after the latest modified backend source.
- Scoped ESLint across the 13 changed frontend/E2E files passed with zero warnings, `/tmp/errands-task29-final-lint.log`; the final E2E spec lint rerun passed after the flag-off assertion, `/tmp/errands-task29-final-lint-e2e.log`. Prettier wrote/checked the same files, `/tmp/errands-task29-final-prettier.log` and `/tmp/errands-task29-final-prettier-check.log`.
- Fresh current-build desktop acceptance: 11/11 passed with zero retries, `/tmp/kandev-run.e2e.CAUesc1l.log` (wrapper `/tmp/errands-task29-browser-desktop-current.log`). Native Pixel5 acceptance: 8/8 passed with zero retries, `/tmp/kandev-run.e2e.RnwKOqu5.log` (wrapper `/tmp/errands-task29-browser-phone-current.log`). This includes real Open/Back, retained DOM state in preview/Threads, multiple normal parents and seven errands, legacy restored panels, direct URLs/reload, role/flag-off retention, historical disclosures, exact reader preservation through delivery, intentional Jump-to-latest handover and subsequent bottom following.
- Fresh screenshots were inspected and saved under `/tmp/errands-task29-artifacts/{desktop-current,phone-current}`. Task29 history/child images show normal-only tabs, scoped scrollable history, full child conversation/Back, fixed drawer controls and containment. Browser assertions prove initial completed collapse, 44px native targets and one visible transcript with no horizontal overflow. These light fixture captures do not satisfy Task25's curated dark media gate.
- Prettier check, i18n check/ratchet, documentation catalog, spec lint and global diff/owned-untracked whitespace checks passed: `/tmp/errands-task29-{prettier-final,i18n-final-current,i18n-ratchet-current,catalog-current,spec-lint-current,diff-check,untracked-whitespace}.log`. Catalog validated 422 decisions and 1862 specifications; the spec linter's 36 tests passed at `/tmp/errands-task29-spec-lint-tests.log`.
- Behavioral RED/GREEN receipts include initial desktop reader loss (`/tmp/kandev-run.e2e.wpYWjZ35.log`), late phone correction after an initially correct restore (`/tmp/kandev-run.e2e.GtcNJ5EV.log`), deterministic late-reader regressions (`/tmp/errands-task29-reader-late-{red,green}.log`) and intent/visibility RED (`/tmp/errands-task29-reader-yield-red.log`) followed by the final 571-test GREEN. Incremental receipts remain in `/tmp/errands-task29-progress.md`. Every heavy command ran serially through `/tmp/kandev-errands-repair-build.lock`.

### Remaining gates and parent integration

Current Task29 UI, targeted browser, helper-consumer, typecheck and scoped frontend verification gates pass. Parent integration must update the shared manifest and public coordination guidance (the existing task-wide grouped panel/full-card wording is stale); those files remain outside this worker's write scope. Tasks21/22 have overlapping implementation repairs and regression evidence, but their original acceptance includes superseded child-tab affordances; their statuses remain unchanged for parent reconciliation. Task25's fresh curated dark media remains open; light E2E screenshots do not satisfy it.

No shared-checkout commits/push, live-instance mutation, Kandev task/session mutation or delegation. Backend and Task19 edits, all untracked source and Task27 remain preserved. All command handles are consumed. `/tmp/errands-task29-process-audit-final.log` records zero residual processes across 25 owned fixture roots and the released build lock. Source/build writer is released at handoff.


### Primary integration checkpoint

The desktop +/Agents menu follow-up now excludes errands using the existing tab role predicate before sorting and numbering, including retained disabled-feature errands. Normal role-less legacy sessions and New Agent remain available. Full web typecheck passes after fixture compatibility repairs. The follow-up records 161 focused tests, scoped lint/format checks, and 2/2 current-assets desktop cases, with the existing unchanged phone evidence retained. Public coordination guidance was reconciled to parent-owned navigation and compact historical disclosures; public-doc tests passed 63/63 and the validator passed all 54 pages (`/tmp/errands-task29-public-doc-tests.log`, `/tmp/errands-task29-public-doc-validation.log`).

Primary integration is closing the original Task21/22 preview-navigation and role-action acceptance against the approved temporary-conversation UI. Those work orders must no longer require a permanent child tab or a child in the regular phone picker. Task25 curated dark media is a separate work order with its own dependencies; it is not an implementation dependency of Task29. No overall repair-package completion is claimed.


### Final primary acceptance

Task29 is done. The original Tasks21/22 are now reconciled to temporary child views and marked done, with a fresh preview browser pass and 77 adjacent tests. The new phone ownership-action assertions were included in that executed unit suite; unchanged native Pixel5 browser evidence remains valid. Desktop +/Agents filtering and fixture compatibility passed their follow-up checks, including full web typecheck. Public coordination guidance and shared manifest are reconciled; public-doc validation passed 63/63 tests and all 54 published pages. Catalog validation passed 422 decisions and 1862 specifications. Historical in-progress statements above describe earlier handoffs and are superseded by this acceptance.

The separate Task25 curated dark-media work remains open under its own dependency chain, as do the other repair work orders. It is not a Task29 prerequisite. This acceptance does not claim the complete Errands repair package is finished, deployed or committed.
