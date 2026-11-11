# Result duration browser verification handoff

## Scope and current result

This packet owns only `apps/web/e2e/helpers/errand-result-duration.ts`, `apps/web/e2e/tests/session/errand-result-duration.spec.ts`, `apps/web/e2e/tests/session/mobile-errand-result-duration.spec.ts`, and this handoff. No production files, shared Errands helpers, locales, plan/spec files, or Git state were changed here.

The desktop duration scenario passed on a fresh managed one-shard Chromium build: it verified the persisted duration against the errand's creation and settlement timestamps, keyboard disclosure activation, Open/Back and composer use, a completed child follow-up, the unchanged parent delivery snapshot, reload persistence, and no horizontal overflow. That run was `/tmp/kandev-run.e2e.WcN9OV9t.log`; a final `--no-build` rerun against the same artifacts as the fresh phone build also passed 1/1 at `/tmp/kandev-run.e2e.b8TQYumd.log`.

The phone scenario passed on a fresh managed one-shard `mobile-chrome` build after the parent-owned compile issue was fixed. It reported 1 passed in `/tmp/kandev-run.e2e.WON8EQtY.log`; source manifests matched before and after. The inspected 1081x1999 long-duration screenshot is preserved at `/tmp/kandev-duration-phone-verified.png` and has SHA-256 `6d912d9b1d09c574967167183b64b59d6ded5ab06456741871609f4e58b89cb6`. Earlier phone attempts remain non-acceptance evidence: the no-build run's hashes changed, and the next fresh build stopped before Playwright on the temporary pointer/value test error.

## Commands and results

The final desktop command, from the repository root, reused the fresh phone build artifacts:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium e2e/tests/session/errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0
```

The first fresh desktop run exposed a keyboard-focus test assumption; after focusing the native disclosure button and activating it with Space, the fresh managed run passed. An earlier no-build retry was rejected by the managed runner because backend source had moved. The final no-build rerun passed after the fresh phone build, with unchanged before/after source hashes. Its manifest SHA-256 was `6b8b79b14b6eb8582886ba0073c1134b10c44ff8539351ed98a4fbdb4dc3e58b` for both `/tmp/kandev-duration-desktop-same-build-source-before.sha256` and `/tmp/kandev-duration-desktop-same-build-source-after.sha256`; this exactly matches both fresh phone manifests and the current source manifest.

The first native-phone command used the existing browser cache and a no-build runner:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/session/mobile-errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0
```

