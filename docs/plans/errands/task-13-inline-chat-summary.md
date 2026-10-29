---
id: "13-inline-chat-summary"
title: "Reuse interactive chat cards for current errands"
status: in_progress
wave: 13
depends_on:
  - "07-task-summary-indicators"
  - "08-errand-ui"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.6
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-010.9
  - AC-TASKS-ERRANDS-010.10
  - AC-TASKS-ERRANDS-011.6
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 13: Reuse interactive chat cards for current errands

## Summary

Add a compact, expandable current-Errands summary inside the parent chat,
following the native changed-files/todo card pattern shown by the user.
Reuse the existing Errands store, grouped rows, navigation and bulk action.
Historical start, await and finished cards retain their identities.

## In scope

- New `errands-inline-summary.tsx`: bordered shell, default-collapsed
  semantic disclosure, total/live counts, attention cue, and task-wide
  **Stop all errands** while any errand is live. Expanding lists parent
  groups and existing title/profile/status/**Open** rows. Keep actions
  separate from disclosure; opening/stopping must not toggle the card.
- Use `@kandev/ui/collapsible` and the visual hierarchy of
  `TurnChangedFilesCard`. Todo/tool `ExpandableRow` is a visual precedent,
  not a ready-made accessible control: do not copy its clickable `div` or
  hover-only disclosure affordance. No new package or widget framework.
- Add a narrow optional `transcriptTail` React-node prop through
  `MessageListProps` in `message-list-shared.tsx`, supplied by `TaskChatPanel`.
  Mount it inside `NativeMessageListBody` after `TranscriptMessageItems`
  and before `NativeMessageListFooter`, under `data-chat-content` so the
  existing ResizeObserver and scroll-follow logic see expansion/live growth.
  Preserve upstream `detailActive` propagation and turn reconciliation.
- Render one summary per eligible normal parent chat while the feature is
  on and task errands exist; hide in errand views and empty/off states.
  Keep local expansion keyed to viewed task/session. Use existing selectors
  and shared load/reconnect state; mounting chip and summary must not create
  duplicate list requests, pollers or alternate counters.
- Reuse/factor list and action content from `errands-panel.tsx` and
  `errands-panel-row.tsx`, leaving overlay wrappers with the existing chip.
  Keep task-wide parent groups aligned with `cancelAllErrands(taskId)`;
  never show a parent-only list beside an unqualified task-wide bulk action.
- Phone: full-width card, visible 44px disclosure/Open/bulk targets and
  stacked header/actions when needed. Expand in normal chat flow, using its
  sole scroller. Open navigates to full-screen child chat with existing Back.
  Existing chip still opens its bottom Drawer while reading elsewhere.
- Reuse localized copy where possible; any new summary/disclosure labels
  go through `t()` and all shipped locales plus pseudo. Preserve the single
  existing polite status-announcement owner.

## Out of scope

- Replacing/grouping historical start calls or moving exact historical diff
  anchors. The new card shows current task state, not a synthetic old turn.
- A Dockview Errands panel, a generic plugin/renderer registry, rich-output
  catalog actions, new MCP/WebSocket contracts, or new lifecycle authority.
- Inline child composers, user-created errands, or additional row-level Stop
  UI. Individual Stop/follow-up/Send result remain in the opened child view.

## Acceptance

1. Exactly one current-state summary appears in the eligible parent chat,
   with live counts/attention, disclosure and task-wide Stop all. Expanded
   parent groups open the correct child without changing primary ownership;
   historical start/await/result cards and diff/todo surfaces remain intact.
2. Keyboard/touch disclosure and independent actions work; empty/off/child
   states hide the card, live changes update it, task/session switches isolate
   local expansion, and chip plus inline mounting shares one list request.
3. Desktop remains compact; phone controls meet 44px targets without overflow
   or nested vertical scrolling. Expansion and asynchronous updates preserve
   a reader's position when scrolled up and bottom-follow when already active.

## UI preview

UI-11 in [the plan](plan.md#inline-chat-summary-ui-11) owns the combined
desktop/phone preview for AC-010.10 and AC-011.6. Structure is required;
illustrative labels use localized copy in the implementation.

- Collapsed: disclosure, Errands total/live counts, attention, independent
  Stop-all action. The action is absent when no task errand is live.
- Expanded: parent heading, child title/profile/status and Open; repeat for
  each parent. No second composer, drawer or scroll region inside the card.
- Phone: stack header/actions if space requires it, retain 44px targets and
  let the main chat own scrolling. Open/back uses the existing full-screen
  errand view; the parent's composer remains available.

## Verification

Run from root after task 00 installs dependencies. The new inline-summary,
native-list and chat-panel Errands suites are planned files, created with
TDD. Existing exemplar suites guard unrelated diff/todo behavior.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errands-inline-summary.test.tsx components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errand-delivery-message.test.tsx components/task/chat/message-list-native.errands.test.tsx components/task/task-chat-panel.last-prompt.test.tsx components/task/chat/turn-changed-files-card.test.tsx components/task/chat/messages/todo-message.test.tsx hooks/domains/session/use-task-errands.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/errands/errands-inline-summary.tsx components/task/chat/errands/errands-panel.tsx components/task/chat/errands/errands-panel-row.tsx components/task/chat/message-list-native.tsx components/task/chat/message-list-shared.tsx components/task/task-chat-panel.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Review converter output and keep only intended new Traditional Chinese keys.
Task 09 adds managed desktop/phone interaction, scroll and reload proof.

## Files likely touched

- New `apps/web/components/task/chat/errands/errands-inline-summary.tsx`
  and test; existing panel/row/list/action content only as needed for reuse.
- `apps/web/components/task/chat/{message-list-shared.tsx,message-list-native.tsx}`
  and new `message-list-native.errands.test.tsx`.
- `apps/web/components/task/task-chat-panel.tsx` and new
  `task-chat-panel.last-prompt.test.tsx`.
- `apps/web/src/locales/*/task.json`; shared selectors/hooks only if a
  focused regression proves existing projections insufficient.

## Dependencies

Tasks 07 and 08; corrected live-state and delivery behavior is supplied
transitively by 10-12. Task 09 proves this additional UI outcome in browsers.

## Risks

- The screenshot's diff/todo surfaces are separate native components, not an
  existing generic widget API. Keep the new composition seam small.
- Mutating historical cards from today's store would destroy result history.
- Hidden sibling groups would misrepresent task-wide Stop-all scope.
- Mounting outside the observed chat content breaks height/scroll tracking;
  nesting controls inside the disclosure breaks independent actions.

## Parallelism

`sequential`

## Inputs

- [Inline summary design](../../specs/tasks/system-design/errands.md#inline-current-errands-summary).
- `TurnChangedFilesCard`, `TodoMessage`, existing Errands panel/row/actions,
  `/mobile-parity`, and scoped chat guidance.
- [Turn changed-files design](../../specs/tasks/system-design/turn-changed-files.md)
  for exact historical placement and scroll ownership; this card is current state.

## Results

Implementation checkpoint, 2026-10-29: the default-collapsed task-wide card,
shared grouped rows/actions, profile labels, and native transcript tail are
implemented. Component and host tests pass; removing either host connection
makes its regression fail. Host coverage extends the existing chat-panel
harness instead of duplicating it in a new test file. Shared loading and
historical card tests pass. Final lint, typecheck and localization checks
passed. Desktop/phone scroll proof remains pending; this is not final acceptance.
