# Shared summary publication

Multiple mounted consumers already share one browser-summary request, but each consumer published loading and the resolved summary into Zustand. The new regression observes the real store subscription: one attempt emitted two loading updates and two successful updates. Error publication was already idempotent for the same error object.

The store now begins each attempt once and accepts its result/error only while that attempt is loading. Existing connection, revision, identity and replacement-attempt guards remain intact. The change removes duplicate store notifications; it does not remove the summary request or alter history freshness. Retry and shared-consumer lifetime controls remain covered.

RED: two failed, one passed, 28 unselected. GREEN: 51 tests across four files pass; typecheck, exact-file Oxlint and formatting pass. This shared state-only change has no rendered layout or touch changes; the mobile-parity state-only exception applies. The existing desktop/phone ordinary benchmark is being repeated separately to measure, rather than assume, any timing effect. Earlier results remain unchanged.

## Source and receipts

| Path | SHA-256 |
| --- | --- |
| `apps/web/lib/state/slices/errands/errands-slice.ts` | `e21ad83a27ae8d076c372472020b31269e0cf2318a2fe3fb6ff85fbf10780629` |
| `apps/web/lib/state/slices/errands/errands-slice.test.ts` | `04b202fd0d5aec9b8f86c377315b5e829a11c7c205aab0e551de2e44ec15abcf` |
| `/tmp/kandev-summary-publication-red.log` | `d8e3ab1c7610a0a65476b065c3ebecaa84402a48fc42fa5be229ba26830a5870` |
| `/tmp/kandev-summary-publication-green.log` | `fe9201f70207420407b7bea61f09164ee4526c7d78b2e87a81015c4861096e97` |
| `/tmp/kandev-summary-publication-typecheck.log` | `f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708` |
| `/tmp/kandev-summary-publication-lint.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-summary-publication-format.log` | `17aa973d3f004560237d9a95171210b0671deff23d61628eecf7322ff5938f20` |

## Rebuilt acceptance before upstream integration

Managed desktop acceptance passed 3/3: ordinary task navigation, demand-only bounded history and repeated child-cache release (`/tmp/kandev-run.e2e.SKvGAJuS.log`). Phone acceptance on the same artifacts passed 2/2: bounded drawer/history and repeated child-cache release (`/tmp/kandev-run.e2e.y4xHRFB9.log`). The measured follow-up is deferred until the upstream-integrated branch and matching upstream control are built; no pre-merge timed cohort was collected. These functional checks do not silently close earlier numerical warnings.
