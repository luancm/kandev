# Recovery attention checkpoint handoff

Scope: AC-TASKS-ERRANDS-002.4–.6 backend projection, reason propagation and capacity parity. Base remains `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. Session `e4106ac7-5637-458d-b831-b8a0e6cc8959` owns only the Go files below and this handoff. The pre-existing `plan.md` edit belongs to the parent and was preserved.

Implementation and scoped verification are complete. Ownership is released to the parent. All command handles are complete; no checks or writers remain running from this session.

## Behavior and evidence

`Repository.ErrandRecoveryAttention` and live/outstanding SQL summaries share the same read-only predicate. A launched, idle, nonfailed session needs a matching typed `AgentDeliveryRecovery` snapshot, completed non-lifecycle work, retained admission identity (including the historical first-turn reservation), and missing exact settlement. An older result cannot mask later unresolved work; a newer authoritative result remains authoritative. Awaiting flags, open turns and runtime IDs alone do not qualify. No new interruption-assessment effect is required for historical rows with sufficient retained recovery identity.

Startup, active work/reconnection, lifecycle-only activity, stale session/incarnation/submission identity, open turns, completed receipts and ordinary terminal reconciliation are excluded. Ordinary reconciliation includes both a uniquely sealed settlement intent and exact projected terminal evidence before intent sealing. Unprojected/stale terminal evidence, conflicting outcomes and duplicate ambiguous receipts do not suppress attention. Cancellation, queued, missing-session and failed-session precedence is unchanged. Permission attention remains specific to active permission requests.

`Service.Project`, `ProjectSnapshot` and `List` use one classifier. Repository/evidence errors propagate through service reads. Parent outstanding-work checks and single/batched SQL summaries retain unresolved work even when `awaiting_result` is false. Result text, revision, state and delivery data are not rewritten. Reads do not settle work, infer provider outcomes or replay prompts. Admission fences, receipt writers and continuation eligibility were not changed.

The batch-summary query-budget tests now count actual driver requests through the existing test wrapper. SQLite authorizer callbacks count nested SELECTs too, so the old counter reported 30 callbacks for two database requests. The unchanged one/two-query assertions and second-request failure coverage now observe the intended boundary.

## Additive shape for the next frontend checkpoint

Optional `attention_reason` has the closed values `permission` and `recovery_unresolved`, emitted only with `needs_attention`. It remains distinct from parent-receipt `delivery_state=uncertain`.

- Internal `errands.Snapshot.AttentionReason`, with `ProjectSnapshot` available to callers needing both status and reason.
- `ErrandDTO.attention_reason`, including task Errand responses and `errand.updated` events.
- MCP session entry `attention_reason`, alongside `errand_status`.
- MCP start response `attention_reason` when present; await response `errands[].attention_reason`.
- Parent delivery envelope `items[].attention_reason`; parent status text includes the reason and an explanation of uncertain earlier work/external effects, with conversation inspection and cancellation mentioned.
- Await timeouts retain `needs_attention` and its reason rather than flattening attention to `pending`.

Older permission-only producers remain compatible. The production bus publisher supports the full snapshot while existing status-only publisher implementations retain their interface. No frontend files changed. Refusal/resume behavior (.7/.8), browser presentation, translations and desktop/phone acceptance remain the next checkpoints.

## Verification

All heavy checks were serialized, using disk-backed `/tmp/kandev-errand-attention-checkpoint`, `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, `-trimpath` and `-p 1`. No tmpfs build scratch, live database/runtime mutation, Git mutation, commit or delegation occurred. Disk remained above 37 GiB available; memory headroom remained healthy.

Commands below run from the repository root with `GOTMPDIR=/tmp/kandev-errand-attention-checkpoint GOMAXPROCS=2 GOMEMLIMIT=3GiB`.

| Handle | Command | Result |
| --- | --- | --- |
| 43055 | `go -C apps/backend test -trimpath -p 1 -race ./internal/errands ./internal/task/dto` | PASS: 10.075s / 1.091s |
| 46012 | `go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite -run 'ErrandRecoveryAttention\|BatchErrandsSummaries\|ErrandsSummaryByTask\|ErrandListingAndLiveCount' -count=1 -v` | PASS: 8.294s; PostgreSQL cases explicitly skipped |
| 37411 | `GOFLAGS='-trimpath -p=1' xargs -a /tmp/kandev-errand-attention-checkpoint/files bash scripts/lint-go-changed` | PASS: 0 issues, no timeout |
| 40187 | `go -C apps/backend run -trimpath -p 1 ./cmd/sqlguard ./internal` | PASS: exit 0, no output |
| 59625 | `go -C apps/backend test -trimpath -p 1 -race ./internal/persistence/storeconformance -count=1` | PASS: 61.422s |
| 81135 | `go -C apps/backend test -trimpath -p 1 -race ./internal/mcp/handlers ./internal/task/handlers -run 'Errand\|ListTaskSessions' -count=1` | PASS: MCP 6.755s / task handlers 1.133s |

