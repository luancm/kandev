# Immutable Errand consumer ownership

Status: consumer slice implemented and verified. Writer ownership is released; no check handles remain running. Existing changes were preserved. Parent owns integration and contracts. No live state, index, commit, producer or parent-delivery changes. This is not whole-task or historical-incident acceptance.

Confirmed the replaceable running binding is the only prompt-generation admission record. Inbox admission checks transport ownership; canonical projection trusts explicit turns or selects unrelated open work. Implementation will retain general admission identity independently of interruption evidence in the existing durable effect store, before dispatch, and enforce it for Errand inbox/projection. Retained admission is never terminal or process-death evidence.

Initial resources: root 42 GiB free; RAM scratch 6.9 GiB free. Tests use the supplied GOTMPDIR and bounded Go settings. Verification pending.

## Implemented and checked so far

`agent_delivery.prompt_admission.v1` is a distinct sequence-zero effect containing task, session, execution, incarnation, harness generation, prompt generation, stream, submission, turn and optional native conversation. The running-row transaction retains it before provider dispatch. Equal retries preserve it; conflicting same-submission bindings roll back the running-row write. Running-row replacement/deletion and event compaction do not remove it; session cleanup owns its lifetime. Legacy bindings without prompt generation or carrying a predecessor execution are not promoted into admission proof. This format does not confer death or terminal authority.

Errand inbox acceptance and canonical/generic projection validate retained admission. Missing/conflicting ownership fails closed. Late output projects to the original completed turn without reopening it. Real unowned output gets a separate turn with typed unowned provenance. Ordinary sessions keep their existing repository projection rules. Lifecycle supplied-submission fallback and explicit turn/generation conflict probes are now permanent tests.

Commands so far used `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1`:

- Handle 23677, SQLite `-run '^TestErrandAdmissionOwnership$' -count=1`: assertion-quality RED, retained admission count expected 1, got 0.
- Handle 88632, same selection: GREEN 0.127s.
- Handle 43442, lifecycle `-run 'TestErrandUnknownSubmission|TestErrandExplicitConflicting' -count=1`: both probes RED, incorrectly selected successor.
- Handle 99024, lifecycle + SQLite ownership/native-async compatibility selection: GREEN 0.100s / 0.185s.
- Handle 54738, expanded SQLite ownership regression: RED at terminal projection, expected original turn, got empty. Earlier assertions passed, including replacement/deletion, conflicting identity, rollback, native activity isolation and duplicate receive.
- Handle 37444, `-race ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'Test(Postgres)?ErrandAdmissionOwnership|TestErrandUnknownSubmission|TestErrandExplicitConflicting|TestDurableEventKeepsExplicitDispatchOwnership|TestNativeAsync|TestRecoveredLiveCompletion|TestErrandInterruptedDelegation' -count=1`: GREEN SQLite 1.998s, lifecycle 10.407s. PostgreSQL selected but DSN-dependent.

Further inspection found callback ownership and active-submission mutation need retained resolution too. A real SQLite restart/journal replay regression is being added. Its first run (36242) hit a fixture workspace omission during schema reopen, not an assertion-quality RED; fixture correction is running as handle 7757. No final completion claim yet.

## Consumer completion details

Live intake and stored-inbox replay now resolve callback ownership through the repository's optional `ResolveAgentDeliveryEventTurn` capability. The real backend supplies the repository directly, so no backendapp wiring change is required. Resolution does not rewrite received bytes; the writer repeats validation before accepting the inbox. Supplied submissions bypass mutable lifecycle turn inference, including when a restarted lifecycle reuses a prompt-generation number. Late output cannot replace the successor's active submission. A proven historical terminal is projected but cannot invoke the successor's completion callback.

Unowned native work during a pending successor remains accepted work. Its turn uses immutable source protocol/control identity, or the exact stream sequence when grouping evidence is absent, plus session/incarnation/harness/stream provenance. Canonical fields reconstructed by lifecycle are not ownership evidence. Typed `agent_delivery_unowned_v1` metadata records the source, and projected Errand message IDs are scoped to the resolved turn. This is not lifecycle-only classification.

