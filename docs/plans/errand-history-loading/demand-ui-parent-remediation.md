# Task03 parent remediation, 2026-11-14

The parent recovered the completed review from the failed-delivery Errand and addressed its three findings directly. No writer is active. No runtime or live database changes were made.

- Full event rows and per-row revisions are evicted when an operational Errand settles. Explicit legacy complete-list consumers retain their existing inventory contract. A 500-child event regression proves the operational maps do not grow with finished history.
- Authentication/workspace scope changes immediately hide and evict browser summary/history/inspection state, abort shared read leases, and advance reset epochs. A-to-B-to-A and denied replacement-summary cases cannot reveal old history.
- Accepted Errand events reconcile owned inspection state, clear obsolete attention and reject older events. Session-only observations invalidate an open child inspection; delayed replies cannot overwrite a newer observation. Owner release removes detail and revision bookkeeping.

The broader session-only slice test exposed a missing optional Errands-state guard; it is fixed without changing ordinary membership behavior.

## Verification

Four original recovery regressions failed before the fix (`/tmp/kandev-task03-recovery-red.log`), then passed. The additional session-only inspection regression also failed before its fix (`/tmp/kandev-task03-inspection-red.log`). Final focused regression and ordinary auth/workspace/session controls passed **22 files / 274 tests**, `/tmp/kandev-task03-remediation-final-tests.log`. The additional delayed-response control passed with the session-only test (**2/2**), `/tmp/kandev-task03-inspection-final.log`.

Web typecheck and scoped Oxlint passed (`/tmp/kandev-task03-remediation-typecheck.log`, `/tmp/kandev-task03-remediation-lint.log`, `/tmp/kandev-task03-session-lint.log`, `/tmp/kandev-task03-inspection-lint.log`). Managed desktop demand-history acceptance passed **1/1** on a fresh production build (`/tmp/kandev-run.e2e.IOZ9apA4.log`); phone passed **1/1** against the same artifacts (`/tmp/kandev-run.e2e.RLD3axLp.log`). Raw output/screenshots remain in `/tmp/kandev-history-task03-parent-{desktop,phone}`. These checks do not substitute for Task04's ordinary compatibility and frozen performance gates.

## Source manifest

- `apps/web/lib/state/slices/errands/errands-slice.ts`: `322aec3328a092b621716496a38ccae680f52b93796470a6be9dbdc811ac5327`
- `apps/web/lib/state/slices/errands/types.ts`: `8c619818e35f8f982da1e77f68635804f23d6821dc9e5a0d240354797de6615b`
- `apps/web/lib/state/slices/errands/selectors.ts`: `a6183d63dcbbf1daa50be86826210548a95329a8979738c93afb11ead2c8f579`
- `apps/web/lib/state/slices/errands/errand-browser-scope.ts`: `6d4d51848b3d8fcc9cfffbef1c7a93551ccbc5722c66a5bf4bd39eb96d5c1f09`
- `apps/web/lib/state/slices/errands/errand-browser-read-coordinator.ts`: `2b9de4f9fe5fa1159da007c08bcadfc5520e1127b182d2ae51b90a028fa675d5`
- `apps/web/lib/state/slices/errands/errand-inspection-state.ts`: `287a2edd5052d6e908821f570ac31209931380680aea3459688b6e58d3be994b`
- `apps/web/lib/state/slices/errands/errand-browser-recovery.test.ts`: `e4b20eefacfb943cce5d69d35f50425508f22b3c8b04b78440f92f69b5e79f75`
- `apps/web/lib/state/slices/errands/errands-slice.test.ts`: `c2773b63f02bfa737372852e5d83471d9059908403f86b6e3ba03189c26b6108`
- `apps/web/lib/state/store.ts`: `df226af43d8a59c91fc097e7f1d868751a86c09f9342e6db541bc070b88832db`
- `apps/web/lib/state/shared-resource-reads.ts`: `277432b0e8bf7a61ecb423003d76aea73387e7d16072be9411cb33fcd959ee35`
- `apps/web/lib/state/slices/session/session-slice.ts`: `9d0ca924d237800fe9509efa0a6ebdf2320ed658fb4548939365399bb760f73a`
- `apps/web/hooks/domains/session/use-task-errands.ts`: `5f1fb95dbb353768c0671088329c488cfc51f3871bb5f812c774207044fd7ad2`
- `apps/web/hooks/domains/session/use-task-errands.test.ts`: `dac29a919dfc0d2ff50e1628fefe0b55a52c10289bbb112f2ee001f793a10009`
- `apps/web/components/task/chat/errands/use-errand-view-model.ts`: `1b7701ede7a715b8b6f45b84cb2a035361b0c3f5437a8e3f78b653d7ad5d5b11`
- `apps/web/components/task/chat/errands/use-errand-view-model-live.test.tsx`: `38ab6e732e66e4d1c0521b3c7bd0aab0b8619ccbc47d5383a90cfa8e1b50cca2`
