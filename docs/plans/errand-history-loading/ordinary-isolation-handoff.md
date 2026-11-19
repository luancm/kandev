# Task02 ordinary loading isolation handoff

Implementation completed on 2026-11-13. The work order remains in progress pending parent review and integration.

## Review remediation checkpoint (2026-11-13)

- P1 stale child navigation: uncached child inspection and detail reads capture request attempt, navigation generation, selected task/session, and navigation-owner identity; each async boundary checks ownership before dependent work or publication. Ordinary opens, cached child opens, replacement requests, owner-context changes, A-to-B-to-A selection, and unmount invalidate older work. The deferred tests assert no stale session-store publication, selection, URL update, or parent-panel focus.
- P1 delayed saved layout: saved-layout application captures task/session, layout environment, Dockview API identity, attempt, navigation generation, and mount lifetime before resolving session identities. One shared attempt owner now advances for every layout replacement selection: saved/custom layouts, customized built-in overrides, synchronous uncustomized built-ins, and Reset Layout. A same-owner replacement invalidates pending saved/custom completion before it can apply or persist over the selected layout.
- P2 persistence verification: the SQLite ordinary-reader assertion compares the entire projection with the complete-reader control filtered only by Errand role. The existing environment-gated PostgreSQL session-summary test invokes the same assertion. Both tests ran against the SQLite control and an isolated PostgreSQL 16 cluster on loopback port 55488; the cluster used unique test schemas and was stopped after verification.

### Review remediation receipts

