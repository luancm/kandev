# Errand recovery browser admission handoff

Scope: test-only browser acceptance for unresolved Errand recovery refusal and explicit continuation after an exact interrupted settlement, on `feature/fix-stranded-errands-mo6` based on integrated `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. The owned test work is complete and ownership is released to the primary session. Production code, primary plan/manifest, terminal identity remediation, result-duration coverage, commit, and push are outside this checkpoint.

## Acceptance evidence

The browser fixtures start a real Errand through the isolated E2E backend, wait for the child prompt to reach the traced mock ACP provider, and inspect the resulting SQLite rows. Admission evidence binds task, child session, queue incarnation, execution, native session, harness generation, stream, submission, turn, payload hash, and the persisted prompt-admission receipt; the provider trace must contain the original prompt exactly once.

For refusal, the fixture abandons the exact admitted turn, stores an identity-bound uncertain delivery recovery record, and removes only the generic runtime recovery block. Errand attention then comes from the still-missing exact Errand settlement. The fixture does not seed `attention_reason` and does not create a successful result. After reload, the browser obtains the backend list projection, opens the child transcript, and submits two drafts through the real composer and WebSocket path. Both receive the stable `errand_recovery_unresolved` conflict; message count, turn count, conversation revision, and provider prompt trace remain unchanged, while each draft stays in the editor and the submit control returns to enabled.

For continuation, the browser opens the real child session and uses its Stop control, then waits for the session to become idle. The test fixture uses the Errand domain settlement service with the exact observed turn, submission, incarnation, and generation to record an interrupted result and one `parent:interrupted` settlement receipt. It asserts that the receipt resolves attention while the original submission hash, native session, and harness generation remain unchanged. The browser then submits a new explicit instruction through the real composer; the ACP trace contains the original prompt once and the new instruction once, with no replay of the original. The new prompt receives its own turn and submission, and the workflow step is unchanged.

The backend fixture is compiled into the existing `e2e-send-now-fixture` binary and skips unless the managed runner supplies `KANDEV_E2E_ERRAND_RECOVERY_ROOT`. It requires an absolute `kandev-e2e-*` directory under the system temporary directory, rejects roots under `~/.kandev`, and requires the ACP trace to remain under that isolated root. The browser helper invokes only `TestErrandRecoveryBrowserFixture` in that binary with `-test.timeout=2m`; global setup includes the new fixture source in its binary freshness inventory.

## Verification

Required artifacts were built sequentially under `/tmp/kandev-errands-repair-build.lock` before browser execution: `make build-backend`, `make build-e2e-delivery-fixture`, `make build-e2e-send-now-fixture`, `make build-e2e-plugin-package`, and `corepack pnpm@9.15.9 --dir apps/web run build:e2e`. Each completed successfully. The handles for these artifact builds and the earlier focused Go lint, frontend typecheck, and component test are in the pre-compaction verification session and were not retained in this resumed context; their exact commands and outcomes are recorded here rather than assigning new handle numbers.

`bash scripts/lint-go-changed apps/backend/internal/orchestrator/e2e_errand_recovery_fixture_test.go` completed with `0 issues`. `cd apps/web && pnpm run typecheck` passed. Focused `oxlint --config oxlint.config.mjs` and `prettier --check` passed for the owned TypeScript changes. `cd apps/web && pnpm exec vitest run components/task/passthrough-chat-composer.test.ts` passed all 17 tests; it exercises the real `useSendPassthroughMessage` hook with a rejected request and verifies localized feedback, retained draft/context, cleared busy state, and no success cleanup.

Handle `21907`: after the mobile selector correction, focused Prettier and Oxlint passed, then the managed phone suite with repository global setup enabled passed both `phone real Errand recovery refusal preserves the admitted conversation` and `phone explicit Errand continuation follows an exact interrupted receipt` (2 passed, 27.4s). Command, run under the shared lock: `cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts --grep "phone real Errand recovery refusal|phone explicit Errand continuation"`.

Handle `20473`: the managed desktop suite with repository global setup enabled passed both `desktop real Errand recovery refusal preserves the admitted conversation` and `desktop explicit Errand continuation follows an exact interrupted receipt` (2 passed, 20.0s). Command, run under the shared lock: `cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --shards 1 --project chromium e2e/tests/session/errands.spec.ts --grep "desktop real Errand recovery refusal|desktop explicit Errand continuation"`.

The first phone run timed out because the shared browser helper searched for the desktop-only `errand-open-*` control inside the phone drawer. The phone surface exposes each full-width row as `errands-panel-row-*`; the helper now taps that real row, matching the existing phone interaction pattern. Both phone cases passed on the next run, so this was a test selector mismatch rather than a production failure. The backend source and UI production source remained frozen throughout this checkpoint.

## Screenshots and limits

Reviewed screenshots are preserved outside the worktree under `/tmp/kandev-errand-browser-admission/reviewed/`: desktop recovery row, transcript, and continuation; phone recovery row, transcript, and continuation. The Playwright run also leaves its latest screenshots in `apps/web/e2e/test-results/` until the next managed run clears that directory. The in-test ACP trace assertions are authoritative for provider activity; the isolated trace files are removed by fixture teardown and are not retained as standalone artifacts.

The provider boundary is the E2E mock ACP agent, and all browser data and processes belong to the managed temporary fixture root. These results prove the browser admission behavior and receipt identity checks in that isolated setup; they do not repair or inspect a live incident, prove behavior of an external provider, or claim whole-task completion. No socket interception, browser request interception, or scratch global-setup bypass was used for these acceptance cases.

## Exact write set

```text
apps/backend/internal/orchestrator/e2e_errand_recovery_fixture_test.go
apps/web/components/task/passthrough-chat-composer.test.ts
apps/web/e2e/global-setup.ts
apps/web/e2e/helpers/errand-recovery.ts
apps/web/e2e/helpers/errands.ts
apps/web/e2e/tests/session/errands.spec.ts
apps/web/e2e/tests/session/mobile-errands.spec.ts
docs/plans/errand-orphan-settlement/browser-admission-handoff.md
```

`errands.ts` receives only an additive optional traced-profile seam, required to direct the isolated mock ACP provider's actual prompt activity to the per-test trace file. No shared-state Git operation, production-code edit, live database/runtime operation, commit, or push was performed. Ownership of these files is released to the primary session for independent integration and review.
