# Errand recovery frontend checkpoint handoff

Scope: frontend AC-TASKS-ERRANDS-002.4–.8 on `feature/fix-stranded-errands-mo6`, based on `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. The frontend package and focused verification are complete; ownership is released to the primary session. Backend source, primary plan/manifest, terminal identity repair, duration work, commit, and push remain outside this checkpoint.

## Behavior

Errand DTO/API and store normalization accept optional `attention_reason` values `permission` and `recovery_unresolved`; unknown and absent values remain compatible, and a full snapshot without a reason removes an older projected reason. List and event updates preserve the same shape.

Recovery attention uses its own warning label and explanation in the existing desktop popover row, phone drawer row, and opened child header. It does not use permission wording. The copy says that earlier work and possible external effects are unknown, offers conversation inspection or stopping the errand, and states that continuation requires verification. It does not promise that stopping repairs or cancels prior work.

Normal and passthrough composers map a send conflict with `details.reason=errand_recovery_unresolved` to localized recovery guidance. The draft remains in the editor and the composer returns to its non-busy state; unrelated error handling is preserved.

The desktop browser case opens the recovery row, checks the child transcript and explanation, then attempts a send and asserts that the draft remains, the submit control is enabled, and message/turn counts do not change. The phone case exercises the drawer, full-width row, child header, Back navigation, touch targets, and horizontal overflow in addition to the refusal behavior. Existing Open, Stop, and Back surfaces remain in place.

## Verification

The frontend implementation was already present at resume. No recoverable pre-change failing-test log or other RED evidence was available, so this handoff does not claim a frontend RED run. The focused unit/component suite was rerun against the resumed source and passed.

`pnpm run i18n:zh-hant` generated the Traditional Chinese catalogs. Final `pnpm run i18n:check` passed across English, pseudo, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, and `ko`; its 608 orphan-key warning is the existing catalog warning. Final `pnpm run i18n:ratchet` passed with 154 added and 352 modified files clean and the 644-entry guard allowlist intact.

The first full frontend lint run found an `i18next/no-literal-string` violation on the existing entity quote literals in `errand-renderers.tsx`. The narrow adjacent fix replaced the quote entities with semantic `<q>` markup. The final full lint passed.

The fresh Vite E2E production bundle was built with `pnpm run build:e2e` before browser runs and is recorded in `/tmp/kandev-errand-frontend-recovery/web-build.log` (exit 0, Vite build 13.84s). The backend E2E binary was checked before the run; no backend source newer than `apps/backend/bin/kandev` was found. The managed runner used `--no-build` and a scratch Playwright config based on `e2e/playwright.config.ts` with only the output directory and test directory redirected and repository `globalSetup` disabled. The default global setup stopped before test execution because the unrelated `apps/backend/bin/e2e-send-now-fixture` and plugin fixture package were absent. Existing worker fixtures still launched an isolated backend and test page; the tests did not use the live application, database, or runtime.

The first browser run failed before UI assertions because `startErrands` waited for the parent session to become idle even though the child result had been delivered; the parent remained `RUNNING`. The helper now has an opt-out used only by these child-only UI cases. It still waits for delivered result state, and all other callers retain the parent-idle wait by default. A following run exposed a strict locator match against both the recovery refusal and the unrelated update toast; the selector now targets the recovery text. The tests also wait for the unrelated update toast to expire before capturing screenshots.

Final checks and handles:

- Handle `61954`: focused Vitest suite passed, 7 files and 124 tests (`lib/types/errands.test.ts`, `lib/api/domains/errand-api.test.ts`, `lib/state/slices/errands/errands-slice.test.ts`, `lib/ws/handlers/errands.test.ts`, `lib/chat/message-send-error.test.ts`, `errand-header.test.tsx`, and `errands-chip.test.tsx`).
- Handle `34583`: `pnpm run typecheck` passed.
- Handle `21054`: `pnpm run lint` passed.
- Handle `20055`: `pnpm run i18n:check` passed, including no em dashes and no non-JSX copy findings.
- Handle `40140`: `pnpm run i18n:ratchet` passed.
- Handle `37215`, log `/tmp/kandev-errand-frontend-recovery/kandev-run.e2e.dngq3Qmn.log`: managed Chromium desktop recovery UI case passed, 1 test in 12.7s.
- Handle `52330`, log `/tmp/kandev-errand-frontend-recovery/kandev-run.e2e.8mci0PVn.log`: managed `mobile-chrome` phone recovery UI case passed, 1 test in 14.4s.
- `git diff --check -- apps/web` passed. The new untracked handoff was checked with `git diff --no-index --check /dev/null docs/plans/errand-orphan-settlement/frontend-recovery-handoff.md`; it produced no whitespace errors (exit 1 is expected for a new file).

Reviewed screenshots are preserved outside the worktree at `/tmp/kandev-errand-frontend-recovery/reviewed/desktop-row.png`, `desktop-child.png`, `phone-drawer.png`, and `phone-child.png`. The update toast is gone in these captures; the phone drawer shows the full warning and Stop action, and the phone child shows the recovery explanation with Back.

## Browser coverage boundary

The browser case starts a real isolated backend task and Errand, waits for the mock provider result to reach the parent, and uses the real page and WebSocket handler path. For this UI case, `routeErrandUpdateBrowserSocket` injects the `recovery_unresolved` attention event and intercepts the next `message.add` request to return the stable conflict shape. Therefore the browser case verifies rendering, localized error handling, draft retention, busy reset, and absence of a new message/turn; it does not exercise the backend's recovery classifier or prove provider non-invocation.

No browser test was added for an exact-settled interrupted Errand followed by a supported explicit new prompt. The current E2E `ApiClient` can seed generic task sessions, messages, and turns, but has no fixture route/helper to create an Errand with the exact persisted interruption and settlement receipts required by the continuation gate. The WebSocket helper above only changes browser projection and cannot create that evidence. The minimal missing seam is an isolated test fixture that drives or seeds the real Errand settlement/receipt and recovery state while preserving native conversation identity, followed by real browser send admission through the supported parent path. Do not replace this with a synthetic provider-success event. The backend provider and authenticated-parent continuation acceptance, including unchanged original submission and receipt state, is recorded in `continuation-handoff.md`; that is backend acceptance, not browser proof.

## Exact write set

```text
apps/web/components/task/chat/chat-input-area.tsx
apps/web/components/task/chat/errands/errand-header-parts.tsx
apps/web/components/task/chat/errands/errand-header.test.tsx
apps/web/components/task/chat/errands/errand-header.tsx
apps/web/components/task/chat/errands/errand-open-button.tsx
apps/web/components/task/chat/errands/errand-status-announcer.tsx
apps/web/components/task/chat/errands/errand-status.tsx
apps/web/components/task/chat/errands/errands-chip.test.tsx
apps/web/components/task/chat/errands/errands-chip.tsx
apps/web/components/task/chat/errands/errands-panel-row.tsx
apps/web/components/task/chat/messages/kandev/errand-renderers.tsx
apps/web/components/task/passthrough-chat-composer.tsx
apps/web/e2e/helpers/errands.ts
apps/web/e2e/tests/session/errands.spec.ts
apps/web/e2e/tests/session/mobile-errands.spec.ts
apps/web/lib/api/domains/errand-api.test.ts
apps/web/lib/api/domains/errand-api.ts
apps/web/lib/chat/message-send-error.test.ts
apps/web/lib/chat/message-send-error.ts
apps/web/lib/state/slices/errands/errands-slice.test.ts
apps/web/lib/state/slices/errands/errands-slice.ts
apps/web/lib/types/errands.test.ts
apps/web/lib/types/errands.ts
apps/web/lib/ws/handlers/errands.test.ts
apps/web/src/locales/en/task.json
apps/web/src/locales/ja/task.json
apps/web/src/locales/ko/task.json
apps/web/src/locales/pseudo/task.json
apps/web/src/locales/pt-pt/task.json
apps/web/src/locales/zh-cn/task.json
apps/web/src/locales/zh-hk/task.json
apps/web/src/locales/zh-tw/task.json
docs/plans/errand-orphan-settlement/frontend-recovery-handoff.md
```

All work remains uncommitted in the shared worktree. No backend source, parent-owned plan/manifest, duration test/helper, live service, database, or runtime was changed by this checkpoint. No commit or push was performed. Ownership is released to the primary session for independent review and final integration.
