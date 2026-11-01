---
id: "23-visible-status-announcements"
title: "Announce rendered Threads status changes"
status: done
wave: 23
depends_on:
  - "20-context-snapshot-freshness"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRANDS-010.6
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 23: Announce rendered Threads status changes

## Scope and authority

Repairs review finding(s) 17 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Production scope is limited to the status announcer's activation condition in `task-chat-panel.tsx`.

## Low-level implementation and regression plan

Owned source apps/web/components/task/task-chat-panel.tsx, ErrandStatusAnnouncer mount. Keep ThreadConversation's isVisible=false unread policy. Reuse the ALREADY computed transcriptIsVisible = panelId === null || isVisible for active={transcriptIsVisible && detailActive}. This is sufficient and narrower than adding another unrelated visibility prop: visible non-Dockview previews/Threads already use this same distinction for transcript geometry. ThreadColumnBody mounts ThreadConversation only while isDetailActive and membership/selected session are ready; hidden Dockview panels have panelId and isVisible=false. Keep that outer activation contract.

Do not set isVisible=true in Threads, change read cursor/unread-divider semantics, mount a second announcer, add a background subscription, or announce initial baseline history.

Owned browser additions to apps/web/e2e/tests/session/{errands,mobile-errands}.spec.ts, using actual Threads and actual TaskChatPanel/ErrandStatusAnnouncer:
- 'desktop Threads announces an errand transition without marking the column read': create actual running child with gate; navigate to Threads; ensure parent child-start UI has rendered baseline; release gate; assert exactly one polite errand-status-announcer for that visible chat contains title + localized Done; assert read cursor unchanged via API.
- Phone equivalent through existing native Threads route/picker. Only snapped active column announces; hidden/offscreen columns have no detail transcript/announcer. On first load announcer is empty; after later parent-origin followup running -> done it updates again.
- Existing hidden-chat subscription/activation tests remain relevant. Assert a hidden sibling does not announce or subscribe; reveal starts a new baseline without replaying all prior statuses. Keep one region per view, not one globally across visible split conversations.

UI-ANNOUNCE-01:
~~~
Desktop Threads               Phone Threads (snapped active column)
[Task A] [Task B]              [Task A / Sessions v]
Audit: Running -> Done         Audit: Running -> Done
[Message parent ...]           [Message parent ...]
(sr-only polite status)        (one sr-only polite status)
Unread cursor unchanged        Offscreen Task B has no detail announcer
~~~

No visible label/layout/scroll/hitbox change; existing Errands drawer and native Threads column are exemplars. Reuse errandStatusAnnouncement translations. Parent/child transcript, actions and composer retain existing targets; no new mobile surface.

~~~
(cd apps/web && pnpm exec vitest run components/threads/thread-column-activation.test.tsx components/task/task-chat-panel.scroll-target.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/task-chat-panel.tsx e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --grep 'desktop Threads' --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --grep 'phone Threads' --retries=0
~~~

Run desktop/phone sequentially. A component test that mocks ErrandStatusAnnouncer and asserts active=true is useful wiring evidence only; it cannot replace these rendered live-text/read-cursor assertions.

## Dependencies and ownership

[Task 20](task-20-context-snapshot-freshness.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Task 23 is done. The announcer now activates from `transcriptIsVisible && detailActive`, while Threads retains `isVisible={false}` for read-cursor behavior and Task29 retained parents remain inert.

- TDD RED: desktop Threads rendered the real running child and left the announcer empty after completion; the test failed on the expected title assertion. Log: `/tmp/errands-task23-desktop-red-expected.log`.
- TDD RED: phone Threads kept an offscreen completed errand silent, mounted it with an empty baseline when opened, and failed on the expected title assertion after a later parent-origin follow-up completed. Log: `/tmp/errands-task23-phone-red-expected.log`.
- GREEN desktop: `pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --grep "desktop Threads" --retries=0` passed 2/2, including the new live-text, one-region and unchanged-read-cursor assertions. Log: `/tmp/errands-task23-desktop-green.log`.
- GREEN phone: `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --grep "phone Threads" --retries=0` passed 2/2 on the configured Pixel 5 project, including the offscreen no-transcript/no-announcer check, quiet reveal baseline, follow-up announcement and unchanged read cursor. Log: `/tmp/errands-task23-phone-green.log`.
- Adjacent visibility tests: `pnpm exec vitest run components/threads/thread-column-activation.test.tsx components/task/task-chat-panel.scroll-target.test.tsx` passed 2 files and 67 tests. Log: `/tmp/errands-task23-vitest.log`.
- Scoped ESLint passed with zero warnings, and Prettier passed for the panel and both E2E files. Log: `/tmp/errands-task23-lint-format-final.log`.
- `make build-web` passed after the production edit. Log: `/tmp/errands-task23-web-build.log`.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed. Log: `/tmp/errands-task23-typecheck.log`.

The first desktop fixture draft assumed a second column was also in the active detail window; it was narrowed to the visible desktop chat. The phone acceptance uses the current native Threads route to reveal the hidden task after the picker attempt did not activate its detail transcript. No read-cursor contract, public docs, shared manifest, backend or schema files were changed.
