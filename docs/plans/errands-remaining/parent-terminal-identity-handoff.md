# Parent terminal identity repair handoff

## Result

The regression came from `9db45d3e75`: removing the missing-turn fallback was correct for uncertain Errand ownership, but the retained/canonical resolver only returned a turn for Errands. An ordinary parent terminal could therefore reach projection without its turn, and the existing guarded workflow settlement left the queued notification undispatched.

Lifecycle now resolves ordinary parent terminal identity from exact submission/admission evidence while keeping the resolver's Errand-role boolean separate. Callback attribution is derived without mutating the received payload. SQLite admission and projection use the exact admitted turn; native no-submission activity and Errand-specific behavior remain covered. Already-projected incomplete terminal effects can be repaired only from the retained terminal and its immutable exact prompt admission, under the existing writer locks and transaction. Conditional repair fills an empty identity only, rejects missing or conflicting evidence, and does not repair completed effects. Orchestrator reconciliation repairs an eligible queued predecessor before recording the normal workflow no-op and releasing queued work; the guarded workflow predicate is unchanged.

The Auto-run acceptance is `TestParentTerminalIdentityReleasesQueuedUserMessage` in the `parentTerminalIdentityFresh` scenario. It queues exactly one ordinary `QueuedByUser` message, supplies the terminal completion effect to `drainQueuedBeforeWorkflowTransition`, and verifies that the controlled provider receives that queued message exactly once, the queue empties, the workflow no-op settles against the original turn, and earlier transcript output is retained. This test does not call `SendQueuedNow`, create a Send Now claim, or enqueue a follow-up prompt; the one pre-existing queue item is dispatched automatically at terminal completion. The manual path remains an independent control, not the fix: the separate `TestSendNowMixedErrandSourcesPreserveOrdinaryInputAndIdentity` test explicitly calls `SendQueuedNow`, and none of the terminal-identity production changes alter that path. The user's observed manual Send Now success is control evidence only.

Additional terminal-identity cases cover repeated reconciliation, missing admission, conflicting effect identity, SQL rollback, stale successor protection, exact callback attribution, and unchanged receipt payload bytes.

## RED/GREEN evidence

The first regression failed before the fix: `TestParentTerminalIdentityReleasesQueuedUserMessage` left the message queued (`red.log`, 1 failure). The next RED run captured the incomplete-effect and lifecycle identity failures (`red3.log`, 3 failures). The expanded pre-fix suite had 5 failures and 1 passing negative control (`red4.log`): fresh dispatch, incomplete-effect repair, payload/callback identity, conflict protection, and SQL rollback were not yet satisfied. The missing-admission protection test already passed.

The final focused race run passed 8 top-level tests across lifecycle and orchestrator, with 0 failures and 0 skips; one lifecycle test has two passing subtests. The broad affected lifecycle/SQLite/orchestrator race selector passed 84 top-level tests, skipped 22 PostgreSQL tests, and had 0 failures. All 22 skips require PostgreSQL, and `KANDEV_TEST_POSTGRES_DSN` was unset.