- Behavioral RED before the fences: child-navigation regressions failed 5 of 11 tests and saved-layout regressions failed 5 of 12 tests. Logs: `/tmp/kandev-task02-review-use-open-red.log` SHA-256 `9fb8319c467b7ca496b6e25219a9a7554bc3deac466f1c1491ecee42c1491359`; `/tmp/kandev-task02-review-layout-red.log` SHA-256 `61a05795fe547b3dbd128e4140b359b7522b8447254f0387fcb5d25b91ffd0e2`.
- Same-owner selection invalidation RED: the selector regression file failed 2 tests / passed 12, with saved A applying after both built-in B and Reset Layout were selected. Log: `/tmp/kandev-task02-preset-invalidation-red.log`, SHA-256 `c3c16dec5f6b6a2084804610d61243df40cc273dbe825cbd777a758592e53e7a`.
- Same-owner selection invalidation GREEN: the focused selector file passed 14/14 tests, including prior saved-layout owner, task/environment/API, and unmount cases. Log: `/tmp/kandev-task02-preset-invalidation-green-final.log`, SHA-256 `12cb83cb5eb44c7d1d7b77320cbdef039b1191682979d81a6af2c45d9f223c45`.
- Focused web typecheck passed, log `/tmp/kandev-task02-preset-invalidation-typecheck.log`, SHA-256 `f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708`. Exact-file Oxlint, TypeScript Prettier, and source diff checks passed, log `/tmp/kandev-task02-preset-invalidation-code-static.log`, SHA-256 `17aa973d3f004560237d9a95171210b0671deff23d61628eecf7322ff5938f20`. The final Task02 document Prettier and diff check passed, log `/tmp/kandev-task02-preset-invalidation-doc-static.log`, SHA-256 `17aa973d3f004560237d9a95171210b0671deff23d61628eecf7322ff5938f20`.
- No new browser run was needed: the change only advances the selector's shared layout-application attempt and does not alter rendered geometry or touch behavior. The deferred component test exercises the actual preset menu callbacks; prior desktop and phone navigation controls remain unchanged.
- Focused GREEN: 3 Vitest files, 27 tests passed, log `/tmp/kandev-task02-review-web-focused-green.log`, SHA-256 `ae21d6407aa3793592a208f908852d196d0b923092f31cf9b8974ce39c063e7c`; web typecheck exited 0, log `/tmp/kandev-task02-review-typecheck-final.log`, SHA-256 `f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708`; exact-file Oxlint with deny-warnings and Prettier check passed.
- Persistence: race-enabled SQLite ordinary control plus the environment-gated PostgreSQL 16 projection test passed with no skips in `/tmp/kandev-task02-review-projection-postgres.log` (SHA-256 `07a9d3f7be5b28a5dd489cfed50ed05907449e060f9b5a3fb53d675df839f4c6`). `go run ./cmd/sqlguard ./internal` exited 0. `go test -race -parallel 1 ./internal/persistence/storeconformance -count=1` passed in 191.781s with `KANDEV_TEST_POSTGRES_DSN` set, enabling its PostgreSQL engine case; log SHA-256 `7482c6885f539927df9c3dd483e671ff2aa45b402ef01569c2c7e0d458a21eb5`. Scoped changed-Go lint reported zero issues, log SHA-256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`.
- Browser integration: the existing desktop task29 normal-navigation control passed 1/1 (`/tmp/kandev-run.e2e.4TqM2PR4.log`, SHA-256 `26fa1d8ef876d9ffcbb02e9fd9f19164227c4a25284c9d9235aa9b293537ae35`); the native phone task29 control passed 1/1 (`/tmp/kandev-run.e2e.rDxT447u.log`, SHA-256 `de65fa3d221342837f3df2c0ebd7b1bdbc90906b71b509dcc681bbbbd60c2795`). Playwright discovery listed exactly one intended test per project. Both runs were managed, host-mode, single-shard, and sequential.
- The shared build lock is released, free disk is 18 GiB, and the disposable PostgreSQL process has stopped. The work order remains `in_progress` pending parent acceptance; no commit, push, PR, or publication was made.

## Delivered behavior

The additive ordinary summary repository/service option filters Errand rows in SQL before session-summary materialization and enrichment. Existing complete summary/list methods and defaults remain complete. Browser HTTP/WS list paths and task-detail boot explicitly use the ordinary projection; boot side maps and ordinary membership are built only from ordinary rows.

Ordinary browser navigation now uses the ordinary list API in the task-session hook, navigation read helpers, task route resolver, and server-rendered session-page state. Complete operational reads remain complete, including task removal's full deletion inventory, message-request and assignment/comment readers, the Office task page, E2E inventory helpers, and the existing full Errands panel read reserved for Task03.

Session membership reconciliation centrally rejects Errand rows. Targeted child details remain available without joining ordinary task membership; child WS events neither insert a child row into the ordinary list nor trigger ordinary-list invalidation.

A direct child URL resolves through inspection of only that requested child and one targeted session-detail read. The route validates task, role, and parent identity, retains minimal parent identity, and does not enumerate siblings or fall back to the first/primary ordinary session while the child is unresolved. Missing parents use the existing generic child-reader/unavailable path, and queued reserved children do not fabricate a session. Direct child reload and Back preserve parent ownership without changing ordinary tabs.

Every saved-layout materialization branch applies the authoritative ordinary identity allowlist before session-panel creation, including normal, maximized, pre-maximized, global migration, environment restore, and custom-layout paths. When identity is unknown, only session panels are held; non-session panels remain, no temporary filtered layout is persisted, and the original layout is replayed after identity resolves. The tests control delayed identity and observe panel creation both before and after release, while retaining saved order, geometry, and surviving maximize grouping.

## Caller audit

| Call site or API                                                                                               | Classification                         | Task02 outcome                                                           |
| -------------------------------------------------------------------------------------------------------------- | -------------------------------------- | ------------------------------------------------------------------------ |
| ListTaskSessionSummaryObservations and complete task/session list methods                                      | Complete internal/default contract     | Kept complete; no global narrowing                                       |
| New ordinary summary repository/service reader and browser HTTP/WS list wiring                                 | Browser navigation contract            | Explicitly selects ordinary rows                                         |
| boot_state_task_detail.go ordinary membership and side maps                                                    | Browser boot contract                  | Uses ordinary rows; a requested child is inspected and loaded separately |
| use-task-sessions.ts, task-navigation-reads.ts, resolve-task-route.ts, session-page-state.ts                   | Ordinary navigation                    | Uses listOrdinaryTaskSessions                                            |
| use-task-removal.ts                                                                                            | Complete deletion inventory            | Still reads complete task sessions before removal                        |
| message-request.ts, use-mcp-assignment-targets.ts, use-run-comment.ts, Office task page, E2E inventory helpers | Complete operational or test inventory | Retains complete reads                                                   |
| Existing Errands panel list                                                                                    | Complete Errands contract              | Unchanged for Task03                                                     |
| setTaskSessionsForTask and WS reconciliation                                                                   | Authoritative ordinary membership      | Errand rows are filtered; targeted child detail stays separate           |

## Verification receipts

No behavior-failing RED receipt was retained for the original Task02 implementation slice. The supplemental reviewer regressions did produce assertion failures before the fixes; their RED logs and hashes are recorded in the review remediation checkpoint above. The available earlier typecheck RED at /tmp/kandev-task02-typecheck-red.log, SHA-256 e7a9384514e8e3e4001352687022f79d473c7e3f944ab49ae16d6a3aa37a912b, reports branded test-fixture and route-helper type mismatches rather than an assertion failure. The corrected final typecheck passed; this compile-time receipt is not presented as behavioral RED evidence.

Backend race command: from apps/backend, GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB with GOCACHE=/tmp/kandev-history-baseline-gocache and GOMODCACHE=/tmp/kandev-history-baseline-gomodcache, run go test -race -p 1 -parallel 1 ./internal/backendapp ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run 'Boot|SessionSummary|SessionScope|Errand' -count=1. All four packages passed with no skips reported: backendapp 30.846s, service 2.781s, handlers 1.552s, SQLite 72.929s. GREEN log: /tmp/kandev-task02-backend-race-final.log, SHA-256 7e94556e7cb18d7cc2c99de02f4a14662d4d7a214391e166c89d9089557bbc63.

Task02-scoped scripts/lint-go-changed over the 12 backend paths in the source manifest passed with zero issues. GREEN log: /tmp/kandev-task02-go-lint-scoped-final2.log, SHA-256 e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47. The wider changed-package lint attempt included unrelated Task00/Task01 packages and timed out during package loading; it is not a pass. Log: /tmp/kandev-task02-go-lint-final.log, SHA-256 81ff2de3b9f14a98b26c0230721d14a4c3016dbffaa361f3ef077b13a9aae4b2.

The focused Vitest selectors covering ordinary hook freshness/reconciliation, route and SSR child identity, direct child opening, session-store membership, and Dockview normal/maximized/environment/custom restoration passed 19 files and 302 tests. GREEN log: /tmp/kandev-task02-focused-vitest-final4.log, SHA-256 39288724cf21fa22e885079f4422e1b456e43d81de10c9e925c3bc1835362fca.

Web typecheck passed. Log: /tmp/kandev-task02-typecheck-final6.log, SHA-256 f38256cb2ac933c36933ca78bbd757a1fc05f6d3c0ac681c064773e0218ed708. Changed-file Prettier, exact-file Oxlint with deny-warnings, i18n check, and i18n new-code ratchet passed. Log: /tmp/kandev-task02-web-static-final.log, SHA-256 1e5b65a8b4e61ab5aa679c66a74e7376f528c3d4aeceaa25c09ee3a2de7f2bfe.

Desktop ordinary/direct-child browser control passed 1/1: task29 normal conversations keep tabs while parent-owned errands open temporarily and history stays collapsed. Log: /tmp/kandev-task02-e2e-desktop-final.log, SHA-256 88c83defeed64c55181c1cd05988ee613b97c77204782a30525e8268f6b3bfb8.

Native phone ordinary/direct-child browser control passed 1/1: task29 phone separates normal conversations and parent errands with temporary child navigation and quiet history. It covers direct child URL, reload, and Back with ordinary tabs retained. Log: /tmp/kandev-task02-e2e-phone-final.log, SHA-256 e6119f073921f86d504920139e65f11e094aa75dbbdc71e9a0ad292fe07c361c.

Changed Go files are gofmt-clean and git diff --check passed. Although Task02 changed no schema or write path, the reviewer-requested persistence checks were run: `go run ./cmd/sqlguard ./internal` exited 0 and the race-enabled store-conformance suite passed with the isolated PostgreSQL DSN enabled. The supplemental ordinary-reader projection check exercised SQLite and PostgreSQL 16. The shared build lock is released and no browser, Vitest, Go race, or Go lint process remains. The user's main runtime and database were untouched. Generated inactive E2E binaries were retained as shared worktree build outputs.

## Limits

These checks do not claim the candidate 0/100/1000 finished-child performance matrix or full PostgreSQL feature certification. The browser controls prove the exercised ordinary navigation, direct child, reload/Back, and delayed-identity restoration paths, not Task04's full performance acceptance or every possible browser flow. Task03 still owns demand-history UI. No commit, push, PR, external publication, or parent-owned task plan/status edit was made.

## Source SHA-256 manifest

The hashes identify the final Task02-owned production and test files.

| File                                                                            | SHA-256                                                          |
| ------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| apps/backend/internal/backendapp/boot_journey_test.go                           | 54fd42af2f2e7e6aaa29ff666ba474076cf66ee351ecb6061c1f22ad58b8d322 |
| apps/backend/internal/backendapp/boot_state_task_detail.go                      | f5f3b438103e48730712b543738e6ec2047e9b9bf7bbb195f6fdfca36538bdbd |
| apps/backend/internal/task/handlers/task_handlers.go                            | 50cbea5796df42fb6ae4b096fc8e82069996e989fdffa3150c2915bd45f4a653 |
| apps/backend/internal/task/handlers/task_http_handlers.go                       | 42192b57f2cd901c2a1d009023c029e6deb3d7c9bbea5cffcd7837d3364d0323 |
| apps/backend/internal/task/handlers/task_session_summary_authorization_test.go  | 0a4bde705b41c391cae2ea062d7ee73e4444064dc8cc8dc78644c385634d8a4b |
| apps/backend/internal/task/handlers/task_ws_handlers.go                         | 3f4acc9f770f978832a4cb1047869b33043709bcd6352b1687ddf78e7e467664 |
| apps/backend/internal/task/repository/interface.go                              | 453cbdf3d8f991a058ce61be6055b56f14914dbf66b3aba63ef1a36b7de69fe9 |
| apps/backend/internal/task/repository/sqlite/session_summary.go                 | 21e63576dd3899cabd8a32fcea0910fcca4e8fff9fa5169f125f56bdd390f122 |
| apps/backend/internal/task/repository/sqlite/session_summary_projection_test.go | a88ccdce84e39ffe63c0b488b7458a1b95c849fb4bcbfb19778cd5d39c400b92 |
| apps/backend/internal/task/service/service_session_summary_projection_test.go   | d0a4cb6e8027dcd817766cc8418b5f45b10588b5c1ff626d7037ae5ff1694ce3 |
| apps/backend/internal/task/service/service_sessions.go                          | 42ce5085f4a4474f5679cc8137f2d7be80625422cf0a47914dcb02ba5c148d46 |
| apps/backend/pkg/websocket/actions.go                                           | f8473fb45aec6ebd1e358b5428a4126e35216b17d9c7c0defaef4395136f1236 |
| apps/web/components/task/chat/errands/use-open-task-session.test.ts             | df283950fa1d23557b1dc2328d06a9a8ac34cda011cd936b1e4f69f5f83575bb |
| apps/web/components/task/chat/errands/use-open-task-session.ts                  | ca69cb5cf5c8e672b5ae4e561163f967677cc49fd93ea02e9eb738f68fb13839 |
| apps/web/components/task/dockview-desktop-layout.tsx                            | 73aa56800ae57b45ce2c46b82afc03b4850a7e96b987ef5315ae237f533fdb8b |
| apps/web/components/task/dockview-header-actions.tsx                            | 06ae6f423eef66c18a98da320074f3e8b90de7673080504a1fa0d77d81334f51 |
| apps/web/components/task/dockview-layout-restore-maximize-session.test.ts       | 86c14ab658e69f72f41dc8390ef963a39fc3cb9fd4dfb243201ccb0d2037eba1 |
| apps/web/components/task/dockview-layout-restore.test.ts                        | 88c9fe7b72c5b911417802abc38d5cdf0c15a117af4f50a1d6f2e72b6d63104f |
| apps/web/components/task/dockview-layout-restore.ts                             | d26bdf54b631128c9a53736e37fe39fd37fc9df8e15f49a6eb863c51589e565b |
| apps/web/components/task/dockview-panel-content.tsx                             | 47a27f322645a4a6b371671e322e6b23346f874f1126674b5e5160c2765dcc32 |
| apps/web/components/task/dockview-session-tab-sync.ts                           | 8b2696a1f4dc65566c1c47dbce0b92989a075c847698f88bb76829a34575162d |
| apps/web/components/task/dockview-session-tabs.test-utils.ts                    | b0351ccacd1af64c65d764ee16b326ab641f675b63b1aeb07ff7a76c4c879788 |
| apps/web/components/task/dockview-session-tabs.ts                               | aca7c896490a59e99843d52ee674456272027d11f3e3b7ba12c59168259c449e |
| apps/web/components/task/layout-preset-selector-session-ids.test.ts             | 913c667a5762cae6d23e2909c0873a2b2dd936d1509cc807115c79b93429a98c |
| apps/web/components/task/layout-preset-selector-session-ids.ts                  | b2642f6641fcf8d875882d09b97328fe2474aba28b8a63830110c11252b6bbd5 |
| apps/web/components/task/layout-preset-selector.test.tsx                        | 6c6705d26a656fdf241bee5cb404640790e08cfdfe7564e13c60b59b3144b52e |
| apps/web/components/task/layout-preset-selector-apply-ownership.ts              | 36aabb19d14414b852e05a261af9e176a98b57428f800132e29a9f7a66a472e5 |
| apps/web/components/task/layout-preset-selector.tsx                             | d340b1de459201dabb244f8f691b2c88e065bca1b5043225bcf7b84b66406e73 |
| apps/web/components/task/sessions-dropdown.tsx                                  | 678751289a167d4697bb3e707cc8a8f2f5e45d69d0976d67bd5adbdf90859083 |
| apps/web/components/task/task-conversation-view.tsx                             | ab4d80a4ee4f480c42286be6d2f71bc951209c43081dcd732f87a5beb249113b |
| apps/web/components/task/task-select-helpers.test.ts                            | a3c70cb78e4f9f117aa2cf328157dc5231b6e214914fe33148612eafbd87a7b2 |
| apps/web/components/task/task-select-helpers.ts                                 | e9af716df1000bcb641423c4a53ebcbf13565afad1c4e7a5c288faab5a4eab4c |
| apps/web/hooks/use-task-removal.ts                                              | 414a6a66bb03791d534a9c287c84a17675ea4a139cce8d13c88e0d57486ea5a8 |
| apps/web/hooks/use-task-session.ts                                              | 8de61c37f7653883fcb0f98b964e19b9fa582d49acc1154d5355a8d1af6689b0 |
| apps/web/hooks/use-task-sessions.foreground.test.ts                             | eeb68584f70fbfd27212336183056e200c51c869088cbf95ef05de74de08665d |
| apps/web/hooks/use-task-sessions.stale-reads.test.ts                            | 5075cf15a81d3667d1eae456624a7c4c7f988576c23615ec3a563d9769ed7b24 |
| apps/web/hooks/use-task-sessions.test.ts                                        | e31116975cb934a0f1ab3525dad38ba1673dd639b4249aa64434e0fb43b844da |
| apps/web/hooks/use-task-sessions.ts                                             | 0f3675fcf3c139c865679ebda9a1ae60f5f62cf9ab01a2895b77c2f357c983cb |
| apps/web/lib/api/domains/session-api.test.ts                                    | 68ca432a98992c1470a5a2884e869c0c04c76a844a17939daf68f61f11e0b289 |
| apps/web/lib/api/domains/session-api.ts                                         | 1f4bcbb6d77b1e78871b0c51a039d5de7f058e42d02ebe5be81a0f1a84845fb6 |
| apps/web/lib/routing/resolve-task-route.test.ts                                 | d753e4bbfddf9cce81d6a05be9ea5f0291c5f87cbf1f5bcd0d7006ce9008fcb8 |
| apps/web/lib/routing/resolve-task-route.ts                                      | 086ea511b8115a40687d0b5ee2578cdeee3858df4eeab2a13a763e58535526d4 |
| apps/web/lib/ssr/session-page-state-child-route.test.ts                         | 8358822450606c288a519a2a87fbae54e36392d605b0b97565386bb20d36094f |
| apps/web/lib/ssr/session-page-state.test.ts                                     | 4b7b1662362f26ff1726511921a8f18485eb87a5e8fa707caba9423796f39750 |
| apps/web/lib/ssr/session-page-state.ts                                          | 57479b035457ace948181d707a863f516fe1624f1be5b135c0fa16919f45c06d |
| apps/web/lib/ssr/task-session-route-identity.ts                                 | 9e2cb96326b14db042de1f2610afbe4b16de5e77861d193fae54e5741bfd6bd5 |
| apps/web/lib/state/dockview-env-switch-action.test.ts                           | 65eeff0f3404e9c924b1e03106b046db7f45145cc1b90e2b179151d507848c29 |
| apps/web/lib/state/dockview-env-switch-maximize-session-filter.test.ts          | 3b422cacf3d3eac2b294dd5d814364beb12d16363bb3f2f425f0a82afc5a687a |
| apps/web/lib/state/dockview-env-switch.ts                                       | 57218fe290192771207d184c480f748d0a4df40125b0cc2f1c93a35655a05cd9 |
| apps/web/lib/state/dockview-preset-persistence.test.ts                          | 1e3871aa597d4a8a29c47fd481625c77b26f9b6079b213a4440d459c03562b24 |
| apps/web/lib/state/dockview-session-restore-gate.ts                             | 31b7cef415f06528f05080d57e3d22483d26880a828c67abb49b47110f6ebdff |
| apps/web/lib/state/dockview-store.ts                                            | 6e874a0ed6c73d72441adffce50b444fcb4277b5f01147c6cd1f1879bdc5f33e |
| apps/web/lib/state/layout-manager/sanitize-serialized-layout.ts                 | 11094c2b38cd43056cbacd384763207d3d0bbe984c9abec97af0977fdfa6128f |
| apps/web/lib/state/slices/session/session-slice.upsert.test.ts                  | 391c4ff2c045bf0ab4e511eecce0aea927384c935150f5287ac633bb61fd3fef |
| apps/web/lib/state/slices/session/session-slice.ts                              | deb389ddcaf92f409c513c4aef3522d0c96c90f5487aab63688867a5768c14ad |
| apps/web/lib/state/task-navigation-reads.test.ts                                | c2d6c1e23b3f32884bf107aa0d63db853ec6e83309b86b7e5d29f4a82544fb7d |
| apps/web/lib/state/task-navigation-reads.ts                                     | 4a6073131592e50ca64d50fa674e69477ad40ba3e22d44a4b23c4673de038174 |
| apps/web/lib/task-conversation-navigation.test.ts                               | 4d57d9001dd5de923c62558dbdd8d857b546a74c5078ef6be8842cac0a20c4b4 |
| apps/web/lib/task-conversation-navigation.ts                                    | d4aebb3d1ebdfde6089dd501c99eb0bd0df37f49011aa48c458767fc52cf6816 |
| apps/web/src/task-detail-route.tsx                                              | d6e8682887d1138a21fe57fac61a5b2ef5f88785c95c35dbdc6c77f46bb02e74 |
| apps/web/src/task-session-admission.tsx                                         | 035f38bb313fcd142682c98f404cc84a9f08eda46e974526adb1268915d06534 |
