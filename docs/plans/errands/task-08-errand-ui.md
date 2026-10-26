---
id: "08-errand-ui"
title: "Build the errand UI for desktop and phone"
status: in_progress
wave: 6
depends_on:
  - "03-frontend-data-layer"
  - "06-errand-mcp-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.3
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.6
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.6
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-010.9
  - AC-TASKS-ERRANDS-011.1
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.3
  - AC-TASKS-ERRANDS-011.4
  - AC-TASKS-ERRANDS-011.5
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 08: Build the errand UI for desktop and phone

## Summary

Render errands in the parent transcript, including **Errand finished** cards
for delivered results, add the errand view header with **Stop** or
**Send result to parent**, add the errands chip and panel with
**Stop all errands** that appear only when a task has errands, keep the
parent composer enabled while errands run, hide primary and handoff actions
for errands, and compose the phone flow with nested sessions, a full screen
errand view, and a bottom drawer. All copy is localized in the seven shipped
locales.

## In scope

- `components/task/chat/messages/kandev/errand-renderers.tsx`: start row
  card and await list card, registered in `registry.ts`; **Errand finished**
  cards matched in `message-renderer.tsx` by `errand_delivery` metadata, one
  per item, below any visible user text. The status block never renders as
  text.
- `components/task/chat/errands/errand-header.tsx` for errand sessions with
  the back link, objective disclosure, profile, status, workspace path and
  branch, and **Stop** while live or **Send result to parent** when eligible,
  never both, using `use-errand-actions`.
- `errands-chip.tsx` (labelled with the live count) and `errands-panel.tsx`
  in `chat-status-bar.tsx`, following `background-work-chip.tsx`: popover on
  desktop and bottom drawer on phones through `useResponsiveBreakpoint`,
  rendered only when the task has errands, with a **Stop all errands** footer
  that sends `errand.cancel_all` while any errand is live.
- Errand state never disables the parent's composer. Errand views keep the
  composer and its MCP explorer like any session, so the user can add MCP
  servers to an idle errand.
- **Open** adds or focuses the errand's `session:<id>` panel without
  `session.set_primary`; `session-tab-menu.tsx` and the phone session menu
  omit **Set as Primary** and **Handoff** for errands.
- `mobile-sessions-section.tsx` nests errands under their parent; the phone
  errand header shows **Back to parent** and a visible actions menu.
- One polite `role="status"` region per view, accessible names, keyboard
  operation, coarse-pointer 44px targets, internal drawer scrolling.
- Copy in `task.json` and `chat.json` for all seven shipped locales, plus
  `pnpm run i18n:zh-hant`; no em dash in copy.

## Out of scope

- Row indicators and the Threads switcher (task 07). E2E specs (task 09).
- A grouped transcript block for consecutive start calls and a dockview
  Errands panel.

## Acceptance

1. Desktop renders UI-01 to UI-04 and UI-09 from store data: row cards, the
   optional await card, **Errand finished** cards, the on-demand errand view
   whose header shows **Stop** only while live and **Send result to parent**
   only when eligible, the chip and grouped popover with
   **Stop all errands** only when errands exist, and an enabled composer.
2. Phone renders UI-06 to UI-08 and UI-10: nested sessions, a full screen
   errand view with back and actions menu, the chip drawer with a 44px
   **Stop all errands** footer, and full-width finished cards, with no
   horizontal overflow.
3. Component, renderer, and menu tests pass, and localization checks pass in
   every shipped locale.

## ASCII UI preview

Excerpts of [UI-01 to UI-04 and UI-06 to UI-10](plan.md#ascii-ui-preview).
Compare the rendered surfaces with the full previews and their structural
notes.

```text
UI-01 Desktop parent session after its turn ended
  [Errand  API audit      Codex / gpt-5.6   Running     [Open]]
  [Errand  Schema review  Claude / sonnet   Done        [Open]]
  Status bar: [Errands: 1 running]     <- only when errands exist
  [ Message Main...                 Send ]  <- composer stays enabled

UI-02 Desktop errand view
  Live:  < Main / Errand: API audit                        [Stop]
  Done after a user follow-up:
         < Main / Errand: API audit       [Send result to parent]
  Codex / gpt-5.6 . Running 2m . ~/.kandev/tasks/auth (feat/auth)

UI-03 Chip popover: groups by parent, each row [Open],
      footer [Stop all errands] while an errand is live
UI-04 Errand finished card: title, status, preview, [Open] per result
UI-09 Optional await card: "Waiting for 2 errands" with live statuses

UI-06 Phone Sessions drawer
  Main (Claude)          *   [.]
    API audit   Running      [>]      <- 44px rows

UI-07 Phone errand view
  < Main      API audit      [.]      <- [.] = Stop while live,
                                         or Send result to parent

UI-08 Phone chip drawer: grouped list scrolls inside the drawer,
      fixed 44px [Stop all errands] footer
UI-10 Phone parent: full-width Errand finished card, chip, enabled composer
```

These views cover AC-TASKS-ERRANDS-010.2 to 010.4, 010.7 to 010.9, 011.1
to 011.5, 003.3, 005.5, 007.1, and 007.6.

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/errand-renderers.test.tsx components/task/chat/messages/kandev/registry.test.ts components/task/chat/errands components/task/chat/chat-status-bar.test.tsx components/task/session-tab-menu.test.tsx components/task/mobile/mobile-sessions-section.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/errands components/task/chat/messages/kandev/errand-renderers.tsx components/task/chat/messages/kandev/registry.ts components/task/chat/message-renderer.tsx components/task/chat/chat-status-bar.tsx components/task/session-tab-menu.tsx components/task/mobile/mobile-sessions-section.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- New `apps/web/components/task/chat/messages/kandev/errand-renderers.tsx`
  and test; `registry.ts`; `apps/web/components/task/chat/message-renderer.tsx`.
- New `apps/web/components/task/chat/errands/` (`errand-header.tsx`,
  `errands-chip.tsx`, `errands-panel.tsx`, `errand-finished-card.tsx`,
  tests).
- `apps/web/components/task/chat/chat-status-bar.tsx`, the chat panel that
  mounts the header, `apps/web/components/task/session-tab-menu.tsx`,
  `apps/web/components/task/session-tab.tsx`,
  `apps/web/components/task/mobile/mobile-sessions-section.tsx`.
- `apps/web/src/locales/*/task.json` and `chat.json`.

## Dependencies

Tasks 03 and 06.

## Risks

- Kandev tool renderers parse tool arguments and results from transcript
  messages; parsing must tolerate a pending call, an error result, and
  provider-specific tool-name prefixes.
- **Errand finished** cards read their snapshot from message metadata, not
  from the live errand row, so a later result never rewrites an older card.
- The chip must not mount subscriptions on tasks without errands; gate live
  effects by the same condition as the render.
- Shared dockview session activation rules apply: opening an errand tab must
  not switch the composer target of another visible panel.

## Parallelism

`sequential`

## Inputs

- [Plan previews](plan.md#ascii-ui-preview) and the
  [system design frontend and phone sections](../../specs/tasks/system-design/errands.md#frontend).
- `/mobile-parity`, `apps/web/AGENTS.md`, and `docs/i18n.md`.
- Exemplars: `background-work-chip.tsx`, `background-work-panel.tsx`,
  `tool-subagent-message.tsx`, `propose-task-renderer.tsx`.

## Results

Pending.