Commands used the shared build lock and the configured resource bounds (`GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `-trimpath`, `-p 1`, `-parallel 1`, and `-race`):

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'TestParentTerminalIdentity|TestErrandRetainedAdmissionReachesLifecycleConsumer|TestRetrySessionDeliveryWithoutExecutionReplaysRetainedTerminal' -count=1 -v)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'Errand.*(Owner|Ownership|Admission|Terminal)|RetainedTerminal|Durable.*Identity|QueuedDeliveryPredecessor' -count=1 -v)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/persistence/storeconformance -count=1)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go run -trimpath -p 1 ./cmd/sqlguard ./internal)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go build -trimpath -p 1 ./internal/backendapp)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The focused run passed 8 top-level tests; the broad selector passed 84, skipped 22, and failed 0. Store conformance passed in 69.009 seconds. SQL guard and backend build exited 0. `gofmt -l` reported no owned files, `git diff --check` passed, documentation validation reported 440 decisions and 1,954 specifications, and all specification files passed lint.

## Lint and shared-worktree limits

`bash scripts/lint-go-changed` completed its five-minute budget with one finding in `apps/backend/internal/orchestrator/errand_completion_cleanup_test.go`, which belongs to the separate cleanup worker, and then timed out. A direct affected-package diff scan with a twelve-minute timeout completed and reported the same single external QF1008 finding; it reported no finding in the files owned by this packet. The cleanup worker's file was not changed.

The concurrent finished-result phone browser packet completed while this shared worktree was active. Its source audit showed one hash difference only, `apps/backend/internal/orchestrator/parent_terminal_identity_test.go`, matching the test-only lint refactor in this packet; the identity production files were unchanged during that browser run. That browser run is not counted as terminal-identity acceptance, and no browser test was started by this packet.

## Receipt hashes

SHA-256 values identify the retained log contents used for the recorded RED/GREEN and validation results:

| Evidence | Log | SHA-256 |
| --- | --- | --- |
| RED: queued parent message remains held | `/tmp/kandev-parent-terminal-identity-red.log` | `5a43704aa87335388f21ae17e83dd52f1a394db7036503c3f98e9e92a09565d9` |
| RED: identity and incomplete-effect failures | `/tmp/kandev-parent-terminal-identity-red3.log` | `d833ba4fb298ab47863aa5b813c129dcd3110b38581f649869b3f7a0756af3aa` |
| RED: expanded five-failure suite | `/tmp/kandev-parent-terminal-identity-red4.log` | `5e63f004e63254eba027967b5b2619adf735e27ae2fa2d47733c761c707e2474` |
| GREEN: initial focused implementation | `/tmp/kandev-parent-terminal-identity-green2.log` | `00a33547701b20dc32a217c29c82b28d52ff301716972b396ce48bb5bd53aadc` |
| GREEN: lifecycle compatibility correction | `/tmp/kandev-parent-terminal-identity-green3.log` | `ed3a20824a0eba77c84810d1abbfc7f54cbf3dbcaea4e0647b4d8e458a4d6893` |
| GREEN: final focused race suite | `/tmp/kandev-parent-terminal-identity-green5.log` | `3e091745485773efd25ac58f21f724857ac4e238914feb9e2dc5cf616048293b` |
| GREEN: broad affected race suite | `/tmp/kandev-parent-terminal-identity-existing-green-verbose.log` | `cbf9cff40644dac07feffadab5982d0102cfece03462700d38f1fccc441fc75c` |
| Changed-file lint output | `/tmp/kandev-parent-terminal-identity-lint-final.log` | `0a909d79ef761be9dfcf74d9b9df46f80c38c000879481fa6b240088f5fbd13f` |
| Store conformance output | `/tmp/kandev-parent-terminal-identity-storeconformance-final.log` | `492be75bcbbc97a7f03dc6b8f6c6c4cd8d2942eb9d7cdeb6a4d009c20d1e6b60` |

SHA-256 values below identify the implementation/test files at handoff:

| File | SHA-256 |
| --- | --- |
| `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go` | `168a23d90417ddc547b8ef5c6399e5ab3a44af8625cc877dd4943e6164740e53` |
| `apps/backend/internal/agent/runtime/lifecycle/retained_delivery_recovery_test.go` | `e065974d2e72da76ee3a3f11223e5c53089ae9cb717e8aaa8da0c4fc4b921a4b` |
| `apps/backend/internal/agent/runtime/lifecycle/retained_event_ownership.go` | `7c917e8e5672654471b40285815cf4a65838c430030010d39d1273424c772d48` |
| `apps/backend/internal/agent/runtime/lifecycle/streams_events.go` | `27b1f12d8893e80f67e574c8e11ecdedd6555adf97c4eb4d7d4d0ffa24afbf55` |
| `apps/backend/internal/agent/runtime/lifecycle/parent_terminal_identity_test.go` | `d95012fd52f6c242930a198d7b41296840ac37999893e19c204c1ed5275a1f7c` |
| `apps/backend/internal/orchestrator/agent_delivery_settlement.go` | `633b7987e58521c4260d0ba6bf7aac8910daecaea6755b9a7b232194a4790440` |
| `apps/backend/internal/orchestrator/parent_terminal_identity_test.go` | `269dfc4386a0ef09845e2334a17773acdd4f16014f1c47dba3393ed9acfafa6c` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery.go` | `6e468354cd16f6194752f420507198f79a974ab252018959a127e6237e301345` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_admission.go` | `0ecf3a2cb5f7c152b22f5dffb8b0415eb4474e8953264fccd7da733103bc5689` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go` | `ff81477a888b7e05ea1f36b61672713757cdc3cc0e9ec860096a0f7a1c98d145` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go` | `f8ed6e20915d92ab38337ee410a33d93a043a3234a393218f2d8fe9e58769e01` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_terminal_identity.go` | `66b50c67d0083aa89e5db20f50f05e4702fbd1e635dfed44ca8b6ab6ff9d00ef` |

## Handoff

The packet is ready for Era Sol review. Reviewer disposition, integration into the parent feature branch, upstream refresh, final branch checks, and delivery remain parent-owned. This packet did not stage or commit files, push changes, modify a live database/runtime, or use Kandev task/session APIs. At final inspection the shared build lock was free and no packet-owned test/build/lint process remained; disk had 13 GiB free. The worktree remains shared with duration, cleanup, and recovery efforts, so their edits were preserved.