The broader affected-package race run 47443 also passed (Errands 1.754s, DTO 1.075s, MCP handlers 3.255s, task handlers 1.126s, SQLite 42.613s) before the final projected-terminal predicate addition. Final-source owning-package and SQLite checks above supersede it for that addition.

TDD behavioral RED evidence: 90803 (capacity expected 1, got 0), 49751 (service expected needs_attention, got running), 49977 (missing parent/DTO reason and terminal-reconciliation false positive), 90333 (unresolved work omitted from outstanding state), 96080 (ambiguous receipt suppressed attention), 71992 (projected terminal before intent sealing incorrectly showed attention). Each is covered by the passing final tests. Initial fixture failures (nil metadata map and full-session writes not persisting metadata) were corrected before counting behavioral RED. Nonfinal race 78769 exposed a test import cycle; the real SQLite service integration was moved to the existing Errands fixture. SQLite run 18220 exposed the query-counter issue described above, fixed and passed in 46012.

Logs and the lint input list remain in `/tmp/kandev-errand-attention-checkpoint/`: `owning-race.log`, `sqlite-final.log`, `lint.log`, `sqlguard-final.log`, `conformance-final.log`, `transport-final.log`, `ambiguous-red.log`, `terminal-red.log`, and `files`.

`KANDEV_TEST_POSTGRES_DSN` is unset. PostgreSQL behavior/conformance runtime coverage is a reported skip, not a pass. The original live incident was not read or mutated in this checkpoint; its described retained recovery identity is represented by the historical-reservation fixture. No deployment or live recovery claim is made.

## Exact write set

```text
apps/backend/internal/errands/attention_test.go
apps/backend/internal/errands/await.go
apps/backend/internal/errands/compose.go
apps/backend/internal/errands/delivery.go
apps/backend/internal/errands/events.go
apps/backend/internal/errands/fakes_test.go
apps/backend/internal/errands/interfaces.go
apps/backend/internal/errands/list.go
apps/backend/internal/errands/list_events_test.go
apps/backend/internal/errands/results.go
apps/backend/internal/errands/service.go
apps/backend/internal/errands/start.go
apps/backend/internal/errands/status.go
apps/backend/internal/mcp/handlers/errands.go
apps/backend/internal/mcp/handlers/errands_test.go
apps/backend/internal/mcp/handlers/list_task_sessions.go
apps/backend/internal/task/dto/errand.go
apps/backend/internal/task/dto/errand_test.go
apps/backend/internal/task/handlers/errand_ws_handlers.go
apps/backend/internal/task/models/errand.go
apps/backend/internal/task/repository/sqlite/errands.go
apps/backend/internal/task/repository/sqlite/errands_attention.go
apps/backend/internal/task/repository/sqlite/errands_attention_test.go
apps/backend/internal/task/repository/sqlite/errands_summary_batch.go
apps/backend/internal/task/repository/sqlite/errands_summary_batch_test.go
docs/plans/errand-orphan-settlement/attention-projection-handoff.md
```

Final whitespace validation: `git diff --check` passed. The parent may review and integrate this checkpoint; no commit was created.

## Read-only review follow-up

Era Sol review `7aca50d3-f348-4c1b-b97e-d8bc1a687973` found one P2 event/list consistency gap. Primary source inspection confirmed `publishAgentDeliveryRecoveryState` publishes only the session event; it bypasses `notifyErrandSessionStateChanged`, whose existing observer publishes the Errand snapshot and wakes awaits. An idle Errand changing typed recovery phase from reconnecting to uncertain can therefore change List projection without an Errand event or await wake. Checkpoint acceptance is conditional on correcting this gap.

Queue a bounded Lunaldinho remediation after current continuation writer `4b7888d4-ea18-44b6-99c6-8829489cb098` releases ownership. Scope: recovery publication path and focused tests, reusing the existing observer after authoritative metadata persistence. Verify actual same-session-state recovery transition produces needs_attention/recovery_unresolved event and wakes an await, including persistence/read failure and ordinary-session isolation; avoid duplicate state-transition notifications and avoid adding settlement authority. Use the shared build lock. Then obtain Era Sol read-only review of the correction. No other actionable findings in reviewed evidence/result precedence, errors, conversions or count predicates.
