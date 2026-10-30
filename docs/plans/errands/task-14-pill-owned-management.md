---
id: "14-pill-owned-management"
title: "Keep errand management in the pill and panel"
status: done
wave: 15
depends_on:
  - "13-inline-chat-summary"
  - "09-e2e-and-docs"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-010.11
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.7
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 14: Keep errand management in the pill and panel

Implement the approved [2026-10-30 refinement](plan.md#accepted-ui-refinement-2026-10-30). Remove the duplicate live inventory from parent transcripts, retaining compact start and optional await entries plus immutable finished cards. Keep task-wide grouped live/completed errands, Open and Stop all in the existing pill popover/drawer. Historical task 13 remains a delivery receipt.

## Scope and ownership

Remove `ErrandsInlineSummary` and its exclusive `transcriptTail` plumbing from `TaskChatPanel`, `message-list-shared.tsx` and `message-list-native.tsx`. Retire tests solely for the removed component/seam and migrate existing desktop, phone and opt-in provider pilot selectors to the pill/panel. `e2e/helpers/errands.ts` owns the causal notification-turn completion wait used by both viewport tests. Preserve shared state/loading, actions, localization, message reconciliation, `detailActive` and historical diff anchors. Update `docs/public/coordination.md` and its coverage manifest. No backend, runtime toggle or contract changes.

## Acceptance and UI preview

```text
Desktop parent chat               Phone parent chat
[start: title/profile/status Open] [start: title/profile/status Open]
[finished: result snapshot Open]   [finished: result snapshot Open]
[composer]                        [composer]
[Errands pill] -> popover          [Errands pill] -> bottom drawer
  parent group                      parent group
  live + completed rows / Open      live + completed rows / Open
  Stop all (while live)             Stop all (while live, fixed footer)
```

The existing Errands drawer is the mobile exemplar: a temporary grouped navigator with an internally scrolling list, safe-area clearance, viewport containment and 44px phone/coarse-pointer targets (also on narrow fine-pointer layouts). Open navigates to full-screen child chat with Back. Desktop keeps the compact popover. Both use existing domain hooks. Chat keeps one scroll owner, a scrolled-up reader's position and bottom-follow as results arrive. Start entries survive completion separately from delivered snapshots; all-completed errands remain accessible after reload.

## Verification

Use the existing host test to prove the removed mount fails before implementation. Extend existing browser journeys for absence, history retention, completed access, navigation, Stop all and scrolling; preserve other delivery scenarios.

```bash
cd apps/web
pnpm exec vitest run components/task/task-chat-panel.last-prompt.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errand-delivery-message.test.tsx components/task/chat/messages/kandev/errand-renderers.test.tsx components/task/chat/turn-changed-files-card.test.tsx components/task/chat/messages/todo-message.test.tsx hooks/domains/session/use-task-errands.test.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

From root, run changed-file ESLint, then managed production-build browser suites separately with one shard:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/session/errands.spec.ts e2e/tests/git/turn-changed-files-scroll.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts e2e/tests/git/mobile-turn-changed-files-scroll.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The provider pilot remains opt-in; migrate its entry point without claiming a fresh provider certification.

## Dependencies, risks and parallelism

Tasks 13 and 09 are accepted. Execute sequentially: shared host/list integration and browser tests touch the same surfaces. Do not delete shared panel copy/hooks or move historical cards. Do not preserve obsolete tests that require the removed inventory. No shared plan writes from investigation agents.

## Results

Completed 2026-10-30. Removed the inline inventory component and its exclusive native transcript slot; retained the shared pill/panel, start/await renderers, immutable finished cards, domain hooks and scroll owner. Retired the removed component/slot tests and converted the existing host regression to reject a transcript inventory. The host regression failed before production removal; the final focused run passed nine suites and 183 tests.

Managed production-build E2E passed eight desktop and five phone scenarios with one worker and no retries. Coverage includes conversational parents, start entries surviving completion/reload, immutable snapshots through follow-up, completed pill access, Open/Back, Stop and task-wide Stop all, optional await, two-client deletion, flag-off restart, reader position, bottom-follow and existing delayed changed-file scrolling. Phone coverage also proves a scrolling contained drawer, a fixed bulk-action footer, 44px controls at phone/coarse-pointer and 767px fine-pointer layouts, and no page horizontal overflow. Inspected drawer, full-screen child and finished-history screenshots against the assigned composition. Final runs reused the freshly rebuilt managed production assets with `--host --shards 1 --no-build --retries=0`; no production source changed between build and final runs.

The tests now wait for the submitted history turn's persisted final message, the actual completed notification turn before releasing the next result, and settled drawer containment. The phone reader fixture uses one long message so history pagination does not confound its scroll-position assertion; the desktop test retains bulk history and reads away from the top pagination boundary.

Typecheck passed with `NODE_OPTIONS=--max-old-space-size=6144` after the default Node heap exhausted memory. Changed-file ESLint, localization validation/ratchet, spec catalog/lint, 63 public-doc validator tests, the 54-page public-doc check and `git diff --check` passed. No locale copy or backend behavior changed. Public docs updated: `docs/public/coordination.md` (how-to guide) and its source coverage manifest. The opt-in provider pilot selectors were migrated; no fresh real-provider pilot was run.

Evidence: `/tmp/errands-refinement-20261030/` contains the red/green unit logs, final desktop/mobile logs, typecheck/localization/docs logs and inspected phone screenshots. Changes remain uncommitted.
