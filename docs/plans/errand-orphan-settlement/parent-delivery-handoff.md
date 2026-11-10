# Actual parent provider acceptance

Status: IMPLEMENTED AND VERIFIED. Final-source focused race and changed-Go lint pass. The permanent backendapp integration regression now crosses actual child dispatch, original-owner death, native recovery, exact interrupted settlement, persistent notification admission, delayed parent readiness and actual parent provider dispatch. No production change was necessary. Earlier investigation-only status is superseded by this implementation.

## Write set and boundaries

- `apps/backend/internal/backendapp/errand_recovery_delivery_test.go`: `TestErrandRecoveryActualProviderDelivery` composes real SQLite task services, `orchestrator.NewService`, `newLifecycleAdapter`, `taskRepositoryAdapter`, `messageCreatorAdapter`, turn service, SQLite message queue, `NewErrandAdapter`, `NewErrandParentDeliverer`, `errandTranscript`, Errand controller and interrupted-delegation reconciler.
- `apps/backend/internal/backendapp/errand_recovery_transport_test.go`: authenticated HTTP/WebSocket external transport fixture with a real `journal.Journal`; captures `agent.prompt` text/submission/prompt generation and retains canonical prompt payload/hash. Owned Linux sleep processes supply supported process identity through real `agentctl.RuntimeOwner` binding. All clients, processes, servers and journals are test-owned and cleaned up.
- This handoff only. Existing dirty production changes and other documentation remain untouched; no index, commit, live-state or parent-checkout mutation.

The scenario begins with an existing idle Errand and a real parent-origin follow-up queued through server-owned sender metadata. Initial session/Errand records and committed harness identity are fixture setup, not claimed coverage of initial Errand creation or executor provisioning. Synthetic execution registration uses the existing lifecycle test seam; initialization and native loading use real transport requests through `SessionManager.InitializeAndPrompt`. The replacement is separately initialized and its readiness is persisted through `Manager.UpdateStatus`, which exercises assessment preservation before running-row replacement. The journal simulates the external provider's interrupted state after the owned original process has been killed and reaped. SQL submission preparation/admission/dispatch/recovery transitions are entirely production-driven; no admitted-state shortcut or fake parent deliverer is used. The journal has no output events, so complete projection is the empty-stream case; warning/output replay belongs to existing separate coverage.

## Permanent assertions

The actual child prompt has a nonempty submission ID and prompt generation. SQLite retains its immutable admission effect and exact original runtime PID, turn, execution and submission binding. Parent admission advances and its queued message drains before recovery begins. Exact original-owner death followed by authenticated native recovery produces one completed `parent:interrupted` settlement and a durable failed/interrupted result for the admitted turn. SQL and journal remain `interrupted_unknown`, journal retirement is recorded, and SQL payload/hash remain unchanged with no synthetic terminal sequence or stream.

The parent initially has no ready runtime. Real parent delivery persists one notification, and another delivery sweep retains exactly that entry. After initializing the parent and marking its session ready, public queue-readiness admission dispatches the result. The captured provider prompt contains exactly one complete system block and the full interruption section. Its text equals both the canonical parent submission payload and persisted parent message. The committed delivery claim binds the correct Errand, parent, result revision and message token. Repeated native recovery, sweep and drain leave no duplicate notification or provider request and never replay the child prompt. The task's workflow step remains unchanged.

## Verification evidence

Commands run from the repository root unless noted. All builds were serialized with `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB` and `go -C apps/backend test -trimpath -p 1`.

- Initial fixture compilation succeeded; setup iterations exposed the correct registry ID, required turn-service wiring, provider liveness response, stream identity and replacement readiness persistence. These were fixture corrections, not production defect RED evidence.
- First actual child dispatch: focused `./internal/backendapp -run TestErrandRecoveryActualProviderDelivery -count=1`, handle 19548, PASS 0.166s.
- Full child-death/recovery/parent-provider chain: same focused command, handle 40450, PASS 0.258s.
- Complete parent-block, canonical payload, message and result-revision assertions: same focused command, handle 74408, PASS 0.261s.
- Final source adds explicit supported birth identity, exact settlement receipt, unchanged payload/hash and absence of synthetic terminal identity. Focused `-race ./internal/backendapp -run TestErrandRecoveryActualProviderDelivery -count=1`, handle 60793, PASS 1.553s, exit 0; output is `/tmp/errand-provider-race.log`.
- Changed-Go lint: `bash scripts/lint-go-changed apps/backend/internal/backendapp/errand_recovery_delivery_test.go apps/backend/internal/backendapp/errand_recovery_transport_test.go`, same resource environment, handle 19001, exit 1: three ST1013 findings for numeric HTTP 409 in the transport fixture. Replaced all three with `http.StatusConflict`; no production change. Final focused race rerun, handle 55409, PASS 1.553s, exit 0 (`/tmp/errand-provider-race-final.log`). Final lint uses the same command and file list, handle 54425, PASS, exit 0, zero issues (`/tmp/errand-provider-lint-final.log`). Initial lint output is `/tmp/errand-provider-lint.log`.

Owned-file whitespace checks (including untracked files) pass. Final headroom: root 28 GiB and tmpfs 6.5 GiB or more free. All command handles completed; no checks remain running.

## Limits

No historical incident eligibility claim, real user-provider request, initial Errand launch/provisioning claim, public uncertain-state/explicit-resume UI coverage, or full warning-stream coverage. Cancellation/newer-result permutations remain with the accepted focused suites and are not duplicated here. No transaction or schema changed, so SQLguard, store conformance and PostgreSQL fixtures are not newly required by this test-only write set. No full-backend suite was run, and the previously documented baseline Send Now failures were not rerun. This closes only the delegated provider-boundary acceptance concern; the overall work order is not marked complete.
