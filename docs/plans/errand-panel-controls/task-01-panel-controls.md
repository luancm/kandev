---
id: "01-panel-controls"
title: "Truthful activity counts and individual Stop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-010.9
  - AC-TASKS-ERRANDS-010.12
  - AC-TASKS-ERRANDS-011.8
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
---

# Task 01: Truthful activity counts and individual Stop

## Summary and scope

Implement the parent chip status breakdown and individual Stop from the panel through existing cancellation. Own the affected components, regression tests, all locale additions and existing public coordination guidance. Keep cancellation, capacity, recovery and transcript retention semantics intact. Permanent deletion and backend recovery changes are excluded.

## Acceptance

- Chip labels count only actual running errands as running and expose nonzero queued and attention counts for the selected parent; mixed and settled inventories behave correctly.
- Every live row offers direct Stop, including queued and recovery rows, with isolated pending/error behavior and authoritative update handling; other errands and navigation remain untouched.
- Desktop and phone tests demonstrate cancellation and retained transcript; phone controls are sibling targets of at least 44px and remain contained and keyboard accessible.

## ASCII UI preview

UI-01: Parent status bar and desktop popover, recovery plus running plus queued work.

```text
Before: Errands: 3 running
After:  Errands: 1 needs attention, 1 running, 1 queued
+--------------------------------------------+
| Errands                           106 total|
| Review browser acceptance                  |
| Recovery needs verification                |
| Earlier work and effects remain unknown.   |
|                              [Open] [Stop] |
| Completed history (103) >                  |
|             [Stop all errands]             |
+--------------------------------------------+
```

UI-02: Phone bottom drawer opened from the same parent chip.

```text
+----------------------------------+
| Errands                     Close|
| [Review acceptance           >]  |
| Recovery needs verification      |
| Earlier effects remain unknown.  |
| [Open conversation]    [Stop]    |
| Completed history (103) >        |
|----------------------------------|
|        Stop all errands          |
+----------------------------------+
```

The summary and Open may share one navigation button; Stop is always a sibling control. UI-02 illustrates hierarchy, not duplicate required Open buttons. List content scrolls inside the existing drawer; the Stop-all footer is fixed. Narrow layouts can wrap count fragments without dropping information. Every phone action is at least 44px; desktop actions retain 28px density. Pending Stop shows progress and is disabled only for that target; failure leaves the row and warning visible with a localized toast. Settled rows retain Open and omit Stop. These structures satisfy AC-TASKS-ERRANDS-010.9, 010.12 and 011.8; spacing and wording are illustrative and must use locale keys.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

First add regressions and demonstrate failure on the existing implementation. Run these commands from the repository root after implementing:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx hooks/domains/session/use-errand-actions.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errands.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errands.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/errands/errands-chip.tsx` and `errands-chip.test.tsx`
- `apps/web/components/task/chat/errands/errands-panel-row.tsx` and new `errands-panel-row.test.tsx`
- `apps/web/hooks/domains/session/use-errand-actions.test.ts` (reuse existing transport behavior)
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/session/errands.spec.ts` and `mobile-errands.spec.ts`
- `docs/public/coordination.md` (check sessions-and-review and agent-communication for stale instructions)

## Dependencies and inputs

None. Read the linked requirement/design and accepted recovery-settlement ADR. The plan documents the source trace and companion-package ownership. Use the existing header Stop handling, Errands drawer and mobile picker sheet patterns. Run `pnpm run i18n:zh-hant` from `apps/web` for Traditional Chinese additions.

## Risks

Mixed count width, propagation from Stop to Open, and optimistic state that masks cancellation failure. Do not change `isLiveErrand` to fix wording.

## Parallelism

The user explicitly authorized a parent-created implementation Errand after reviewing this package. One frontend writer may work in parallel with backend cleanup Errand `18d429d6-327a-4dc1-9ad2-89dcd6e4e4e7`; file ownership is disjoint. All heavy builds, lint/typecheck and browser checks share `/tmp/kandev-errands-repair-build.lock`. No recursive delegation, backend edits, live runtime changes or worker Git mutations. Parent owns combined acceptance and delivery.

## Results

Implementation and frontend checks are complete; final integrated desktop and phone acceptance remains pending the backend cleanup source freeze. No code was changed in backend-owned files.

### TDD and focused checks

