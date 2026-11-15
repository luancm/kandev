# Recovery audit remediation

Current state: both accepted blockers are repaired and ready for primary inspection. Final-source focused race, all three changed-Go lint groups and SQL guard pass. PostgreSQL fixtures were selected but skipped for missing DSN. No slice-3 or live-incident claim.

Bounded remediation of the two accepted slice-2 audit findings. Existing implementation and primary-owned documents are preserved. No live runtime/database operations, commits or delegation.

## Terminal supersession

RED: extended TestErrandInterruptedDelegationTerminalEvidenceWins through completed turn, real Errands result reconciliation with Manager.ReconcileInterruptedErrandDelivery wired, pending/unsettled inspection, replay and successor reservation. Command from apps/backend: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 ./internal/agent/runtime/lifecycle -run TestErrandInterruptedDelegationTerminalEvidenceWins -count=1`. Handles 81225 and 41018 both failed with “native resume journal work remains unresolved” after real result settlement. An initial path typo accidentally launched the command before the test edit; both handles were retained and finished. These runs overlapped inadvertently; subsequent checks are serialized.

Change: exact terminal receipt transaction resolves the assessment as block_resolved, preserving the original JSON proof. Resolution requires one terminal, exact task/session/incarnation/generation/submission/turn/stream, payload hash, positive terminal sequence and resolved/projected terminal authority; adopted SQL terminal coordinates must match. Reconciliation settles existing terminal receipts before attempting native recovery. No provider outcome is synthesized.

GREEN in progress, handle 77935, same command. Resources before: root 2.8 GiB, RAM scratch 9.7 GiB free, memory available 13 GiB. During checks: root 2.6 GiB, scratch 9.7 GiB free, memory available 11 GiB.

## Process inspection

Pending regression and repair. Existing processidentity functions use real OS inspection; no injection seam exists. The planned regression will isolate an actual OS read failure in a subprocess and exercise real repository preservation through lifecycle replacement persistence.

## Initial remaining work (completed below)

Both repairs, expanded supersession rejection/rollback coverage, final focused race and lint, PostgreSQL selection. No incident eligibility or parent-provider delivery claim.

Process-proof RED: handle 82045 ran `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 ./internal/agent/runtime/lifecycle -run 'TestErrandInterruptedDelegation(TerminalEvidenceWins|InspectionFailurePreservesBinding)' -count=1`. The isolated subprocess first proved the original process dead, then exhausted its file descriptors at metadata serialization immediately before the second OS check. Real lifecycle replacement persistence returned nil instead of EMFILE. It uses the existing running-reader interface and real SQLite preservation, with no production injection API or shared global failpoint.

Change: propagate operational owner-inspection errors with context; explicitly retain unsupported/unverifiable historical proof as non-authoritative and preserve ordinary-role behavior. Final retry must save the assessment before replacement.

Terminal expanded run: unknown, adopted and rollback paths passed in handle 82045. Stream/generation/hash negative cases passed. Ambiguous fixture had a positional SQL column mistake; fixed to explicit column names. Handle 77935 had passed settlement/state assertions but exposed a missing successor-turn fixture, now created through the real guarded CreateTurn API.

Combined GREEN running as handle 79283. Root 2.2 GiB free, scratch 9.5 GiB free, available memory 12 GiB before this run.

GREEN: handle 79283 passed both regression groups (1.348s), including real EMFILE preservation/retry, SQL unknown and SQL adopted terminal, exact mismatch negatives, ambiguity, rollback, replay and real CreateTurn/reservation successor gate. Additional cancellation case passed in handle 82955 (0.146s), using the same environment and `-run 'TestErrandInterruptedDelegationTerminalEvidenceWins/cancelled'`. Cancellation converges through the receipt-only fallback, retaining discarded state.

Final race started as handle 91071 from apps/backend: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/errands ./internal/orchestrator -run 'ErrandInterruptedDelegation|NativeResume|ErrandHistoricalReceipt|ErrandRestartAbandonmentSettlesExactInterruptedTurn|ErrandExactTerminalDecisionPreexistingSuccessor' -count=1`. Before: root 2.0 GiB free, scratch empty (9.7 GiB free), available memory 13 GiB. Source frozen during checks.

Final race handle 91071 exited 0: lifecycle 27.192s, SQLite 16.869s, Errands 1.328s, orchestrator 4.183s. The orchestrator build dominated elapsed time. Resource monitoring stayed above the stop thresholds; root space independently increased to 7.4 GiB during the run (this Errand deleted no shared files), later 7.1 GiB. Peak observed scratch usage was 545 MiB and available memory remained 5.3 GiB or more.

Retention inspection: ordinary event retention only deletes canonical agent_delivery.event receipts; the assessment survives under its distinct key/type, including block_resolved. Explicit session retirement still purges all owned effects as before. Supersession does not promise preservation beyond session retirement.

