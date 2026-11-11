# Safe continuation checkpoint handoff

Scope: backend AC-TASKS-ERRANDS-002.7–.8, session `7a28eef3-aff1-4f9f-9108-63ab7fc63aec`, base `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. Accepted attention changes and primary documents are preserved. Implementation and final verification are complete; ownership is released to the primary session. Browser presentation, translations, and desktop/phone acceptance remain outside this checkpoint.

## Behavior

`errands.WSError` and `ReasonOf` expose the existing exact-settlement sentinel as `details.reason=errand_recovery_unresolved`, with conflict status and an actionable explanation. The original error remains available to callers through the existing error chain. Operational errors, including a joined settlement/read failure, are not classified as uncertainty.

Browser and parent MCP handlers validate before message persistence. The production browser wrapper forwards the check to the real orchestrator. Transactional turn-writer refusals also retain the stable transport reason. The repository preflight reuses `db.RequireErrandTurnSettlements` in a read-only transaction; both existing conversation-receipt writers keep their authoritative guards and lock ordering. No universal conversation-write guard was added.

A missing executor record is permitted only for an idle launched Errand with an interrupted result, every relevant completed turn exactly settled, no current open turn, matching retained native identity/generation and a native-resume-capable non-passthrough profile. Restoration pins `RequiredNativeConversationID`; it does not broaden completed-session permissions. Missing/conflicting identity, unsupported profiles, later unresolved work and operational read errors remain fenced.

The actual provider fixture exposed a further admission defect: storage-side recovery can commit the interrupted receipt without clearing this orchestrator's cached original turn. A new explicit prompt then reused that completed turn and correctly hit the immutable settlement identity guard. After exact decisions are reconciled, `retireSettledErrandPrompt` removes only the matching completed Errand cache entry and matching accepted queue marker via compare-and-delete. Active or newer owners are preserved. Receipt and immutable submission guards remain unchanged.

## Acceptance evidence

The real SQLite/task-service/browser/authenticated-parent MCP/orchestrator/lifecycle/provider fixture now passes for both origins (67985, race, backendapp 2.135s). Repeated refused requests return the stable reason without advancing conversation revision or changing turns. The same fixture first reaches the external provider with the original instruction, proves native recovery/interrupted settlement, and then reaches it with only a newly requested follow-up. The prior uncertain submission state, payload/hash and result revision/text remain unchanged; workflow step remains unchanged. Only the external provider/process boundary is simulated.

Missing-executor eligibility is separately exercised through the real SQLite-backed `loadResumeSessionCandidate`, including unsupported provider, conflicting native identity and later open work. Existing lifecycle native-only restoration tests verify rejection of replacement conversations. No live incident repair or deployment is claimed.

The provider fixture now persists the replacement executor binding, as production does. Its positive continuation uses the explicit direct-prompt API with trusted parent provenance where applicable; the fixture does not subscribe to terminal lifecycle events. Queue regression coverage remains in the orchestrator suite. Temporary diagnostic wrappers were removed after proving the cached-turn conflict.

## Checks

All heavy commands are serialized with disk-backed `/tmp/kandev-errand-continuation-checkpoint`, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `-trimpath` and `-p 1`. Compiler memory exceeded the Go soft limit, so actual shared-cgroup use was monitored; no concurrent checks or live-service mutations occurred. Root retained 21–23 GiB free during final verification. `KANDEV_TEST_POSTGRES_DSN` is unset; PostgreSQL runtime cases are skips.

- Behavioral RED: 80383 (missing refusal projection), 45746 (browser accepted write and missing executor rejection), 31458 (exact preflight unavailable and joined operational error misclassification), 45389 (MCP reason missing), 30401 (browser adapter wiring and later-open-turn exception), 13194 (exact cached original turn reused for a new submission). Initial fixture compile error 45364 is not RED evidence.
- 14762: focused race passed Errands, SQLite, orchestrator, task handlers and MCP; backendapp compile failed because a new helper was added after the command collected its file list. Superseded by the fresh provider run.
- 16305: selected orchestrator (3.204s), task handlers (1.407s), MCP (1.670s), SQLite (3.038s), Errands (1.132s), lifecycle (2.397s) passed. Backendapp's queue-fixture timeout led to the direct-prompt acceptance test and subsequent cached-turn diagnosis; this is not a whole-command pass.
- 42578: scoped lint hit the script's five-minute package-loading timeout, exit 123. 35473 completed the same scope with a 15-minute timeout and found one nesting violation; the handler helper now removes it. Final scoped `golangci-lint` over backendapp, errands, MCP handlers, orchestrator, orchestrator handlers, task handlers, and SQLite returned `0 issues`.
- Final focused race checks passed for stable refusal classification, SQLite exact-receipt preflight, user and authenticated-parent refusal before writes, missing-executor continuation eligibility, queue refusal/dispatch, and the actual provider continuation fixture for both user and parent origins (backendapp 2.125s).
- The saved regression run 60840 failed only `TestSendNowErrandCanonicalPersistenceFailurePreventsProviderIO/durable_v1/established/claim`, with the existing queue assertion expecting two messages but observing one; all other selected packages passed. The exact subtest reproduces with the saved baseline overlay, while the isolated current-source subtest passes. This intermittent unrelated queue failure was not changed or counted as a passing whole run.
- Final SQL guard passed; `go test -race ./internal/persistence/storeconformance -count=1` passed in 66.845s. PostgreSQL runtime coverage was skipped because the DSN is unset.
- Final `gofmt -l` over the continuation Go file list and `git diff --check` produced no output. No production source changes were made during final verification.

## Frontend refusal contract

WebSocket error `code` is the existing conflict code; `details.reason` is exactly `errand_recovery_unresolved`. The explanation states that earlier work/external effects remain uncertain, recovery must verify them before continuation, and conversation inspection/cancellation remain available without implying cancellation resolves uncertainty. Localize this reason on existing send surfaces. This reason is distinct from checkpoint 1's `attention_reason=recovery_unresolved` and parent delivery uncertainty.

## Exact write set

```text
apps/backend/internal/backendapp/adapters_errand_continuation.go
apps/backend/internal/backendapp/adapters_errand_continuation_test.go
apps/backend/internal/backendapp/errand_continuation_transport_test.go
apps/backend/internal/backendapp/errand_recovery_delivery_test.go
apps/backend/internal/errands/errors.go
apps/backend/internal/errands/recovery_error_test.go
apps/backend/internal/mcp/handlers/errand_continuation_test.go
apps/backend/internal/mcp/handlers/handlers.go
apps/backend/internal/orchestrator/errand_continuation.go
apps/backend/internal/orchestrator/errand_continuation_test.go
apps/backend/internal/orchestrator/errand_recovery_interruption_test.go
apps/backend/internal/orchestrator/task_operations.go
apps/backend/internal/orchestrator/handlers/queue_error_responses.go
apps/backend/internal/orchestrator/handlers/queue_handlers.go
apps/backend/internal/orchestrator/handlers/queue_operations_handlers.go
apps/backend/internal/orchestrator/handlers/queue_errand_continuation_test.go
apps/backend/internal/task/handlers/message_handlers.go
apps/backend/internal/task/handlers/message_handlers_recovery_refusal_test.go
apps/backend/internal/task/repository/sqlite/errand_continuation.go
apps/backend/internal/task/repository/sqlite/errand_continuation_test.go
apps/backend/internal/task/handlers/message_handlers_errand_continuation.go
apps/backend/internal/orchestrator/errand_terminal_admission.go
docs/plans/errand-orphan-settlement/continuation-handoff.md
```

No frontend, commit, push, shared-state Git operation, persistent platform task/session, live database, cancellation, restart, configuration or deployment mutation was performed. The accepted checkpoint 1 files and primary plan remain outside this write set.

## Independent review

Era Sol review `fe59bc44-2917-4157-a54b-b5ae5c732e90` found no actionable production issues in the continuation/refusal snapshot. It inspected browser, queue and authenticated-parent admission, stable refusal/error preservation, exact receipts, missing-executor native restoration and cached-turn retirement. Completed-session permissions remain unchanged. Limits: positive provider continuation uses direct PromptTask; compare-and-delete retirement has no dedicated deterministic successor-race test; PostgreSQL remains skipped and the Send Now baseline failure remains documented. Reviewer ran no checks. Primary independent race handle91849 passed after acquiring the shared lock: orchestrator1.832s/backendapp2.260s, including refusal and actual provider continuation. Primary accepts continuation within the documented fixture limits.