The permanent lifecycle regression uses the actual typed binding producer and running-row writer, closes/reopens SQLite, replaces/deletes mutable running metadata, appends/replays a real journal event, and drives live preparation plus stored-inbox projection/callbacks. It covers same-number generation reuse, missing-generation historical terminal suppression through the real processor, native activity during the pending successor, transcript ownership and active-submission preservation. It does not repeat the independently accepted process producer tests or parent-provider integration.

Additional execution evidence:

- 7757: corrected SQLite restart fixture reached assertion-quality RED (callback turn empty instead of original); 13670 GREEN 0.154s after resolver wiring.
- 75322: RED native activity during pending B was rejected after inheriting B; 17608 race GREEN SQLite 8.372s / lifecycle 10.930s after separating unowned activity.
- 73096: SQLite lint found two complexity violations and one predicate simplification; refactored.
- 40632: same-generation successor test RED at identity validation; 70358 race GREEN SQLite 8.444s / lifecycle 10.944s after eliminating supplied-submission inference and guarding active submission by resolved turn.
- 66169: RED, reconstructed canonical fields changed unowned replay turn; 1887 race GREEN SQLite 8.487s / lifecycle 10.849s after using producer provenance and scoping message IDs.
- 89746: RED, old terminal callback was not suppressed; 75603 lifecycle race GREEN 10.799s after suppression. The test also executes the actual prepared-event processor and asserts no completion callback.
- 80351: SQLite lint found one nested-block complexity violation; extracted the canonical Errand resolution helper. Post-refactor race handle 75604 is the final-source run, pending at this checkpoint.

Write set (only this slice): lifecycle `types.go`, `native_async_turn_identity_test.go`, `durable_delivery_stream.go`, `streams.go`, `received_delivery_projection.go`, new `retained_event_ownership.go` and `errand_consumer_ownership_test.go`; SQLite `executor.go`, `agent_delivery.go`, `agent_delivery_projection.go`, new `agent_delivery_admission.go` and `errand_admission_ownership_test.go`; this handoff. All other existing changes were preserved. No schema migration, agentctl API, frontend, contract, index or live-state changes.

## Final boundary corrections

Native ACP payload `session_id` is a native conversation ID, not necessarily the Kandev session ID in the authenticated delivery envelope. The admission record now also retains optional `native_session_id`; nonempty native payload IDs must match that exact retained native conversation, or the exact SQL harness-generation native identity when no native identity was retained. Unowned native activity uses the latter source proof. Envelope task/session/incarnation/harness/stream checks remain independent. The permanent journal/reopen fixture now uses this actual ACP payload shape for original output, terminal and native activity.

Errand observations no longer assign the mutable active submission at all; admission/recovery owns that field. Completion claim checks submission and known turn ownership while holding `promptLifecycleMu`, in addition to generation checks. This closes the gap between intake validation and a later completion callback. The new `TestRetainedTerminalCannotClaimSuccessorWithReusedGeneration` rejects both the old submission and an explicit different turn, then positively claims the real current submission. `manager_events.go` is the only additional write-set file.

Check history before these final corrections: 75604 race GREEN (SQLite 8.442s / lifecycle 10.947s); 27765 SQLite lint and 96125 lifecycle lint both exit 0, zero issues; 12646 SQL guard exit 0; 93314 store conformance race GREEN 62.571s. Native-session regression 58514 then failed at real intake with identity mismatch; 81857 race GREEN (SQLite 8.620s / lifecycle 10.964s) after distinguishing native and Kandev identity. Completion-claim regression 64189 failed because the old terminal claimed the successor; final regression/check results follow below.

## Final results and handoff

Implemented the bounded consumer slice. There are no remaining source changes or active checks for this writer. All write-set files remain uncommitted; no Git index operation, live repair, restart, deployment or parent-checkout mutation occurred.

The shared completion regression initially failed at `TestHandleAgentEvent_CompleteConsumesCausingRunIDAfterReady` (62091). The final guard preserves numbered legacy completions only when no submission or turn binding is known and no turn is supplied; this does not bypass Errand SQL admission validation. Combined lifecycle regressions passed afterward (42638, 11.323s).