SQLite changed-Go lint running as handle 9344: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOLANGCI_LINT_CACHE=/dev/shm/kandev-errand-remediation-0n6s5t7i/lint-cache GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/errands_interrupted_delegation.go apps/backend/internal/task/repository/sqlite/errands_interrupted_supersession.go apps/backend/internal/task/repository/sqlite/errands_turn_settlement.go apps/backend/internal/task/repository/sqlite/errands_historical_receipt.go`.

Lint 9344 found cyclomatic limits (supersession 16, current receipt commit 17). Extracted the exact-owner predicate and completed-receipt read path; SQLite rerun 50908 passed with 0 issues. Subsequent final identity inspection added an adopted SQL terminal-turn mismatch case. RED handle 51735 (`-run 'TestErrandInterruptedDelegationTerminalEvidenceWins/adopted_turn'`, non-race, same environment) showed an incorrect successful settlement. The query now requires SQL terminal_turn_id to match the terminal effect, or to be empty for interrupted_unknown. Race handle 63864 then passed lifecycle 26.913s, SQLite 21.319s, Errands 1.488s with `-run 'ErrandInterruptedDelegation|NativeResume|ErrandHistoricalReceipt|Errand.*(Settlement|Terminal)'`.

A pre-existing committed receipt needed its own retry path: RED handle 62502 (`-run 'TestErrandInterruptedDelegationTerminalEvidenceWins/receipt_replay'`, non-race) reproduced unresolved native work with a completed real receipt and pending assessment. Added ReconcileErrandInterruptedTerminalReceipts under the same task/session locks and exact validation, called by the production lifecycle recovery callback before requesting native proof. This path changes only the assessment state, never the existing result, publication or notification. The test asserts zero publication/notification, preserved existing result text, replay and successor admission.

Final-source race handle 3669 is running: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/errands -run 'ErrandInterruptedDelegation|NativeResume|ErrandHistoricalReceipt|Errand.*(Settlement|Terminal)' -count=1`. Root before the added regression was 5.5 GiB free, scratch 9.7 GiB free, available memory 13 GiB.

Final-source race 3669 passed: lifecycle 27.060s, SQLite 21.966s, Errands 1.498s. This covers every new/changed regression, including adopted terminal-turn rejection and already-committed receipt repair. Orchestrator race 91071 passed before the final repository retry addition; no orchestrator source was changed by this remediation.

Final SQLite lint is handle 50948, using the exact four-path command above (9344), after all production edits.

Final SQLite lint 50948 passed, 0 issues. Lifecycle lint 20400 passed, 0 issues: same environment and script with explicit paths `apps/backend/internal/agent/runtime/lifecycle/errand_interrupted_delegation.go apps/backend/internal/agent/runtime/lifecycle/errand_interrupted_result_test.go apps/backend/internal/agent/runtime/lifecycle/errand_process_inspection_test.go`. Errands lint 94971 is running with explicit path `apps/backend/internal/errands/terminal_decisions.go`.

## Remediation write set

- apps/backend/internal/task/repository/sqlite/errands_interrupted_delegation.go: propagate operational inspection errors, explicitly exclude unsupported identities.
- apps/backend/internal/task/repository/sqlite/errands_interrupted_supersession.go (new): exact atomic supersession and completed-receipt retry.
- apps/backend/internal/task/repository/sqlite/errands_turn_settlement.go: resolve assessment within current receipt commit/replay under session locks.
- apps/backend/internal/task/repository/sqlite/errands_historical_receipt.go: same resolution in historical receipt commit/replay.
- apps/backend/internal/errands/terminal_decisions.go: reconcile available terminal receipts before native recovery, then settle recovery-produced decisions.
- apps/backend/internal/agent/runtime/lifecycle/errand_interrupted_delegation.go: retry completed real receipts before requesting native evidence.
- apps/backend/internal/agent/runtime/lifecycle/errand_interrupted_result_test.go: real result, callback, mismatch, adoption, cancellation, rollback, committed-receipt replay and successor cases.
- apps/backend/internal/agent/runtime/lifecycle/errand_process_inspection_test.go (new): isolated real OS failure through replacement persistence and successful retry.
- docs/plans/errand-orphan-settlement/recovery-audit-remediation.md: this log only; no other documentation was edited by this Errand.

The inspection regression exercises the actual lifecycle binding-replacement path after an initial successful real death check, rather than directly invoking source-row deletion. Journal retirement and the original row remain unchanged on the failed attempt. No production test-only export or shared process-inspection global was introduced.


## Final validation and limits

- Final-source race: 3669, exit 0 (lifecycle 27.060s, SQLite 21.966s, Errands 1.498s). All new and changed regressions ran after the final production change.
- Earlier orchestrator focused race: 91071, exit 0 (4.183s), before the final completed-receipt retry addition. No orchestrator files changed in this remediation.
- Changed-Go lint: SQLite 50948, lifecycle 20400, Errands 94971; each exit 0, 0 issues. Paths and environment are recorded above.
- PostgreSQL: 43816, exit 0 with every selected runtime fixture SKIPPED for unset KANDEV_TEST_POSTGRES_DSN. Command from apps/backend: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 -v ./internal/task/repository/sqlite -run 'TestPostgresErrandInterruptedDelegationStorage|TestPostgresErrandHistoricalReceipt' -count=1`. No PostgreSQL runtime parity claim.
- SQL guard: 79006, exit 0. Command from apps/backend: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go run -trimpath -p 1 ./cmd/sqlguard ./internal`.
- No commits, pushes, PRs, persistent task/session operations, live database/runtime/config operations or recursive delegation. Existing changes and primary documentation were preserved.
- No incident-eligibility or actual parent-provider-delivery claim. Missing historical proof still fences. Supersession retains original assessment JSON in block_resolved alongside the authoritative receipt until normal explicit session retirement.

Final cleanup: all check handles finished before removing only the contents of the task-owned RAM scratch directory. Scratch is empty (9.7 GiB free); root has 3.6 GiB free and available memory is 13 GiB. Shared caches and other tasks' scratch were not deleted. Final git diff --check passed.
