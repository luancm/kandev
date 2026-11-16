# Task04 compatibility checkpoint

Task04 is in progress. Tasks00–03 are accepted; functional compatibility fixes below are verified, while measured performance acceptance remains open. No commit or push was made. The separate terminal-delivery repair package remains at its design checkpoint.

## Changes and functional evidence

Explicitly inspected children now remain selected in Kanban preview and Threads without entering ordinary session membership. Parent tabs stay highlighted, and ordinary navigation remains available. Known compact-history child session events invalidate history without inserting full ordinary rows. Obsolete history responses leave loading state and retain explicit Refresh; rejected stale data is not published.

The initial ordinary/demand-history controls passed 31/31 desktop and 7/7 phone (`/tmp/kandev-run.e2e.R7kAT9pO.log`, `/tmp/kandev-run.e2e.ELlDc9zK.log`). Broader existing Errands controls exposed additional defects and drove RED regressions for preview selection, Threads selection, compact session events and stale-page loading. Focused GREEN suites passed 206 and 127 tests.

The additional desktop run passed four cases and failed three before the final fixes (`/tmp/kandev-run.e2e.qb729lgs.log`); the three failed cases then passed on a fresh build (`/tmp/kandev-run.e2e.gXXr5kyi.log`). The phone batch recorded ten passes before external termination, exit 143 (`/tmp/kandev-run.e2e.YoT61URS.log`); the remaining normal-context scenario passed separately (`/tmp/kandev-run.e2e.j2J6O0wI.log`). This is individual passing scenario evidence, not one clean eleven-case batch. The termination cause remains unproven. The inspected phone history screenshot shows contained scrolling and footer actions.

The final combined unit run recorded 310 passes and one failure caused by the strengthened Threads test sending click instead of the tab's mouse-down event (`/tmp/kandev-history-last-green.log`). Correcting that event produced a focused pass (`/tmp/kandev-history-thread-final.log`). Final typecheck passed (`/tmp/kandev-history-final-typecheck.log`). Scoped lint reported one repeated test timestamp warning; extracting the timestamp constant cleared the focused lint (`/tmp/kandev-history-thread-final-lint.log`). The remaining scoped files passed (`/tmp/kandev-history-final-lint.log`). No new production change followed the final browser evidence.

## Performance gate

The first candidate completed all 42 scenarios but failed five of thirty frozen budgets; [candidate-results.json](../errands-simplification/evidence-index.md#candidate-results) retains the measurements unchanged. Structural bounds passed across those samples: three ordinary rows, at most 100 history summaries, and no child subscriptions after Back. Structural success does not establish timing or heap acceptance.

Two lazy-import experiments did not meaningfully reduce retained heap and were removed. Temporary isolated profiling measured the ordinary reader below 1 ms while existing whole-boot projections took hundreds of milliseconds; this does not prove the source of the candidate regression. No diagnostic production code remains.

Fresh original-source controls contain two desktop samples and seven phone zero-history samples, retained in [baseline-control-results.json](../errands-simplification/evidence-index.md#baseline-control-results). They also exceed the original frozen heap/reload limits. Frontend and fixture hashes match the original baseline; the backend binary hash differs. The user decision on retaining absolute limits versus a fresh paired comparison remains pending. Original caps and failed candidate samples remain unchanged; no regression-free claim is made.

## Resume and resource state

No owned build, browser or test process remains. The shared lock is free and approximately 20 GiB is available. The temporary control worktree at `/tmp/kandev-history-baseline-control` is retained for possible paired comparison. Main-worktree artifacts were built during the removed lazy-panel experiment, so rebuild restored eager source before another candidate browser run. Do not reuse those artifacts as final source evidence. Preserve original measurement logs and samples; do not rewrite their budgets.

## Final remediation source hashes

- `apps/web/components/task/use-preview-conversation.ts`: `0fc5019cd78926997414c77b7caa78895a53195c774770caff2b278c34edc7c7`
- `apps/web/components/task/preview-session-tabs.tsx`: `2805051683d7fff33f18ec7eb9ae70a5be6d9c11c8ccc08ffc86646e13329fd6`
- `apps/web/components/task/preview-session-tabs.test.tsx`: `e51fc39f96e18d650aec0316b560d93b3c4db320ef5a2658a011104a0934ae5d`
- `apps/web/hooks/domains/session/use-inspected-errand-session.ts`: `c946f05d803394d39e0bfcd328ae413c0764c171e87c66479426be57404c5b8e`
- `apps/web/components/threads/thread-column.tsx`: `59eaea5dd27ee99b8d6bb17897bf3c9e3ae44a4ac7a2c75c1d3e8750154362fc`
- `apps/web/components/threads/thread-column-errand.test.tsx`: `b9c1af5d4f3bdfcee61ccd41db7c9b6c32aa232c01367f16924d910b897f8c00`
- `apps/web/components/threads/thread-session-switcher.tsx`: `3bddd813a5756e2c0d32dd43e92724b762cda0e385f9c203a63c888890e9e156`
- `apps/web/components/threads/threads-board.test-helpers.tsx`: `25ba0da6b50450612b627a5a5baf47bffecc183e1b4188e13f51d62523bb95b5`
- `apps/web/lib/state/slices/session/session-slice.ts`: `f5acfed275b6fe1b7272fb0e17e46b4c3e5d9f1f85c718280706fb04cad39592`
- `apps/web/lib/state/slices/errands/errands-slice.ts`: `e19a9a9642340f582cb01a7d606355aa4de8663c4970612ad26764f1297d405f`
- `apps/web/lib/state/slices/errands/errand-browser-recovery.test.ts`: `bf77e2abb49c1ca62c8efadbe210550e6424c2ac4ebe3a7399c74c4adc2b2b20`
- `apps/web/e2e/helpers/errands.ts`: `b2fc797289685b3b2c31bb454d4907281a7771257ad086a426a6fbca14145c9a`
- `apps/web/e2e/tests/session/errands.spec.ts`: `3c6281e364929aab8b4197e698339863f988c417255b53f97bae5056a1eb6fa9`

The user authorized the fresh paired comparison on 2026-11-14. The [paired protocol](paired-performance-protocol.md) fixes interleaving, samples, unchanged threshold formula and structural gates before collection. Earlier references to a pending user decision are historical; Task04 remains in progress until this matrix passes.