Unknown/evicted producer generations with no submission are genuine unowned activity, even if lifecycle still has a generation-to-turn cache entry. Regression 38280 failed because such activity was rejected. Intake now probes the original producer event and removes only lifecycle-inferred turns from the new Errand inbox payload before acceptance; replay never rewrites stored bytes. The fixture deliberately supplies a misleading local cache entry. Race run 8759 passed SQLite 8.838s and lifecycle 11.282s. Regression 91891 then proved an unowned numbered terminal could still claim an admitted prompt; the final intake guard retains/projects that terminal while suppressing its prompt-completion callback. Existing unnumbered async completion behavior remains supported. Final lifecycle race run 63430 passed 11.034s, including the actual prepared-event processor, shared completion handlers and lock-order regression.

Exact final regression commands (repository root):

```sh
GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'Test(Postgres)?ErrandAdmissionOwnership|TestErrandRetainedAdmission|TestRetainedTerminalCannotClaim|TestErrandUnknownSubmission|TestErrandExplicitConflicting|TestDurableEventKeepsExplicitDispatchOwnership|TestNativeAsync|TestRecoveredLiveCompletion|TestErrandInterruptedDelegation|TestInbox|TestCanonical|TestHandleAgentEvent_.*Complet|TestHandleCompleteEvent|TestMarkReadyDoesNotDeadlockWithPromptCompletion' -count=1
GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/agent/runtime/lifecycle -run 'TestErrandRetainedAdmission|TestRetainedTerminalCannotClaim|TestErrandUnknownSubmission|TestErrandExplicitConflicting|TestDurableEventKeepsExplicitDispatchOwnership|TestNativeAsync|TestRecoveredLiveCompletion|TestErrandInterruptedDelegation|TestInbox|TestCanonical|TestHandleAgentEvent_.*Complet|TestHandleCompleteEvent|TestMarkReadyDoesNotDeadlockWithPromptCompletion' -count=1
```

Final checks ran serially in execution cell 174; each process was polled to exit 0:

- SQLite lint: handle 89051, zero issues. `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery_admission.go apps/backend/internal/task/repository/sqlite/agent_delivery.go apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go apps/backend/internal/task/repository/sqlite/executor.go apps/backend/internal/task/repository/sqlite/errand_admission_ownership_test.go`.
- Lifecycle lint: handle 81597, zero issues. `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/types.go apps/backend/internal/agent/runtime/lifecycle/native_async_turn_identity_test.go apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go apps/backend/internal/agent/runtime/lifecycle/streams.go apps/backend/internal/agent/runtime/lifecycle/received_delivery_projection.go apps/backend/internal/agent/runtime/lifecycle/retained_event_ownership.go apps/backend/internal/agent/runtime/lifecycle/errand_consumer_ownership_test.go apps/backend/internal/agent/runtime/lifecycle/manager_events.go`.
- SQL guard: handle 37030, passed. `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend run -trimpath -p 1 ./cmd/sqlguard ./internal`.
- Store conformance: handle 42123, passed with race detector, 62.044s. `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/persistence/storeconformance -count=1`.

PostgreSQL behavior selection includes `TestPostgresErrandAdmissionOwnership`, sharing the real repository exercise, but `KANDEV_TEST_POSTGRES_DSN` is unset. PostgreSQL runtime/locking parity is not claimed. No full-suite or orchestrator acceptance run is claimed.

Remaining parent-owned work: actual child-dispatch-to-parent-provider integration, public uncertain-status/explicit-resume behavior, full stream warning coverage, contract/documentation reconciliation and whole-work-order acceptance. Historical incident repair remains unproven; old records lacking exact ownership stay fenced. The retained admission record is not death, interruption or provider-completion evidence. No live mutation was attempted.

Final whitespace check: `git diff --check` passed. Final available space: root 32 GiB; RAM scratch 6.9 GiB. No shared cache or unowned scratch was removed.
