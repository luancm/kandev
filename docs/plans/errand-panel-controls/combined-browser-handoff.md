# Combined browser verification handoff

Parent update: this is a historical worker checkpoint. Final review remediation and acceptance passed 19/19 desktop and 16/16 phone cases; see [final integration handoff](final-integration-handoff.md).


## Result

This verification used the released panel-controls and terminal-cleanup source snapshot with normal managed E2E builds and global setup. Desktop passed all 19 discovered tests. Phone ran all 16 discovered tests, with 14 passing and two failing. Combined browser acceptance remains incomplete until the phone geometry sampling and ordering-selector failures below are resolved and rerun.

The verifier made no production or test-source edits. The only repository file added by this errand is this handoff. No live instance or database was used; both managed runs used isolated host fixtures.

## Commands and receipts

Bare `pnpm` in this shell resolved through Corepack to 12.18.0 and rejected the repository pin. Commands below explicitly invoked the declared `pnpm@9.15.9`; dependencies were already installed, and no install or package changes were made.

Discovery confirmed 19 desktop tests and 16 phone tests before execution:

```sh
corepack pnpm@9.15.9 --dir apps/web exec playwright test --config e2e/playwright.config.ts --project=chromium tests/session/errands.spec.ts --list
corepack pnpm@9.15.9 --dir apps/web exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome tests/session/mobile-errands.spec.ts --list
```

The managed desktop run rebuilt the backend, E2E fixtures, production Vite assets, and fixture plugin, then ran Chromium with one worker and a fresh output directory:

```sh
flock -w 300 -x /tmp/kandev-errands-repair-build.lock env KANDEV_RUN_QUIET_DIR=/tmp/kandev-errands-combined-browser-af43466b/logs scripts/run-quiet e2e -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project=chromium tests/session/errands.spec.ts -- --output=/tmp/kandev-errands-combined-browser-af43466b/desktop-results
```

Result: 19 passed, zero failed or skipped, in 4.0 minutes. Full log: `/tmp/kandev-errands-combined-browser-af43466b/logs/kandev-run.e2e.SdbRQ63u.log` (SHA-256 `973b467740053204c6635425d30c66cf971cf74445c7d1f0d60f6e6df5c5d006`).

The managed phone run rebuilt the same artifacts and fixture using the Pixel 5 `mobile-chrome` project, one worker, and a separate fresh output directory:

```sh
flock -w 300 -x /tmp/kandev-errands-repair-build.lock env KANDEV_RUN_QUIET_DIR=/tmp/kandev-errands-combined-browser-af43466b/logs scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project=mobile-chrome tests/session/mobile-errands.spec.ts -- --output=/tmp/kandev-errands-combined-browser-af43466b/phone-results
```

Result: 14 passed and two failed, zero skipped, in 3.6 minutes. Full log: `/tmp/kandev-errands-combined-browser-af43466b/logs/kandev-run.e2e.eTMdWyXB.log` (SHA-256 `2d7958d663caf2fcbb597d2198083503411b00ab937977230ee06c126a44f237`).

The runner used host mode and fresh builds for both runs; neither used `--no-build`. The existing `apps/web/e2e/test-results` directory was not cleared or used as output.

## Acceptance findings

| Behavior | Evidence |
| --- | --- |
| Individual Stop cancels only its selected errand, refreshes the live count, retains transcript, leaves its sibling running, and permits Open/Back | Passed in desktop and phone Stop tests. |
| Desktop Stop target is 28px | Passed the rendered 27–29px bounding-box assertion. |
| Phone Stop and Open targets are at least 44px | Passed the natural Pixel 5 drawer test. The narrow fine-pointer case also passed its 320px row and Stop target checks before failing a later drawer geometry assertion. |
| Mixed attention/running/queued label remains complete, accessible, and wraps at 320px | The narrow fine-pointer test passed exact text and `aria-label`, wrapping, no-ellipsis, and trigger-height assertions before the geometry failure. It did not reach its later 393, 767, or 768px iterations. |
| Parent-scoped ordering through settlement and switching | Desktop ordering test passed. The phone ordering test failed its row-count helper before comparing order, so phone ordering remains unverified. |
| Pending accessible cancellation progress | Not asserted by these browser specs. The frozen `errands-panel-row.test.tsx` contains a pending cancellation test that checks only the selected Stop is disabled and a translated `role="status"` with `aria-live="polite"` announces `Stopping Review acceptance`; the implementation handoff reports the focused three-file suite passed 34/34. This verifier did not rerun that completed unit suite. See [implementation-handoff.md](implementation-handoff.md). |

