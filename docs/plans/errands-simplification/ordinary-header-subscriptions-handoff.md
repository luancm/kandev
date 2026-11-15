# Task04 source handoff

The [work order](task-04-ordinary-header-subscriptions.md) implements the approved resource optimization in one production hook. Current-state role, enabled browser row or scoped inspection determines recognition before selecting child-only inputs. One `useShallow` store subscription includes the existing feature boolean. Incomplete recognized children retain detail, and presentation changes do not become ownership-effect dependencies. The public hook/callers are unchanged; private helpers satisfy the existing complexity limit. Production size is 201 → 263 lines.

Source is frozen for parent integration on base HEAD `77b155597c1dab4b16016bc5e5da17ac6a16c632`. Independent read-only review reported no findings and confirmed the four hashes, unchanged production callers, and original owner/effect blocks. Source acceptance is distinct from the pending browser/performance acceptance; work-order status stays `in_progress`.

| Check | Result | Raw log under the evidence root |
|---|---|---|
| Original baseline | 26 passed, 7 stale mobile-fixture failures | `logs/kandev-run.vitest.ix6MaQSO.log` |
| Repaired original mobile cases, before production edits | 7/7, assertions retained | `logs/kandev-run.vitest.XnGfNYmL.log` |
| Resource RED | 4 expected failures, 12 listeners / 12 callbacks above baseline | `logs/kandev-run.vitest.MaRvZ2Nk.log` |
| Final scoped tests | 47/47 | `logs/kandev-run.vitest.rJuu38au.log` |
| Explicit GREEN resource receipt | 4/4, 1 listener / 1 callback above baseline; unmount zero | `logs/kandev-run.vitest.ZN8wnui1.log` |
| Web typecheck | Passed | `logs/kandev-run.typecheck.094y31vL.log` |
| Exact-file lint | Passed | `logs/kandev-run.lint.MUDH7gG4.log` |
| Prettier / whitespace | Passed | `logs/kandev-run.format.bYsK4gXM.log`; `git diff --check` |

Evidence root: `/tmp/kandev-task04-subscriptions.xd_z_4rm/`. `verification-receipts.json` retains the exact executed commands, handles, exit codes, log hashes and intermediate failures; `frozen-source.json` records protected files and byte equality for the inspection-owner function and detail-effect block. All commands used the shared flock and pinned pnpm 9.15.9 wrapper prepended to the existing PATH. The final extra resource-only run disabled Vitest console interception solely to expose the passing counts, which the default reporter suppressed.

The essential RED command was `pnpm --dir apps/web exec vitest run components/task/chat/errands/use-errand-view-model-resource.test.tsx`. Final GREEN was `pnpm --dir apps/web exec vitest run --silent=false components/task/chat/errands/errand-header.test.tsx components/task/chat/errands/use-errand-view-model-live.test.tsx components/task/chat/errands/use-errand-view-model-resource.test.tsx components/task/mobile/mobile-session-picker-row.test.tsx`. Both were wrapped by `flock -x /tmp/kandev-errands-repair-build.lock scripts/run-quiet vitest --` with the recorded environment. No budget or assertion was relaxed.

Both flag states are exercised through the real exported header and phone row; the real picker supplies its existing 15-listener baseline (header baseline 1). New live tests protect late identity, role/browser/inspection recognition, flag-off behavior, task precedence, scope changes, incomplete retention, title updates without retain/release churn, and pending reconnect rejection. The original session-revision, delayed-response and pending-detail/unmount assertions remain. Total added tests: 14; deleted: zero. The old seven mobile cases now use the real store instead of mocked selectors, retaining their existing presentation stub and every assertion.

Frozen SHA256 values:

- `apps/web/components/task/chat/errands/use-errand-view-model.ts`: `0c14eb224a724f30aee4aa80a09d54fe57e798ca1e9c10bf422b82e13a28a687`.
- `apps/web/components/task/chat/errands/use-errand-view-model-live.test.tsx`: `d6ddbde59efba35c02bbb3fe264c528b9161d40b34d1ee83b9dcf2e9506d4662`.
- `apps/web/components/task/chat/errands/use-errand-view-model-resource.test.tsx`: `927eb5d6dc0c42204d29af05c250faf887f045a414716de6f57ae982ebdeb240`.
- `apps/web/components/task/mobile/mobile-session-picker-row.test.tsx`: `50174d76e5df7d83c1c92a2afa6faeaf03a755dd69372e24ccaa3e3a45b9aba6`.

The parent-owned package plan, prior final-verification documents, header test suite and both production callers retain their pre-task bytes. No permanent performance harness, store, renderer, backend, API, locale or public-copy file changed.

All check handles are terminal and the shared lock is released; `process-release.json` records no remaining check processes and 9,954,062,336 bytes (9.27 GiB) free at 14:25:20 UTC. Sole checkout writer ownership is released after these Results/handoff edits. No stage/commit/push, runtime/database operation, browser, build or comparison was performed.

The result is a measured 12 → 1 reduction in ordinary subscriptions and unrelated-update callbacks per consumer. It does not establish return-navigation speed, remove the original warning, or attribute that warning to Task01. Parent owns the normal-hook checkpoint, fresh managed desktop/phone controls and unchanged matched comparison. Prior historical misses, fixed-heap exception and profiling limitations remain disclosed; the pilot remains pending.