- RED: `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx'` first ran against the existing implementation and reported 20 passed, 10 failed. It showed an attention plus running inventory labelled as two running and live rows without a named individual Stop. One initial row-role query used an incorrect accessible-name selector; the selector was corrected before production changes, and the product regressions remained.
- GREEN: `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx hooks/domains/session/use-errand-actions.test.ts'` passed 3 files and 34 tests.
- `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck'` passed, including rich-output catalog validation and TSRS.
- `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && pnpm run lint'` passed on the final files. The first full lint attempt identified two local issues (function length and a duplicate test selector literal); both were corrected and the rerun passed.
- `pnpm run i18n:check` passed all catalog, plural, translation-scope, em-dash and non-JSX copy checks. The catalog validator reports all six translated shipped catalogs complete and pseudo in sync. `pnpm run i18n:ratchet` passed for 268 added and 478 modified files, with 644 guard entries intact.
- `NODE_OPTIONS=--max-old-space-size=4096 make build-web` passed. Vite emitted existing chunk-size, deprecated-option and ineffective-dynamic-import warnings; the build exited 0.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests; `node scripts/validate-public-docs.mjs` validated 55 published pages. `git diff --check` passed.

### Accepted reviewer remediation

- The chip count label now wraps at narrow widths without truncation, and the phone trigger can grow beyond its 44px minimum when the label spans multiple lines. The complete status breakdown remains in the trigger's accessible name.
- Individual Stop is 28px on fine-pointer desktop and retains the shared 44px minimum on phone and coarse-pointer layouts. Pending cancellation now updates a persistent translated polite status region outside the disabled button; the button keeps its existing stable `Stop {{title}}` accessible name.
- The narrow fine-pointer browser case now seeds three running errands, routes attention and queued snapshots through the browser's `errand.updated` handler, and asserts the complete mixed count label, actual wrapping at 320px, trigger height, 44px row targets, and 320/393/767/768px containment. Desktop Stop coverage now measures the button at 28px.
- Corrected RED: `flock -w 300 -x /tmp/kandev-errands-repair-build.lock /bin/bash -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-panel-row.test.tsx'` reported 6 passed and 1 failed because the pending progress message had no polite status region. The first query selected the row's existing visible status label; the assertion was narrowed to the translated `Stopping Review acceptance` region before the corrected RED run.
- GREEN: `flock -w 300 -x /tmp/kandev-errands-repair-build.lock /bin/bash -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx hooks/domains/session/use-errand-actions.test.ts'` passed 3 files and 34 tests.
- The locked remediation run also passed web typecheck, full web lint, `i18n:check`, `i18n:ratchet` (268 added and 478 modified files, 644 guard entries), and `e2e:sleep-ratchet` (207 added and 196 modified E2E files). Prettier reported all remediation source and locale files unchanged. `i18n:zh-hant` and pseudo generation completed; unrelated generated catalogs were restored, leaving only the Errands task catalogs changed.
- Source snapshot base: `027432d2e046ea58811995c6c529fb121ebdc831`. The current binary diff for owned tracked frontend, locale, and public-guidance files hashes to `25ce813f7e7159917a81b513dab961e3db7a5a6374c9140951c4dc6ebcacf1fa`; `errands-panel-row.test.tsx` hashes to `5bbb53bdf547dbc79f895530eec90fc8260e46f5f7fdfff263ae277a59acb647`.

### Browser acceptance status

Desktop `errands.spec.ts` and phone `mobile-errands.spec.ts` acceptance have not run, and no desktop or phone screenshots have been produced or hashed. The backend cleanup Errand `18d429d6-327a-4dc1-9ad2-89dcd6e4e4e7` still owns modified backend files and has not explicitly frozen source. Do not treat browser results against that changing source as integrated acceptance. Run the exact desktop and phone commands in Verification sequentially after the backend owner explicitly freezes source; inspect and hash the resulting screenshots and update this section and `implementation-handoff.md`.

The production web build passed before this remediation; the cleanup-owned backend build and both browser runs remain pending. The work order stays `in_progress` until rendered cancellation, sibling isolation, retained transcript, count refresh, mixed-count wrapping, 28px desktop Stop geometry, and phone hitbox evidence are recorded and reviewed by the parent.

## Final parent acceptance

Completed after backend source freeze and review remediation. The two initial phone failures were corrected in test code: await finite drawer animations before geometry reads, and exclude action wrappers from row-order locators. Final fresh managed desktop acceptance passed 19/19 and phone acceptance passed 16/16, with zero failures or skips. Parent inspected desktop Stop and narrow phone screenshots; all 22 source hashes remained stable. The backend compatibility suite passed 77 race tests and scoped Go lint reported zero issues. Earlier 34-test frontend/typecheck/lint/locale receipts remain applicable to unchanged production UI. The preceding `in_progress` statements describe historical worker checkpoints; this final section supersedes them. Exact commands, source hashes, limits and retained artifacts are in [the final integration handoff](final-integration-handoff.md).
