# Parent terminal notification acceptance handoff

## Result

This packet closes Era Sol's P1 acceptance gap for automatic Errand result notifications after an ordinary parent terminal arrives without a producer `TurnID`. No production code changed. The new evidence pairs actual lifecycle intake and SQLite identity resolution with actual orchestrator settlement, notification admission, Auto-run dispatch, and Errand result delivery.

`TestParentTerminalIdentityResolvesSQLiteAdmissionThroughLifecycleIntake` calls the lifecycle package's real `StreamManager.processAgentEvent` with a submission-bearing terminal whose producer payload has an empty turn ID. Its SQLite fixture contains the exact prompt submission and admission binding. The test checks the callback turn, projected terminal effect, and byte-identical retained producer payload. A local fault-injection wrapper hides only the optional turn resolver while retaining the real SQLite receive/projection methods; `TestParentTerminalIdentityFaultInjectedResolverLossIsObservable` proves the callback assertion detects missing identity.

`TestParentTerminalIdentityAutoRunsEmptyErrandNotificationAfterLifecycleRepair` has fresh-terminal and incomplete-effect-reconciliation cases. Each uses real SQLite submission, prompt admission, transcript, terminal, and pending Errand result records. The full orchestrator completion handler settles the parent, admits one empty `QueuedByServer` notification through the normal Errands deliverer, composes the pending result at dispatch, and sends it to a controlled provider with Auto-run enabled. Assertions cover exactly one provider call and result composition, an empty queue afterward, the original turn's committed `not_required` workflow outcome, delivered notification accounting, preserved earlier transcript, and unchanged pre-existing successor state. The incomplete case first removes the derived terminal identity and returns the effect to pending, then invokes normal settlement reconciliation before completion handling.

The notification fixture keeps its pre-existing successor completed so the normal parent-readiness guard allows notification admission, then verifies its completion timestamp remains unchanged. An exploratory active-successor fixture was correctly held by that guard and did not admit a notification (`/tmp/kandev-parent-terminal-notification-final-focused.log`); active-successor preservation remains covered by the ordinary queued-message control, which is not counted as notification acceptance.

`TestParentTerminalIdentityKeepsErrandNotificationHeldWithoutSafeEvidence` retains missing-admission and conflicting-terminal controls. Both admit the empty notification through the normal Errands parent-settlement path, then verify the guard keeps it queued, leaves the result pending, and makes no provider call or workflow settlement.

The ordinary queued-user-message tests remain controls for parent queue release. Existing Send Now tests remain controls for manual recovery. Neither path is used as evidence for automatic Errand notification acceptance.

## Package boundary

Lifecycle's `StreamManager.processAgentEvent` is private to `internal/agent/runtime/lifecycle`, while Errand notification settlement and provider dispatch are owned by `internal/orchestrator`. There is no existing public lifecycle entry point for one orchestrator test to invoke without adding a production API solely for test convenience. The two package-local tests therefore exercise the real boundaries independently against SQLite: lifecycle asserts the projected terminal and callback derived from exact admission; orchestrator seeds the same concrete submission/admission/terminal artifact shape, exercises normal reconciliation and completion handling, and asserts the resulting notification effects. Both use real prompt-admission and terminal identities; neither supplies a canned resolver or fabricated successful identity.

The test receipt below gives the exact focused and notification-control commands, matched counts, and logs. SQLite has no `TestParentTerminalIdentity` tests, so its selector output is reported as zero matches rather than a passing test count. PostgreSQL-backed coverage is not claimed.

## Validation receipt

The final focused race selector passed nine top-level tests: two lifecycle tests and seven orchestrator tests, with four nested notification subcases also passing. The SQLite package matched zero tests and explicitly reported `[no tests to run]`. The notification controls passed four top-level tests and two mixed-source subcases. No tests were skipped.

The exact commands and retained receipts follow.

Focused race command: `(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestParentTerminalIdentity' -count=1 -v)`.

Notification-control command: `(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run '^(TestErrandParentDelivererRequestsReadinessForAdmittedNotification|TestErrandParentDelivererLeavesNotificationQueuedWhenAutoRunIsOff|TestSendNowMixedErrandSourcesPreserveOrdinaryInputAndIdentity|TestSendNowStaleNotificationOnlyStartsNoTurn)$' -count=1 -v)`.