## Phone failures and proposed fixes

1. `mobile-errands.spec.ts:692`, narrow fine-pointer layout: after opening the drawer at 320px, the containment assertion at line 789 expected `drawerBox.y + drawerBox.height <= 851` but measured `1293.5533447265625`. The failure screenshot is 320×850 and shows the settled drawer visually within the viewport, while the test reads geometry immediately after `toBeVisible()`. This is consistent with sampling Vaul's entrance animation before it finishes. The smallest test fix is to use the existing finite-animation helper before reading geometry:

   ```ts
   import { waitForFiniteAnimations } from "../../helpers/animations";

   await expect(drawer).toBeVisible();
   await waitForFiniteAnimations(drawer);
   ```

   If the settled measurement still exceeds the viewport, inspect the drawer's actual scroll and height constraints before accepting the UI. No production source was changed by this verifier.

2. `mobile-errands.spec.ts:1152` phone ordering case: `apps/web/e2e/helpers/errands.ts:590` expects three rows but receives six because `[data-testid^="errands-panel-row-"]` matches both each phone row's `errands-panel-row-actions-*` wrapper and its inner Open button. The equality/order assertion is never reached. The smallest helper fix is to exclude the wrapper from the shared row locator:

   ```ts
   const rows = surface.locator(
     '[data-testid^="errands-panel-row-"]:not([data-testid^="errands-panel-row-actions-"])',
   );
   ```

   The current row markup confirms one wrapper and one matching Open target per phone row. This is a browser-test selector failure, not evidence of incorrect product ordering.

## Source freeze

The before and after manifests contain the same 21 backend, UI, locale, and E2E source paths. `sha256sum --check` passed for all paths after desktop and again after phone; `cmp` confirmed the manifests are identical. The SHA-256 of each manifest file is `c2a3a4a2cba46469a100cb649cd8303ce3a6d528c1b11bfb2880fce5752b7627`. Full before and after files are retained at `/tmp/kandev-errands-combined-browser-af43466b/source-before.sha256` and `/tmp/kandev-errands-combined-browser-af43466b/source-after.sha256`.

The unchanged test helper cited in the second failure, `apps/web/e2e/helpers/errands.ts`, was clean in the initial and final worktree status; its current SHA-256 matches `HEAD` at `047623e3c1385fda3909d735283c2cd9b916833d1f349be01e98508974e00b14`.

