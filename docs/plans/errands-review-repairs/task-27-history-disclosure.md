---
id: "27-history-disclosure"
title: "Compact historical await disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-AWAIT-DURATION-001
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.1
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.3
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.4
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.11
system_design:
  - ../../specs/tasks/system-design/errand-await-duration.md
  - ../../specs/tasks/system-design/errands.md
---

# Task 27: Compact historical await disclosure

User explicitly selected Option C and authorized implementation. The mockup is `.tmp/errands-history-options/index.html`. Own await renderer/parser and adjacent focused tests, relevant locales, existing desktop/phone Errands browser cases. No backend changes. Serialize these files before Task 04 presentation changes and Task 24 final browser validation.

## Behavior and design

Completed await renders a muted, borderless disclosure, initially collapsed. Show count, actual duration and recorded end reason. Hide titles until expanded. Use a real semantic disclosure with keyboard access and 44px phone/coarse-pointer target; fine-pointer styling follows existing ActivityChip. Expanded detail identifies its historical time and preserves Open navigation and returned-result markers. No spinner or other activity animation on any completed snapshot. Preserve actual stored statuses using past-tense/contextual copy; generic pending means not finished then, not running. Do not substitute current live child status or modify stored payloads. In-flight waits remain live. Preserve errors, legacy missing fields, zero duration, result snapshots and scrolling. All new copy localized under existing rules.

```text
Desktop / phone collapsed:
> Waited 30s for 1 errand · Timed out

Expanded:
v Waited 30s for 1 errand · Timed out
  At the end of this wait
  Audit checklist                [Open]
  Not finished
```

## Validation

Use TDD for changed parsing/view-model logic. Add a behavioral regression with a completed timeout pending/running snapshot and live done child: collapsed titles absent, expansion historical/static, Open works; later completion and reload never change the historical meaning. Cover keyboard disclosure and native phone tap/containment with existing browser cases. Avoid creating component tests solely for markup; follow current TDD and e2e skills. Run focused parser/renderer existing tests, typecheck, exact-file ESLint, i18n checks. Coordinate expensive builds/browser runs with primary; do not overlap managed browser suites. Record exact commands/results and pending browser gates honestly.

## Results

Recovery on 2026-10-30 retained the partial implementation, including the untracked `apps/web/components/task/chat/errands/errand-await.tsx`. Completed waits use a muted semantic disclosure, localized historical labels and static icons; titles, Open controls and returned-result markers appear only in expanded details. The parser/view helper keeps recorded pending distinct from running, preserves unknown legacy status and follows live state only for in-flight calls. No production logic was changed during recovery; the inherited focused tests were already green. Browser screenshots were added to the existing regressions for rendered inspection. The historical message seed was corrected to use an explicitly completed turn; it previously created an open synthetic parent turn that blocked subsequent result delivery.

Checks run from `apps/web` unless noted:

- `pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/messages/kandev/errand-renderers.test.tsx`: passed, 2 files / 52 tests; `/tmp/errands-option-c-unit-inventory.log`.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed; `/tmp/errands-option-c-typecheck.log`.
- `pnpm exec eslint --max-warnings 0 components/task/chat/errands/errand-await.tsx components/task/chat/errands/errand-parse.ts components/task/chat/errands/errand-parse.test.ts components/task/chat/messages/kandev/errand-renderers.tsx components/task/chat/messages/kandev/errand-renderers.test.tsx e2e/helpers/errands.ts e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts`: passed; `/tmp/errands-option-c-lint-inventory.log`. After screenshot additions, both modified browser files passed again; `/tmp/errands-option-c-lint-final.log`.
- Exact-file `pnpm exec prettier --check` over those eight files and the eight changed chat catalogs: passed; `/tmp/errands-option-c-prettier.log`. After screenshot additions and Traditional Chinese generation, the check passed again from `apps/web` (`/tmp/errands-option-c-prettier-final.log`); the later seed-helper correction was formatted separately and passed exact-file ESLint.
- `pnpm run i18n:check`: passed with existing unused-catalog warnings only; `/tmp/errands-option-c-i18n.log`. The final rerun after preserving preexisting Traditional Chinese copy also passed (`/tmp/errands-option-c-i18n-final.log`).
- `pnpm run i18n:zh-hant --namespace chat`: passed; `/tmp/errands-option-c-zh-hant.log`. New generated keys matched the inherited values; three older labels per locale were preserved after the generator rewrote them. No backend catalog diff retained.
- `pnpm run i18n:ratchet`: passed; `/tmp/errands-option-c-i18n-ratchet.log`.

Fresh managed desktop/phone browser gates and rendered inspection passed. The first root-cwd managed invocation stopped before build/discovery with `ERR_PNPM_BAD_PM_VERSION` (`/tmp/kandev-run.e2e.JCD0qCcv.log`); subsequent commands use the working `apps/web` toolchain without a configuration change.

Desktop attempt 1: `../../scripts/run-quiet e2e --summary -- pnpm e2e:run --project chromium e2e/tests/session/errands.spec.ts -- --grep 'optional await|completed await history|result delivery retains' --retries=0` ran against a fresh build, with 2 passed / 1 failed (`/tmp/kandev-run.e2e.crEy0kwP.log`). The real await/reload and reader-scroll cases passed. The new history case passed disclosure/static status, keyboard and Open/back, then failed waiting for later delivery (both results remained pending). The seed harness's `ensureSeededTurn` creates an open turn for an idle parent, and `ErrandParentDeliverer.NotificationBlocked` correctly blocks notification behind that turn. `seedTimedOutAwait` now sets `newTurn`, `turnStartedAt` and `turnCompletedAt`; no assertions were removed or waits increased. Its exact-file ESLint passed (`/tmp/errands-option-c-lint-fixture.log`). Attempt-1 screenshots and context: `/tmp/errands-option-c-artifacts/desktop-attempt-1/`.

Desktop final: the identical command with a fresh managed production build passed all 3 cases, without retries (44.3s), `/tmp/kandev-run.e2e.bq6h07fr.log`. This proves real await/result-returned/reload behavior, semantic keyboard disclosure and Open/back, later live completion plus successful delivery without changing the saved pending/running statuses or metadata, and reader-scroll preservation. Screenshots are retained at `/tmp/errands-option-c-artifacts/desktop/errands-await-desktop-{collapsed,expanded}.png`; the presentation was inspected.

Phone final: `../../scripts/run-quiet e2e --summary -- pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --grep 'phone await|phone completed await|phone retains start' --retries=0` ran serially after desktop with another fresh managed production build, passing all 3 native Pixel 5 cases without retries (45.2s), `/tmp/kandev-run.e2e.V32Mgmat.log`. Coverage includes actual wait duration and returned-result details after reload; 44px disclosure/Open controls, center hit testing, row containment, native tap/Open/back, later live completion, immutable historical statuses/metadata after reload, a single visible transcript, no document horizontal overflow and preserved reader scrolling. Both phone screenshots at `/tmp/errands-option-c-artifacts/phone/errands-await-phone-{collapsed,expanded}.png` were inspected: the disclosure remains quiet, readable and contained with clear historical details and reachable Open controls.

Task27's gates are complete. These focused mock-provider runs do not replace Task24's final integrated validation or Task16's requested dark media. The new helper and this repair-package work order remain untracked and must be retained when the primary later stages the authorized changes.

Incremental recovery report: `/tmp/errands-option-c-progress.md`. No delegation, commits, push, publication, live-instance mutation or edits to the primary-owned manifest.