Exact-file lint command: `flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go apps/backend/internal/orchestrator/parent_terminal_identity_test.go apps/backend/internal/orchestrator/errand_completion_cleanup_test.go`.

Format/diff commands: `gofmt -d apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go apps/backend/internal/orchestrator/parent_terminal_identity_test.go apps/backend/internal/orchestrator/errand_completion_cleanup_test.go`, `gofmt -l apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go apps/backend/internal/orchestrator/parent_terminal_identity_test.go apps/backend/internal/orchestrator/errand_completion_cleanup_test.go`, `rg -n '[ \t]+$' apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go apps/backend/internal/orchestrator/parent_terminal_identity_test.go apps/backend/internal/orchestrator/errand_completion_cleanup_test.go docs/plans/errands-remaining/notification-acceptance-handoff.md`, and `git diff --check`. The formatters and diff check were clean, and the whitespace scan had no matches.

Both race commands exited 0. `gofmt -d`, `gofmt -l`, a trailing-whitespace scan across the three Go files and this handoff, and `git diff --check` were clean. The exact-file Go lint was attempted twice; its first run ended while loading packages with `context deadline exceeded`. The warmed-cache retry reached analysis and emitted one QF1008 on `durableDeliveryTestAgentManager` in the separate cleanup test file before it timed out during package loading. Lint did not complete, so a clean full result is not claimed. This packet removed only the separately authorized `retentionPromptAgentManager` selector; the remaining nested selector was left untouched because the permission was limited to that named cleanup.

An exploratory run with an active successor correctly failed notification admission in both positive subcases because the parent-readiness guard treats that successor as live. That fixture state was reverted. The final tests use a completed successor and assert its completion timestamp is unchanged; this exploratory guard behavior is retained in `/tmp/kandev-parent-terminal-notification-final-focused.log`.

| Evidence | Log | SHA-256 |
| --- | --- | --- |
| Final focused race selector | `/tmp/kandev-parent-terminal-notification-final-focused2.log` | `35a9ad3ba3fd466a9a0c31a55e5093b7bbd91a1b01bd7bf9cf978cff05fea52e` |
| Existing notification and manual Send Now controls | `/tmp/kandev-parent-terminal-notification-controls.log` | `1a14dff665a7967eb7b4c499fc13533c3a4f6b6d366f02a97c15a57f0bf901e9` |
| First lint attempt, package-load timeout | `/tmp/kandev-parent-terminal-notification-lint.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Lint retry, QF1008 and package-load timeout | `/tmp/kandev-parent-terminal-notification-lint-retry.log` | `970d9a24bfa9859d650b8e7f2babe642e2e205f056ca57ab69242dfdc13969b9` |
| Exploratory active-successor readiness guard | `/tmp/kandev-parent-terminal-notification-final-focused.log` | `37cacce82402d1aa50dd8ab954c0b246c8d20efb165a6facdab93d1aaae3d672` |

## Source hashes

| File | SHA-256 |
| --- | --- |
| `apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go` | `d2169134507b291dac6421583a2f85035895b4901fb716415e2d37d3d3449c45` |
| `apps/backend/internal/orchestrator/parent_terminal_identity_test.go` | `52e9df9f5b40db48861d210b1e6bfc9489650840716a4efb70b10b56ed2e8460` |
| `apps/backend/internal/orchestrator/errand_completion_cleanup_test.go` | `2781cf69b9a726421c0ca54cfda5c019a08b028fa3a55e3e856548acb070b874` |

## Scope and review

Only the two parent-terminal identity test files, this handoff, and the explicitly authorized redundant embedded-selector removal in `errand_completion_cleanup_test.go` belong to this packet. Shared duration, cleanup, and recovery changes were preserved. No production API or production code was changed. Tests used isolated SQLite fixtures. The packet did not run against or modify a live runtime or database, and did not stage, commit, push, or mutate Git state.

Ready for Era Sol review with the lint timeout/QF1008 limitation above. Review disposition and integration remain parent-owned.