It reported 1 passed in `/tmp/kandev-run.e2e.uLCbW0ev.log`, but its source manifest changed from SHA-256 `5057b24ae3922598ff50def66b1f4825a78a38e28d9fd86dc9206355706f926e` to `dabae13f711df1066eca2c6b41caba5a1d75b5c37080381b7e12d87fa1750354`; the changed file was parent-owned `apps/backend/internal/task/repository/sqlite/agent_delivery_terminal_identity.go`. The next fresh managed build stopped before Playwright because the pointer/value mismatch at lines 281 and 287 described below; the parent corrected the test type, the focused backend suite passed, and this fresh managed phone command then passed:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0
```

The failed compile is recorded in `/tmp/kandev-run.e2e.0CtmK5Xb.log`; a temporary pointer/value mismatch in a parent-owned Go test was corrected before the successful rerun. The fresh successful phone run used source manifest SHA-256 `6b8b79b14b6eb8582886ba0073c1134b10c44ff8539351ed98a4fbdb4dc3e58b` both before and after, from `/tmp/kandev-duration-phone-verified-source-before.sha256` and `/tmp/kandev-duration-phone-verified-source-after.sha256`.

Scoped checks passed: Prettier check, Oxlint on the three owned TypeScript files, E2E sleep ratchet, and Playwright discovery of exactly one test in `chromium` and one in `mobile-chrome`. Backend and Vitest checks are the implementation worker's accepted evidence in [`duration-handoff.md`](duration-handoff.md); they were not repeated here.

## Rendered phone inspection and mock limits

The inspected phone screenshot at `/tmp/kandev-duration-phone-verified.png` shows the expanded `123h 45m 56s` fixture, Done status, and Open on screen without horizontal overflow; the parent composer remains visible and usable. The browser test separately exercises real taps for disclosure, Open, and Back, and verifies the composer accepts and clears text.

The 123-hour card is seeded history for a long-label fit check; it does not represent a mock run lasting 123 hours. The real mock result row used a positive 1.1-second clock-separation dwell and its recorded duration was compared exactly with `floor((SettledAt - CreatedAt) / 1ms)`. This verifies backend composition and persisted display behavior, not external provider timing precision.

All browser runs used isolated E2E mock data and the already-installed shared Playwright browser at `/tmp/kandev-errand-frontend-recovery/browsers`; no live instance or database was inspected or mutated. An earlier attempt in this packet also installed Playwright's browser in the default user cache before discovering the shared recovery cache; no further browser install was run afterward, and neither cache was removed.

## Verification complete and ownership release

The desktop and phone browser checks are complete on the same current source snapshot. The owned browser packet is ready for parent integration and acceptance.

The browser packet owns only the three new E2E/helper files and this handoff; integration, backend fixes, production changes, commit/push, and task-plan status remain parent-owned. No Git mutation, live operation, or external publication was performed.

## Keyboard reachability reviewer correction (2026-11-11)

Reviewer ec73090f identified that focusing the disclosure directly did not prove Tab reachability. The desktop scenario now starts at the preceding “Errands parent returned” agent message: it reveals that message’s action row, clicks its visible Copy control to establish focus, tabs through Raw, Metadata, and Favorite with a focus assertion at each stop, asserts the finished-result disclosure receives focus, then activates it with Space and checks `aria-expanded` and the existing duration, preview, and Open assertions. The original follow-up, immutability, reload, composer, long-label, and overflow checks remain in the same test.

The final standard managed Chromium run matched one test and passed the full test body, 1/1 in 14.9s. It ran with global setup enabled through the normal host runner and `--no-build`; no focus-only branch, early return, or environment guard was present. Exact invocation under the required shared lock: `flock -w 600 /tmp/kandev-errands-repair-build.lock /tmp/kandev-errand-duration-keyboard-final-run.sh`. The wrapper ran `scripts/run-quiet e2e --summary -- bash -c "cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --shards 1 --project chromium e2e/tests/session/errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0"`.

The run log is `/tmp/kandev-run.e2e.sabJ1ZnL.log`, SHA-256 `352046c0d166cc30770a99be92de76deefee9660e42e8b9f6407afbe6cad4d98`. Source manifest SHA-256 was `2ce115597d3914e0e1735881ebc6b6153dd8a825894bae8997252828dde71cc2` before and after the run across 21,332 backend/web source files; the manifest files are `/tmp/kandev-errands-duration-keyboard-final-run-before.sha256` and `/tmp/kandev-errands-duration-keyboard-final-run-after.sha256`. The spec SHA-256 was `a0e117332898695af10c277d59344f1cfefa361e16b8e3cadce69e6f744def69`.

Before no-build reuse, the current source manifest differed from the fresh managed-build manifest only at `apps/web/e2e/tests/session/errand-result-duration.spec.ts`; no application source changed after the fresh backend and web artifacts were produced. The artifact timestamps were `2026-11-11 13:34:51 UTC` for `apps/backend/bin/kandev` and `2026-11-11 13:38:56 UTC` for `apps/web/dist/index.html`. Exact-file Prettier, exact-file Oxlint, and `pnpm run e2e:sleep-ratchet` passed; the ratchet reported 146 added and 155 modified E2E files clean. The existing phone result above was not rerun, as requested.

This correction changes only `apps/web/e2e/tests/session/errand-result-duration.spec.ts` and this handoff. No production, helper, phone-test, live-instance, or Git state was changed; no commit, push, or publication was performed.