```text
bd6cc151131dc7c8ee54c680b89bcd0073dbbf7e0e05ec6f5e3d1b65c85556d1  apps/backend/internal/errands/service.go
a07869e0068b7e537da5668774d3385a1e61f90e9e814606e2a4195df99a67b0  apps/backend/internal/errands/teardown.go
00a2db0a2b3669b9560faecf738348398429bbdb98dfe94a1ce3c2d0037f4e48  apps/backend/internal/errands/teardown_compatibility_test.go
dc83d4ee451b9807b3cec844b3e8ec1f03bb6f1d2caba1a6499c2523b3c0d0bf  apps/backend/internal/orchestrator/errand_completion_cleanup.go
baf6beaab49c9acc4ad96ffbb6c9a1fc76b0f40f2b4ae49dd889e938bebe1538  apps/backend/internal/orchestrator/errand_teardown.go
6f6f2e4d6db205deffbe9d2520638c0adc2867d77814181f3a35c337d38cca97  apps/backend/internal/orchestrator/errand_teardown_integration_test.go
875d7e0dc9c8095c77e6b8baf9b70638d20c72d0e7d65fab89e84d6413e6be9c  apps/backend/internal/orchestrator/errand_terminal_compatibility_test.go
8e80e1855d9834972c399482974f73491c30252a0f373d0c92466b07a671760c  apps/web/components/task/chat/errands/errands-chip.test.tsx
000385d2d045389bb0c439665dc8129c41ad38a40fc54fbde3303837f84fb496  apps/web/components/task/chat/errands/errands-chip.tsx
5bbb53bdf547dbc79f895530eec90fc8260e46f5f7fdfff263ae277a59acb647  apps/web/components/task/chat/errands/errands-panel-row.test.tsx
77cb9b03861581a83785daf54972404b6da31a806cad23f97c9875bf43cc71a3  apps/web/components/task/chat/errands/errands-panel-row.tsx
a63297dbf8ffcdeccb585d979f7e39ecb4c7eeb76d3017f32c8c8c08c5d4857e  apps/web/e2e/tests/session/errands.spec.ts
7b1f6198e0adb156f30f1f6a77f622b307c70a43ca249a58ad52a934e7512762  apps/web/e2e/tests/session/mobile-errands.spec.ts
a7b046f023c7cb8e37e0c08923839ae4b23dd0ed5046fdc9cab3328f787acdae  apps/web/src/locales/en/task.json
61b84174f916966cd1730aaefff9ec32bd19551139d72da4fc955cc24b705e1f  apps/web/src/locales/ja/task.json
eb2e6bf967deb8e351535b35e19a8d9a88c180a8326d12db14f912b773ce1e45  apps/web/src/locales/ko/task.json
b1d56f5260a9ea3bd05b4f16ddd43392dd8687a8da499bdeb7b9d171bd31f96a  apps/web/src/locales/pseudo/task.json
496e0233218e7aabee29f2aff5d98b36771f6f98a6142de545fdcbaf4068cd44  apps/web/src/locales/pt-pt/task.json
b549d145f85c4396343b0596ebc68015ad3419080b05744fb6445790de2b515b  apps/web/src/locales/zh-cn/task.json
5107685ba7c0a0e575877d450db8ae2a92d8a99572517052fe19b645b51db636  apps/web/src/locales/zh-hk/task.json
62ca4bbaca39466c36bda0a1fdd6fc7870ea922ba236ba4bfbe7ec95daaa4d5d  apps/web/src/locales/zh-tw/task.json
```

## Screenshots and cleanup

The desktop run produced 13 PNGs and the phone run produced 19. Every PNG was hashed; screenshot manifests are `/tmp/kandev-errands-combined-browser-af43466b/desktop-screenshots.sha256` (manifest SHA-256 `4ea26bb187af52ae5b41afdd81a55b2179523964f2a5db30c74912a67b393cac`) and `/tmp/kandev-errands-combined-browser-af43466b/phone-screenshots.sha256` (manifest SHA-256 `8a1fdea6c0adfb543a58eff3617a35cb01057abafb044f746aae092e70df4a15`).

Inspected screenshots: desktop `errands-desktop-panel-stop.png` (SHA-256 `9e806550018d5277e5e8e3482a37b72a8bb8e8fc5bc86903079cce89e93ee007`), phone `errands-phone-drawer.png` (`cf724bae7c5f9618766787c347de0d1766f153ec7b1ec1e4776eb2ebfde2e1c6`), narrow failure `test-failed-2.png` (`d8a382df5641b4d297daf11409eb3de779a84e1995f6aa883bbe20535e218d75`), and the ordering failure screenshot `test-failed-1.png` (`6b4dd84008e67c09bee0b080390b538d5569f64594c1a6757bd418513875de45`). The phone drawer capture has an unrelated update toast over the header; the row actions and Stop-all footer remain visible.

Both E2E processes and fixture backends exited; no run process remained and a non-blocking lock check succeeded. Per-run fixture temp directories were removed by managed teardown. At completion, 14 GiB memory and 38 GiB disk were available. The 2.4 MiB pre-existing E2E result directory was left untouched. Logs, screenshots, and hash manifests remain in `/tmp/kandev-errands-combined-browser-af43466b` for parent review.

These results certify only the listed frozen snapshot. The parent's accepted backend diagnostic P2 remediation was intentionally scheduled after this browser snapshot and must be assessed for any affected acceptance refresh.
