# Panel-controls implementation handoff

Parent update: this is a historical worker checkpoint. Final review remediation and acceptance passed 19/19 desktop and 16/16 phone cases; see [final integration handoff](final-integration-handoff.md).


## Scope and changed paths

The implementation provides truthful selected-parent attention, running and queued counts with a wrapping narrow trigger; individual Stop controls for live rows; isolated pending and failure feedback; a translated pending progress announcement with a stable Stop name; and desktop and phone regression coverage. It reuses the existing cancellation action and preserves transcript, task-wide Stop-all, live-status and capacity behavior.

- UI and unit coverage: `apps/web/components/task/chat/errands/errands-chip.tsx`, `errands-chip.test.tsx`, `errands-panel-row.tsx`, and `errands-panel-row.test.tsx`.
- Browser coverage: `apps/web/e2e/tests/session/errands.spec.ts` and `mobile-errands.spec.ts`.
- Localization: `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- Public behavior guidance and results: `docs/public/coordination.md` and `task-01-panel-controls.md`.

## TDD and checks

- RED: `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx'` reported 20 passed and 10 failed. It reproduced the chip counting an attention errand as running and live rows lacking an individually named Stop. An initial row selector mismatch was corrected before implementation.
- GREEN: `flock -x /tmp/kandev-errands-repair-build.lock sh -c 'cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx hooks/domains/session/use-errand-actions.test.ts'` passed 3 files and 34 tests, including selected-parent counts, live statuses, isolated cancellation, pending/error feedback, settled-row behavior and phone sibling buttons.
- Typecheck, full web lint, `i18n:check`, `i18n:ratchet`, production `make build-web`, 63 public-doc validator tests, validation of 55 published docs pages, and `git diff --check` passed. The web build exited 0 with existing Vite warnings; exact commands and counts are in the work-order Results.
- Reviewer remediation RED: the focused row suite reported 6 passed and 1 failed because pending cancellation had no polite status region containing `Stopping Review acceptance`; the first role query was narrowed to avoid matching the existing visible row status.
- Reviewer remediation GREEN: the focused three-file Errands suite passed 34/34. Web typecheck, full lint, `i18n:check`, `i18n:ratchet`, and `e2e:sleep-ratchet` passed; Traditional Chinese conversion and pseudo generation completed, with unrelated generated catalog changes restored.
- Source snapshot base remains `027432d2e046ea58811995c6c529fb121ebdc831`. The current binary diff for the owned tracked frontend, locale and public-guidance files hashes to `25ce813f7e7159917a81b513dab961e3db7a5a6374c9140951c4dc6ebcacf1fa`; `errands-panel-row.test.tsx` hashes to `5bbb53bdf547dbc79f895530eec90fc8260e46f5f7fdfff263ae277a59acb647`.

## Desktop and phone acceptance

Real desktop and phone E2E has not run. Therefore no browser result or screenshot exists to hash or visually inspect. After source freeze, run `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errands.spec.ts`, then run `pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errands.spec.ts`. The desktop test covers Stop of one of two live errands, authoritative status/count refresh, sibling isolation, transcript-message retention, Open/Back, and a rendered 28px Stop target. The phone test covers a 44px Stop target beside Open, retained transcript and live sibling, updated counts, Open/Back, and narrow fine-pointer containment. Its mixed-count case now verifies visible wrapping and the full accessible label at 320px, keeps 320/393/767px drawer geometry, and checks popover behavior at 768px.

The backend cleanup owner still owns uncommitted backend source; those paths are outside this Errand's ownership. Integrated E2E is intentionally pending the cleanup owner's explicit source freeze. Do not treat an E2E run against the changing backend as acceptance. After freeze, run the desktop command first and phone command second from the work order, inspect both screenshots, calculate their SHA-256 hashes, and append the commands, pass counts, screenshot paths and hashes here and to Results.

## Limits and handoff

The current worktree includes the separate cleanup-owned backend changes and the preserved untracked loading-performance note; none is included in this frontend handoff. At this remediation check, disk had 12 GiB free. Recheck the backend changed-path manifest and source hashes after the cleanup owner explicitly freezes source and before browser acceptance; record that frozen manifest with the E2E evidence.

The frontend implementation and local checks are ready for parent review. The work order remains `in_progress` until the frozen-source desktop and phone E2E runs, rendered screenshot inspection, hashes and combined acceptance review are complete.
