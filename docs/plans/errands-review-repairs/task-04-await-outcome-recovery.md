---
id: "04-await-outcome-recovery"
title: "Fenced await outcomes and visible uncertainty"
status: done
wave: 4
depends_on:
  - "27-history-disclosure"
  - "03-atomic-result-claims"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.13
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
---

# Task 04: Fenced await outcomes and visible uncertainty

## Scope and authority

Repairs review finding(s) 1 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Passes 04A, 04B and 04C are complete; focused uncertainty browser acceptance is recorded in Results. Task 24's combined desktop/phone matrix remains a separate gate.

## Low-level implementation and regression plan

Own new `apps/backend/internal/errands/await_outcomes.go`, existing `await.go`, `reconcile.go`, `interfaces.go`, new task-repository attempt operations/tests beside `errands_claims.go`, `internal/mcp/server/errand_tools.go`, `internal/mcp/handlers/errands.go`, and a narrowly scoped abort-intent persistence/replay seam in `internal/agentctl/journal/` plus the existing backend bridge. Additive DTO projection is in `internal/task/dto/errand.go` and web `lib/types/errands.ts`; presentation is in `components/task/chat/errands/` and existing await renderers, with all affected locale catalogs. All backend paths are beneath `apps/backend/`, web paths beneath `apps/web/`. No general native receiver protocol or automatic resend is authorized.

Create task-scoped `errand_await_attempt` (`token` PK, `task_id`, `parent_session_id`, `state`, timestamps) and `errand_await_attempt_item` (PK `(token, errand_id, result_revision)`, captured generation). Retain attempts, including zero-item tombstones, until task deletion; bind operations to task and parent, index by task/state. Support SQLite and PostgreSQL.

Use the state, response and evidence contract in [Await reception evidence](../../specs/tasks/system-design/errand-await-receipts.md) in full. States are `open`, `claimed`, `offered`, `uncertain`, `aborted` and `received`. Capture trusted parent incarnation/harness generation and originating turn/submission when available at attempt creation. Commit `offered` before returning success; this never proves receipt. Return the bounded `await_delivery` token/exact-revision object in the actual successful MCP JSON body. Supported positive native observations settle `received`; unknown attempts remain retained.

`ClaimResults(task, parent, route, token, candidates)` takes the task guard. For await it creates/locks the attempt, checks task/parent and rejects an existing `aborted` tombstone. It reads the full parent inventory under the guard, validates the requested snapshot and distinct candidate IDs/versions, then claims all pending selected revisions and inserts all immutable attempt items in one transaction. Turn-context/notification uses the same atomic claim and records exact revisions in its receipt. On mismatch roll back and boundedly re-read/reproject. Return transaction snapshots only after commit; include text only for the exact token/revision. Assemble fallible content before claiming where possible; publish only committed changes. A commit error is unknown: retain the token and read back.

`AbortAwait(task, parent, token)` is idempotent and takes the same guard. Upsert an `aborted` tombstone even if no attempt or claim exists yet, then atomically release matching items subject to the identity, generation and admission rules above. Thus an abort that arrives before a delayed claim prevents that claim; a claim that wins first is released by the abort. Retry DB errors and ambiguous commits with the same token. Persist deferred release on the aborted attempt until admission resolves.

On backend error or cancellation the live MCP handler irrevocably selects error and persists a token/task/parent-bound abort intent in the existing agentctl journal. It cannot later return success. Retry on the authenticated bridge until the backend tombstone confirms; journal failure still returns error and leaves any unconfirmed backend claim conservative after loss. Before success commit `offered`; ambiguity selects the fenced error branch. No post-return socket callback can prove abandonment. `received` is terminal against late abort; contradictory receipt-after-abort is retained and diagnosed.

Implement the two supported positive sources (Codex ACP and native Codex app-server) and the atomic inbox reduction now, not as future hooks. Extract exact identity before display truncation; preserve the bounded adapter-issued observation through journal/replay. The repository accepts it only against authenticated owner/sequence/hash evidence and the complete attempt item set. Receipt update and inbox acceptance/cursor are one transaction. Missing observations, previews, completed tool cards and unsupported providers never authorize release.

Presentation uses optional revision-bound `delivery_state: received|uncertain`; omission is unavailable/in-progress. Keep `offered` internal and silent. After ownership loss or the exact originating turn's authenticated terminal, publish uncertainty only for the unresolved claimed revision, after reducing available positive evidence. A late valid receipt clears only its revision's uncertainty. Preserve the retained transcript, historical response, lifecycle and existing manual composer recovery. Legacy claims remain uncertain; zero-item waits warn about no result. No resend control or positive model-consumption badge is introduced.

### Sequential implementation passes

- **04A:** Attempt outcomes and trusted bridge attribution, offer/abort single-disposition fencing, durable abort journal/replay. Task 03 provides schema and immutable item storage.
- **04B:** Supported native receipt parsers, bounded serialized event observation, journal/display preservation and atomic inbox reduction. Include real replay/ownership integration before declaring reception support.
- **04C:** Revision-bound DTO/store/renderer projection, localized uncertainty and late receipt clearing. Pass ownership to Task 24 for final desktop/phone scenarios.

Each pass receives the concrete contract, exclusive files and its behavioral tests; preserve all predecessor edits. The following ownership table and regressions complete this packet.

| Owner | Files and exact responsibility |
| --- | --- |
| Task 03 / single migration owner, integrated before task 04 | `internal/task/repository/sqlite/errands_schema.go`, planned `errands_claims.go` and attempt schema/scanners: add `offered`, captured parent owner and compact receipt columns to the already planned attempt contract in SQLite and PostgreSQL. Preserve task guard ordering. One owner for all shared schema changes, coordinated with teardown. |
| Task 04, response/offer/abort | `internal/errands/await.go`, planned `await_outcomes.go`, `interfaces.go`, `reconcile.go`; `internal/mcp/handlers/errands.go`, `internal/mcp/server/errand_tools.go`: exact claimed revision metadata, fallible assembly before offer, durable offered transition, single terminal handler disposition, guarded abort/receipt precedence, retained recovery classification. |
| Task 04, trusted bridge attribution | `internal/agent/runtime/agentctl/agent.go` (`dispatchMCPRequest`), existing MCP dispatch context/verified-caller wiring, `internal/agentctl/server/api/agent.go` and `internal/mcp/server/backend_client.go` only where the existing abort/offer bridge needs it: capture the launched parent owner, not current mutable state; retain task/parent/token attribution. These are existing bridge seams, not a native receiver-ack endpoint. |
| Task 04, native extraction | `internal/agentctl/server/adapter/transport/acp/adapter_tools.go`, `dialect_codex.go`, `codex_mcp_tool_test.go`; `internal/agentctl/server/adapter/transport/codexappserver/adapter.go` (`emitItem`) plus a sibling MCP-result helper/test. Preserve real MCP identity and result; extract only supported successful native observations. |
| Task 04, bounded event contract | `internal/agentctl/types/streams/agent.go`, a narrow sibling receipt type/parser and its tests, `tool_display_prepare.go` only if necessary to preserve the field; `internal/agentctl/server/process/manager.go` (`persistDeliveryEventContext`) and journal tests: observation is extracted before `PrepareToolEventWithLimit`, committed before publish, immutable through snapshot/JSON/replay, within existing event budget. Keep actual native result unchanged. |
| Task 04, atomic positive decision | `internal/task/repository/sqlite/agent_delivery.go` (`ReceiveAgentDeliveryEvent` / `receiveAgentDeliveryEventTx`) and new sibling attempt-receipt helper/tests: authenticated inbox receipt and attempt decision in one transaction, guard before session/stream locks, no duplicate/hash rewrite, compact proof retention. Existing `agent_delivery_owner.go` admission remains authoritative. |
| Task 04, integration tests | `internal/agent/runtime/lifecycle/` sibling receipt/replay tests exercising `streams.go`, `durable_delivery_stream.go` and `stream_event_processor.go`; production changes there only if required to wire the existing receipt path or post-commit refresh. Do not move receipt authority to orchestrator callbacks. |
| Task 04, existing presentation | `internal/task/dto/errand.go`, `apps/web/lib/types/errands.ts`, existing `apps/web/components/task/chat/errands/`, await parser/renderers and affected locales: received/uncertain projection and timing, retained historical semantics, Open/manual recovery. |
| Task 24, exclusive browser writer | Existing desktop Errands suite and `apps/web/e2e/tests/session/errands.spec.ts` and `apps/web/e2e/tests/session/mobile-errands.spec.ts`: add scenarios only after ownership is released by their previous writer. |
| Parent / task 26 | Reconcile the shared design, task 04, plan ownership and public response/uncertainty reference after integration. No durable file was edited by this check. |


## ASCII UI preview

UI-UNCERTAIN-01, desktop popover and phone drawer share the row semantics:

```text
Child audit: Done
Result delivery uncertain                 [Open]
Open -> retained child transcript + composer
Phone: one whole-row tap -> full-screen child [Back]
```

The line is localized in every shipped locale; phone retains one 44px target, one internal list scroller and the fixed footer. No automatic replay, no false lifecycle failure and no duplicate announcer.

RED: `TestAwaitAbortBeforeClaimFencesLateWorker`, `TestAwaitAbortAfterClaimReleasesExactRevision`, `TestAwaitErrorAndCancellationNeverReturnSuccessAfterAbort`, `TestAwaitAbortReplaySurvivesBackendDisconnect`, `TestAwaitRecoveryPreservesUnknownExposure`, `TestAwaitLegacyClaimsRemainUncertain`, and `TestAwaitAbortCannotReleaseSupersededOrForeignResult`. Test known abort, unknown commit, journal failure, late backend response, task/parent mismatch and concurrent successor admission. Frontend tests cover omission/uncertainty, legacy rows, retained history and actual desktop/phone Open.

1. **Response identity:** `TestAwaitResponseCarriesOnlyCommittedRevisionReceipts` covers batch claims, status-only/already-delivered rows, zero items, exact token/revisions, repeated equal result text with distinct revisions and no fallible post-claim reload. Verify offer failure/ambiguous commit selects error and durable abort; success never implies received.
2. **Supported native positives:** `TestCodexACPAwaitReceiptUsesSameCanonicalMCPResult` and `TestCodexAppServerAwaitReceiptPreservesActualResult` use native-shaped fixtures, real native call IDs, correct server/tool, successful full MCP body and exact item set. Test error envelope, `isError`, status-only updates, absent payload, missing initial provenance, foreign server/tool, shell/assistant text containing identical metadata, nested child thread, missing native ID and malformed/duplicate/versioned items. These must not mint receipts. The native app-server fixture must use `tool`, `server` and `result` from the pinned schema.
3. **Preparation/journal preservation:** `TestAwaitReceiptSurvivesToolDisplayBoundsAndJournalReplay` starts with a large real native result and proves the small observation survives display truncation, snapshot, JSON encoding, journal commit/reopen and replay, while the source native result is untouched. Include a journal append failure: no durable positive proof is claimed, the attempt remains unresolved, no result replay.
4. **Atomic SQL acceptance:** `TestErrandAwaitReceiptAndInboxCommitAtomically` injects failure between receipt update and inbox commit and verifies neither state nor cursor commits. Retry the same event after reopening the database. Include ambiguous commit readback, duplicate same event/hash, conflicting payload hash and a later identical receipt on another sequence. Verify first proof is retained and one `received` decision is published. Run on SQLite and real PostgreSQL with independent connections for the guard/race cases; a skipped PostgreSQL run remains unproved.
5. **Ownership/terminal races:** `TestErrandAwaitReceiptCannotCrossParentOrRevision` covers wrong task, session, incarnation, harness generation, token and item set; use the real owner-admission path. A valid old-revision receipt settles its retained attempt but leaves a successor revision/token/generation unchanged. `TestAwaitReceivedCannotBeReleasedByLateAbort` and the reverse contradictory-abort case prove terminal fences and no replay. Include controlled barriers around offer, abort, receipt, parent admission and result replacement.
6. **Real replay integration:** `TestAwaitNativeReceiptReconcilesAfterBackendRestart` commits a supported native result to the real agentctl journal with backend unavailable, reconnects through authenticated lifecycle replay and verifies received before ACK/pruning. Separately crash after inbox+receipt commit but before projection/ACK and replay again. Native result reception before journal append with process loss remains uncertain. Never call Prompt, re-execute await or enqueue result delivery in these tests. Include Errands disabled with a retained attempt and a fake completed tool card without result; only the real receipt resolves it.
7. **Presentation timing:** parser/store/component tests prove no warning for live `open`/`claimed`/`offered`, no false `received` from omission, uncertainty only for the exact recovered claimed revision, late receipt clearing, no warning for zero-item response, unchanged lifecycle and historical await card. Desktop and phone scenarios exercise uncertain -> Open -> retained child -> Back and late receipt -> reload; preserve the existing phone one-scroller drawer/footer, 44px whole-row target and full-screen child. No composition change is proposed, so reuse task 04's existing ASCII preview with the warning absent after positive resolution. This follows the mobile-parity skill's state/data-only scope; rendered desktop/phone checks remain assigned to task 24 rather than run during this read-only design check.


```bash
(cd apps/backend && go test -trimpath -race ./internal/errands ./internal/mcp/handlers ./internal/mcp/server ./internal/agentctl/journal ./internal/agentctl/types/streams ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'Await|Errand.*(Receipt|Claim|Abort|Attempt|Uncertain)|PostgresErrand' -count=1)
```

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/errands components/task/chat/messages/kandev/errand-renderers.test.tsx lib/types/errands.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
git diff --check
```

Run changed-Go and exact-file ESLint for touched files, recorded explicitly. Task 24 owns the serialized final desktop/phone suites; add uncertainty scenarios to those files only under exclusive ownership after their previous writer releases them.

## Dependencies and ownership

[Task 03](task-03-atomic-result-claims.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

04A complete and verified, 2026-11-01. Task04 remains in_progress until 04B and 04C are accepted. The complete work order, owning await-receipts/shared-recovery designs, accepted uncertainty ADR and Task03 final acceptance were read before implementation. Task03's existing schema, immutable snapshots, shared guard, bounded same-token retry and exact admission/release foundation are preserved.

Attempt authority is captured from the authenticated launched parent by value, with the originating submission when unambiguous. An unavailable turn/submission association stays empty. The repository adds `BeginErrandAwait`, `OfferErrandAwait` and `AbortErrandAwait` on the existing task guard and attempt tables. `AwaitRequest.Owner` carries the captured authority; `AwaitResult.Delivery` carries the bounded version-one `await_delivery` token/exact revisions in the actual successful JSON text body. Status-only and previously delivered rows acquire no receipt items; equal text on different revisions keeps distinct identities. The backend assembles the response before committing `offered`; any failed or ambiguous offer selects error, including a SQL write that committed before its acknowledgement was lost. Sender success records offered state; native receipt authority remains 04B.

The MCP handler chooses one terminal disposition. Error/cancel discards a late successful body, persists the task/parent/token/owner-bound abort intent in the existing bbolt journal and retries only that abort through the authenticated bridge until the backend tombstone confirms. The private journal bucket participates in accounting, survives reopen and rejects conflicting identities. `AwaitAbortStore` is wired through the process manager/API; the existing stream lifecycle owns and joins `ReplayAwaitAborts`. A live memory retry covers journal-write failure. A fresh server without durable evidence preserves the unresolved claim after loss. Received remains terminal against late abort; foreign identities and superseded revisions cannot be released. Retained aborts remain usable with Errands disabled and after a harness-generation replacement, using the original stored identity.

Exact source/test inventory was recorded here before edits; actual final inventory is below, also saved in `/tmp/errands-task04a-existing.txt` and hashed in `/tmp/errands-task04a-source-hashes.json`. The shared identity DTO avoids a journal/models/streams import cycle; the model aliases that single DTO. The initially reserved API `await_abort.go` sibling was unnecessary. All paths below are beneath `apps/backend/`.

- `internal/task/models/errand_await.go`
- `internal/task/repository/sqlite/errands_await_outcomes.go`
- `internal/task/repository/sqlite/errands_await_outcomes_test.go`
- `internal/task/repository/sqlite/errands_await_abort.go`
- `internal/errands/await.go`
- `internal/errands/await_outcomes.go`
- `internal/errands/interfaces.go`
- `internal/errands/await_outcomes_test.go`
- `internal/errands/fakes_test.go`
- `internal/mcp/handlers/errands.go`
- `internal/mcp/handlers/errands_test.go`
- `internal/mcp/handlers/errand_await_outcomes_test.go`
- `internal/mcp/server/errand_tools.go`
- `internal/mcp/server/errand_await_outcomes.go`
- `internal/mcp/server/errand_await_outcomes_test.go`
- `internal/mcp/server/server.go`
- `internal/mcp/server/backend_client.go`
- `internal/mcp/origin/await.go`
- `internal/agentctl/types/streams/mcp_execution.go`
- `internal/agent/runtime/lifecycle/mcp_identity.go`
- `internal/agent/runtime/lifecycle/mcp_await_owner_test.go`
- `internal/agentctl/journal/await_abort.go`
- `internal/agentctl/journal/await_abort_test.go`
- `internal/agentctl/server/process/await_abort.go`
- `internal/agentctl/server/process/await_abort_test.go`
- `internal/agentctl/server/api/server.go`
- `internal/agentctl/server/api/agent.go`
- `internal/agentctl/server/api/await_abort_test.go`
- `internal/mcp/handlers/errand_await_outcomes.go`
- `internal/agentctl/types/errandawait/identity.go`
- `internal/agentctl/journal/journal.go`

Behavioral RED receipts: `/tmp/errands-task04a-disposition-red.log` reproduces success after cancellation/release; `/tmp/errands-task04a-owner-red.log` and `/tmp/errands-task04a-journal-red.log` establish the missing persistence boundaries; `/tmp/errands-task04a-journal-failure-red.log` reproduces lost live retry after journal failure. The process-manager regression in `/tmp/errands-task04a-boundary-green.log` caught the closed-journal/no-active-submission gap before its fix. Initial infrastructure/compile fixture failures are retained as failures, not passes. Lint's two findings are retained in `/tmp/errands-task04a-lint-initial.log`; a private constant/helper extraction resolved them without changing SQL queries or outcome rules, followed by the final-source recheck.

All heavy commands used `/tmp/kandev-errands-repair-build.lock`; Go used `GOMAXPROCS=2 GOFLAGS=-p=1`, explicit `-p 1` and the 8GiB compiler/export target. Lint analysis used 3GiB and concurrency one. Final runtime recheck command: `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04a-recheck.sh`, exit 0. The complete journal/conformance run is `/tmp/errands-task04a-verify.sh`; those unchanged contracts were not repeated after the lint-only refactor.

| Exact command from `apps/backend` | Observed result | Receipt |
| --- | --- | --- |
| `go test -trimpath -race -p 1 ./internal/errands ./internal/mcp/handlers ./internal/mcp/server ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'Await\|Errand.*(Claim\|Abort\|Attempt)\|PostgresErrand\|MCPHandlerFor\|AgentStream.*MCP' -count=1 -v` | 104 final-source top-level passes, including 38 actual PostgreSQL cases; zero skips, failures or race reports | `/tmp/errands-task04a-race.log` |
| `go test -trimpath -race -p 1 ./internal/agentctl/journal -count=1 -v` | Complete package: 87 top-level passes, zero skips, failures or race reports | `/tmp/errands-task04a-journal-race.log` |
| `go test -trimpath -race -p 1 ./internal/agentctl/journal -run Await -count=1 -v` | Both new journal regressions passed again after the final production edit | `/tmp/errands-task04a-journal-final-regressions.log` |
| `go test -trimpath -race -p 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3\|pgx)/^task$' -count=1 -v` | All 14 task scenarios passed on SQLite and actual isolated PostgreSQL | `/tmp/errands-task04a-conformance.log` |
| `go run -trimpath -p 1 ./cmd/sqlguard ./internal` | Pass after final edit, empty output | `/tmp/errands-task04a-sqlguard.log` |
| `go build -trimpath -p 1 ./internal/backendapp` | Pass after final edit, empty output | `/tmp/errands-task04a-build.log` |

Final lint command: `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04a-lint.sh`. All 12 owning packages use the canonical comparison base `74a91cfc161916f0d5264b86762adae14e43119e`; exact `go list -p 1 -buildvcs=false -pgo=off -tags=fts5,e2e -deps -test -export -compiled` preparation passed at 8GiB, then `golangci-lint run <those packages> --new-from-rev=<base> --timeout=5m --concurrency=1` passed with 0 issues at 3GiB. Receipts: `/tmp/errands-task04a-lint-exports.log`, `/tmp/errands-task04a-lint.log`.

PostgreSQL used only the disposable `/tmp/errands-pg-check`, localhost55487, with isolated test schemas. Start and stop used `flock --close`; final stop succeeded and `postmaster.pid` is absent. Receipts: `/tmp/errands-task04a-pg-{start,stop}.log` and `/tmp/errands-task04a-pg-recheck-{start,stop}.log`. All 31 owned hashes match; all 62 Task03 files outside the inventory match their accepted hashes. Documentation/specification validation and final diff check are recorded in `/tmp/errands-task04a-docs.log` and `/tmp/errands-task04a-diff-check.log` at final release. No source writer, build process or database daemon remains from this pass.

Next gates are explicit: 04B owns supported native positive parsing, bounded observation preservation through display/journal/replay and atomic authenticated inbox-plus-receipt reduction. 04C owns recovered uncertainty and revision-bound DTO/store/UI projection. Task24 owns final browser scenarios; parent/Task26 owns public response/reference reconciliation. SQL schema, frontend, shared manifest, native receiver protocol and automatic result resend were not changed here. The completed-errand resource-parking proposal remains outside scope. Changes remain uncommitted; Task04 stays in_progress.

### 04B inventory before implementation, 2026-11-01

Pass 04A final acceptance is authoritative. Pass 04B is in progress, limited to native receipt extraction, bounded serialization, atomic authenticated inbox reduction and replay verification. No 04C projection or shared manifest changes. Exact additional source/test inventory (all beneath `apps/backend/`):

- `internal/agentctl/types/streams/agent.go`
- `internal/agentctl/types/streams/errand_await_receipt.go`
- `internal/agentctl/types/streams/errand_await_receipt_test.go`
- `internal/agentctl/types/streams/tool_display_prepare.go`
- `internal/agentctl/server/adapter/transport/acp/adapter.go`
- `internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `internal/agentctl/server/adapter/transport/acp/await_receipt.go`
- `internal/agentctl/server/adapter/transport/acp/await_receipt_test.go`
- `internal/agentctl/server/adapter/transport/codexappserver/adapter.go`
- `internal/agentctl/server/adapter/transport/codexappserver/await_receipt.go`
- `internal/agentctl/server/adapter/transport/codexappserver/await_receipt_test.go`
- `internal/agentctl/server/process/await_receipt_test.go`
- `internal/task/repository/sqlite/agent_delivery.go`
- `internal/task/repository/sqlite/errands_await_receipt.go`
- `internal/task/repository/sqlite/errands_await_receipt_test.go`
- `internal/task/repository/sqlite/errands_await_receipt_race_test.go`
- `internal/agent/runtime/lifecycle/await_receipt_replay_test.go`

The existing process manager and journal serialization paths will be exercised without production edits unless the behavioral tests expose a required seam. Any further file will be inventoried here before editing. Native app-server fixtures use the pinned v0.154.0 schema fields `server`, `tool`, `status`, `result`, `error` and item `id`. Receipt decisions stay within the existing task guard and live owner admission. Task04 remains in_progress.

04B additional inventory before edits: `internal/task/repository/sqlite/errands_await_abort.go` and `internal/task/repository/sqlite/errands_await_outcomes.go` (beneath `apps/backend/`) require only replacing repeated state literals with shared private constants after the receipt reducer raises package-wide lint counts. Their SQL, transaction and terminal semantics remain unchanged. Initial lint evidence is `/tmp/errands-task04b-lint-initial.log`; no suppression or resource bypass is used.

### 04B implementation and verification

04B is complete and accepted, 2026-11-01. Both supported native parsers and atomic replay reduction are implemented and verified. Task04 stays in_progress for 04C. The final source/test inventory contains the 19 files recorded above; `/tmp/errands-task04b-source-hashes.json` identifies the exact candidate.

Supported Codex ACP observations require the tracked implementation-marked canonical parent MCP call, its native call ID and the successful original outer result envelope. Native Codex app-server observations use the pinned v0.154.0 `mcpToolCall` item fields `server`, `tool`, `status`, `result`, `error`, item `id` and the actual owning source thread. Missing provenance, remapped child threads, errors, status-only updates, previews, unrelated tools and assistant text cannot mint observations. The ACP completion now preserves its canonical MCP `tool_name`; an actual wire-to-replay regression caught that missing field before acceptance. Missing origin and malformed observations are diagnosed.

`streams.AgentEvent.ErrandAwaitReceipt` carries only bounded version/token/exact revision items, independently of normalized display output and the nonserialized MCP bit. Extraction precedes display preparation. Preparation detaches the observation's item slice; the ordinary process manager and journal carry it through JSON, append, reopen and replay. The five-result, 16KiB-per-result fixtures mirror the actual await response. Display truncation preserves the observation and leaves the source native MCP body unchanged.

`ReceiveAgentDeliveryEvent` takes the task delivery guard before session/stream locks for receipt-bearing events. After live-owner admission and sequence/hash validation, the transaction-local reducer checks captured task/session/incarnation/harness ownership and the complete immutable attempt item set, then commits receipt state/first compact proof together with inbox acceptance and the received cursor. Native reception binds the exact call/token/owner independently of a later turn association; originating turn/submission remains retained for 04C classification, and native event submission admission is unchanged. Operational SQL failure rolls back the entire transaction. Same-identity replay reads back unknown commits; repeated valid reports retain the first proof, and conflicting accepted event content is rejected.

Received remains terminal against abort. A contradictory receipt after abort retains its first compact proof and diagnoses the conflict while leaving the abort terminal. Historical receipts do not write current errand rows, including a successor result already claimed under a new token at a later admission generation. Existing lifecycle callbacks run only after receipt acceptance commits; reception adds no result dispatch or prompt execution.

Both real native adapter fixtures pass through bbolt append/reopen and authenticated lifecycle/SQL replay for backend-offline reception and crash after inbox-plus-receipt commit before projection/ACK. Loss before journal append, failed append and a receipt-free completed card remain unresolved. Retained evidence is accepted with Errands disabled. PostgreSQL tests use independent connections and controlled barriers for offer, abort, receipt, admission, result replacement and harness generation replacement, including proof that a receipt waiting for the task guard holds no session lock.

Behavioral RED receipts: `/tmp/errands-task04b-native-red.log` (missing native observations), `/tmp/errands-task04b-sql-red.log` (inbox accepted without receipt), `/tmp/errands-task04b-native-matrix-red.log` (contradictory ACP input), `/tmp/errands-task04b-diagnostics-red.log` and `/tmp/errands-task04b-acp-origin-red.log` (missing diagnostics), `/tmp/errands-task04b-origin-turn-red.log` (incorrect late-turn restriction), `/tmp/errands-task04b-acp-integration-red.log` (actual ACP completion lost tool identity), and `/tmp/errands-task04b-schema-diagnostic-red.log` (schema mismatch was rejected without a diagnostic). Earlier compile/fixture failures, initial lint findings and the incomplete first race run remain recorded as failures in `/tmp/errands-task04-progress.md`; they are not final passes.


All heavy commands used `/tmp/kandev-errands-repair-build.lock`; Go used `GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=8GiB`, explicit `-p 1`, and tests additionally used `-parallel 1`. Final source recheck: `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04b-recheck.sh`, exit 0. Every new or changed regression ran after the final production edit. The complete journal and task-store conformance receipts predate only the isolated app-server schema diagnostic; their source and contracts were unchanged.

| Exact command from `apps/backend` | Observed result | Receipt |
| --- | --- | --- |
| `go test -trimpath -race -p 1 -parallel 1 ./internal/task/repository/sqlite ./internal/agentctl/types/streams ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'Await\|Errand.*(Receipt\|Claim\|Abort\|Attempt\|Uncertain)\|PostgresErrand' -count=1 -v -timeout=300s` | 85 top-level passes, including 43 actual PostgreSQL cases; zero skips/failures/races | `/tmp/errands-task04b-race.log` |
| `go test -trimpath -race -p 1 -parallel 1 ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver -count=1 -v -timeout=300s` | 716 top-level passes, zero failures/races; one unrelated installed OpenCode compatibility test skipped without its explicit opt-in | `/tmp/errands-task04b-native-race.log` |
| `go test -trimpath -race -p 1 -parallel 1 ./internal/agentctl/journal -count=1 -v` | Complete package: 87 top-level passes, zero skips/failures/races | `/tmp/errands-task04b-journal-race.log` |
| `go test -trimpath -race -p 1 -parallel 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3\|pgx)/^task$' -count=1 -v` | 14 task-store scenarios across SQLite and actual isolated PostgreSQL passed | `/tmp/errands-task04b-conformance.log` |
| `go run -trimpath -p 1 ./cmd/sqlguard ./internal` | Pass after final edit, empty output | `/tmp/errands-task04b-sqlguard.log` |
| `go build -trimpath -p 1 ./internal/backendapp` | Pass after final edit, empty output | `/tmp/errands-task04b-build.log` |

Final lint command: `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04b-lint.sh`, exit 0. All six owning packages use canonical comparison base `74a91cfc161916f0d5264b86762adae14e43119e`; exact `go list -p 1 -buildvcs=false -pgo=off -tags=fts5,e2e -deps -test -export -compiled` warming passed at 8GiB, then `golangci-lint run <those packages> --new-from-rev=<base> --timeout=5m --concurrency=1` reported 0 issues at 3GiB. Logs: `/tmp/errands-task04b-lint-exports.log` and `/tmp/errands-task04b-lint.log`. The initial wrapper build failure from an accidental test-only `-parallel` flag is retained in `/tmp/errands-task04b-build-initial.log`; its corrected build passed and the final source recheck independently built again.

PostgreSQL used only disposable `/tmp/errands-pg-check`, localhost55487, with isolated test schemas. Start/stop commands used `flock --close`. The final stop succeeded and `postmaster.pid` is absent; receipts are `/tmp/errands-task04b-pg-{start,stop}.log` and `/tmp/errands-task04b-pg-recheck-{start,stop}.log`. The installed OpenCode compatibility skip supplies no live-provider evidence; both supported Codex paths use real protocol fixtures and authenticated journal/lifecycle/SQL integration. This is the designed mocked-provider boundary, not a live-provider crash guarantee.

The 19-file source/test freeze is `/tmp/errands-task04b-source-hashes.json`; all files outside this inventory from 04A match their accepted hashes. No frontend, shared manifest, public documentation, runtime settings, live database, commit or push changed in this pass. 04C still owns uncertainty classification and revision-bound DTO/store/rendering; Task24 owns final browser scenarios and parent/Task26 public references. The resource-parking proposal remains outside scope.

Final release check: documentation catalog validated 422 decisions and 1862 specifications, specification lint passed, and full diff check is empty (`/tmp/errands-task04b-docs.log`, `/tmp/errands-task04b-diff-check.log`). All 19 final source hashes match. All 62 Task03 files outside the 04A/04B inventories and every 04A file outside the 04B inventory match their accepted hashes. Owned untracked files pass direct trailing-whitespace validation. No source writer, build handle, build lease or disposable PostgreSQL process remains from 04B. Changes remain uncommitted.

### 04C backend implementation and verification, 2026-11-01

The bounded backend portion of 04C is implemented and verified. Task04 remains **in_progress** for the separate sequential frontend/localization packet and remaining acceptance gates. No frontend source, locale, browser build, shared manifest, runtime flag/default, live instance, commit or push was changed by this packet. The completed-errand resource-parking proposal remains outside scope.

The exact additive browser contract is `delivery_state?: "received" | "uncertain"` on the existing ErrandDTO, list/detail responses and errand.updated payload. Omission means unavailable or still in progress, never received. The repository projects state in the same SQL snapshot as the current row, matching task, parent, current result revision, delivery token and captured admission generation against the immutable attempt item. Live attributable open/claimed/offered attempts remain silent. Only a current delivered await claim can expose state; zero-item/status-only waits do not warn named results. Legacy delivered await claims without attributable proof remain uncertain. An old receipt cannot clear a successor claim's warning. The DTO rejects internal/invalid values and omits projection outside delivered-await results. Mutation responses read the same projection inside their existing guarded SQL transaction, preventing a stale input model from restoring old uncertainty or attaching an old receipt to a replacement revision. The model field is transient and excluded from immutable claim JSON.

`ListUnresolvedErrandAwaits(ctx, parentID)` captures retained immutable identities; an empty parent selects startup inventory. `RecoverErrandAwait(ctx, identity)` classifies exactly the retained invocation under the existing task guard, preserving received/aborted terminal decisions and every result claim. `ClassifyErrandAwaitTerminal(ctx, event)` requires the exact accepted authenticated inbox event, matching stored content, completed projection and a cursor at/past its sequence. Every captured nonempty turn/submission dimension must match. Missing origin uses recovery only; no latest-turn guess or timer is used. Neither classification releases a claim, resends content, prompts a provider, or infers non-reception.

The existing lifecycle pipeline refreshes committed current evidence independently of ordinary callback/duplicate suppression. Exact-origin terminals classify after ordered projection. Recovered streams process all available native evidence before classifying unresolved attempts owned by the captured task/session/incarnation/harness generation, including loss before a journal stream exists. Existing recovery leases are checked before classification, including empty-stream recovery. `Manager.SetErrandAwaitRefresh` is wired to `Service.RefreshAwaitDelivery` in backendapp. Retained recovery and refresh remain active with Errands disabled. Publication uses the existing errand.updated surface; a missed publication is repaired by existing list/reconnect hydration. No endpoint, receiver protocol, schema or runtime ownership architecture was added. Existing task-guard and atomic receipt/inbox/cursor authority remain unchanged.

Frontend handoff: add the optional closed field to the existing errand type/store/renderer, preserve authoritative full-DTO omission semantics, and clear only the uncertainty line on received. Attempt decisions do not change the errand Version/UpdatedAt/lifecycle, so an unchanged result timestamp must not suppress a delivery-state update. Omission, handler success, completed tool cards, delivered result state and historical result-returned markers are not receipt evidence. Preserve historical await responses and existing Open/manual composer recovery; do not add a cognition badge or resend control. No child polling/new endpoint is required. The exact handoff is also saved at `/tmp/errands-task04c-backend-handoff.md`. Task24 retains final real desktop/phone browser scenarios; the UI packet may cover focused rendered acceptance.

The 22-file source/test inventory was recorded before edits at `/tmp/errands-task04c-backend-inventory.txt`, with prior hashes at `/tmp/errands-task04c-backend-before-hashes.json`. The frozen verified candidate is `/tmp/errands-task04c-backend-source-hashes.json`. Three predecessor test files were updated only for their new derived-field/recovered-state expectations; their persisted-row, no-release and no-receipt assertions remain. The existing receipt reducer changed only its private uncertain-state constant reference, with no SQL/owner/terminal behavior change. Concurrent Task18 service-source/Results and Task24/manifest edits outside this inventory were observed and preserved.

Behavioral REDs are retained under `/tmp/errands-task04c-backend-`: projection-red.log, dto-red.log, terminal-red.log, service-red.log, lifecycle-red.log, snapshot-red.log, dto-closed-red.log, lease-red.log, missing-stream-red.log and mutation-red.log. Initial import-cycle/client-Close fixture failures were corrected before behavioral RED. Initial contract failures were the new derived-field/recovery expectations, not failures hidden as passing evidence. The initial single goconst finding is retained in lint-initial.log and was resolved without suppression. Progress chronology and exact interfaces are in `/tmp/errands-task04-progress.md`.

All heavy work used `/tmp/kandev-errands-repair-build.lock`, `GOMAXPROCS=2 GOFLAGS=-p=1`, explicit `-p 1`, test `-parallel 1`, shared `GOCACHE=/home/luancm/.cache/go-build` and `GOMODCACHE=/home/luancm/go/pkg/mod`. Large compiler/export preparation used the known-required 8 GiB soft target; light SQLguard/lint analysis used 3 GiB, with lint concurrency one. These are per-process soft targets, not aggregate hard caps. No caches were cleaned and no live cgroup/service settings were changed.

| Final command / scope | Observed result | Receipt |
| --- | --- | --- |
| `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04c-backend-verify.sh`, focused new/changed `go test -trimpath -race -p 1 -parallel 1` over repository, errands, DTO and lifecycle |35 top-level passes, including 12 actual isolated PostgreSQL cases; zero failures/skips/races. Covers ordered origin mismatch/absence, live/recovered state, both native producers through authenticated replay, loss/late receipt, successor isolation, zero/legacy/omission, callback suppression, guarded races, ambiguous commit, mutation responses and failed publication/list repair | `/tmp/errands-task04c-backend-race.log` |
| Same script, `go test -trimpath -race -p 1 -parallel 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3\|pgx)/^task$' -count=1 -v -timeout=300s` | All 14 task-store scenarios passed across SQLite and actual PostgreSQL | `/tmp/errands-task04c-backend-conformance.log` |
| Same script, `go run -trimpath -p 1 ./cmd/sqlguard ./internal` and `go build -trimpath -p 1 ./internal/backendapp` | Both passed after final production changes, empty output | `/tmp/errands-task04c-backend-{sqlguard,build}.log` |
| `go test -trimpath -race -p 1 -parallel 1 ./internal/task/dto -run 'ErrandDTO\|ErrandUpdatedEvent' -count=1 -v` | All 3 existing public-field/preview/predicate/event-shape contracts passed after final changes | `/tmp/errands-task04c-backend-dto-contract.log` |
| `flock /tmp/kandev-errands-repair-build.lock bash /tmp/errands-task04c-backend-lint.sh` | Six owning packages, canonical base 74a91cfc161916f0d5264b86762adae14e43119e; exact tagged export warming passed at 8 GiB, then changed-code analysis passed with 0 issues at 3 GiB/concurrency1 | `/tmp/errands-task04c-backend-lint-exports.log`, `/tmp/errands-task04c-backend-lint.log` |

Unchanged 04A/04B native-adapter/journal receipts remain valid; those full suites were not wastefully repeated. Existing MCP handler/server await contracts also passed in `/tmp/errands-task04c-backend-contract-initial.log`; their code/contracts were unchanged by the later lifecycle/mutation corrections. Supported native evidence is proved through actual protocol fixtures, real journal and authenticated lifecycle/SQL integration. No live-provider crash guarantee or browser acceptance is claimed here. Disposable PostgreSQL uses only `/tmp/errands-pg-check`, localhost55487, with isolated schemas. Documentation/preservation and final shutdown receipts follow below.

Final backend release check: documentation catalog validated 422 decisions and 1862 specifications; all specification files passed, and full diff check is empty (`/tmp/errands-task04c-backend-docs.log`, `/tmp/errands-task04c-backend-diff-check.log`). All 22 frozen source/test hashes match and owned untracked files pass direct trailing-whitespace checks. The release audit records preserved concurrent changes and unchanged frontend source (`/tmp/errands-task04c-backend-release-audit.json`). Disposable PostgreSQL was stopped successfully under flock --close; postmaster.pid is absent (`/tmp/errands-task04c-backend-pg-stop.log`). No owned source writer, build handle, build lease or database daemon remains. The backend handoff is ready; Task04 remains in_progress for frontend/localization and the existing remaining gates.

### 04C frontend and localization implementation, 2026-11-01

The frontend/localization packet is implemented. The existing Errand type accepts optional `delivery_state: "received" | "uncertain"`; existing store and WebSocket ingestion retain full-DTO replacement semantics, equal-version/equal-timestamp updates, and older-event/request fences. No production store or handler change was needed. Regressions cover same-timestamp received and omission clearing through upsert, list projection, and real `errand.updated` ingestion, with stale-event and sibling-isolation checks.

The shared errand row displays the localized, passive uncertainty line only for explicit `uncertain`, in both the desktop popover and native phone drawer. Received and omitted values clear it. The phone Open target describes the warning and keeps the existing minimum target size; no received badge, status/history mutation, duplicate announcement, resend action, polling, or new state machine was added. All eight required task locale catalogs were updated, and the required `i18n:zh-hant` generator was run; its synchronized existing Traditional Chinese task/chat entries are preserved.

| Exact command from `apps/web` | Observed result | Receipt |
| --- | --- | --- |
| `pnpm exec vitest run lib/types/errands.test.ts lib/state/slices/errands/errands-slice.test.ts lib/ws/handlers/errands.test.ts components/task/chat/errands/errands-chip.test.tsx hooks/domains/session/use-task-errands.test.ts --maxWorkers=1` | 86 tests passed across 5 files | `/tmp/errands-task04c-ui-unit-final2.log` |
| `pnpm exec eslint --max-warnings 0 lib/types/errands.ts lib/types/errands.test.ts lib/state/slices/errands/errands-slice.test.ts lib/ws/handlers/errands.test.ts components/task/chat/errands/errands-panel-row.tsx components/task/chat/errands/errands-chip.test.tsx e2e/helpers/errands.ts e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts` | Passed with zero warnings | `/tmp/errands-task04c-ui-eslint-final.log` |
| `pnpm exec prettier --check lib/types/errands.ts lib/types/errands.test.ts lib/state/slices/errands/errands-slice.test.ts lib/ws/handlers/errands.test.ts components/task/chat/errands/errands-panel-row.tsx components/task/chat/errands/errands-chip.test.tsx e2e/helpers/errands.ts e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts src/locales/en/task.json src/locales/ja/task.json src/locales/ko/task.json src/locales/pseudo/task.json src/locales/pt-pt/task.json src/locales/zh-cn/task.json src/locales/zh-hk/task.json src/locales/zh-tw/task.json src/locales/zh-hk/chat.json src/locales/zh-tw/chat.json` | All matched files use Prettier style | `/tmp/errands-task04c-ui-prettier-final.log` |
| `pnpm run i18n:check` and `pnpm run i18n:ratchet` | Both passed; all required catalogs complete and changed-copy ratchet clean | `/tmp/errands-task04c-ui-i18n-check-final.log`, `/tmp/errands-task04c-ui-i18n-ratchet-final.log` |
| `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` | Full web typecheck passed | `/tmp/errands-task04c-ui-typecheck-final2.log` |
| `pnpm --filter @kandev/web build:e2e` | E2E Vite bundle built after the shared-row production change | `/tmp/errands-task04c-ui-vite-build.log` |
| `git diff --check -- apps/web/lib/types/errands.ts apps/web/lib/types/errands.test.ts apps/web/lib/state/slices/errands/errands-slice.test.ts apps/web/lib/ws/handlers/errands.test.ts apps/web/components/task/chat/errands/errands-panel-row.tsx apps/web/components/task/chat/errands/errands-chip.test.tsx apps/web/e2e/helpers/errands.ts apps/web/e2e/tests/session/errands.spec.ts apps/web/e2e/tests/session/mobile-errands.spec.ts apps/web/src/locales/en/task.json apps/web/src/locales/ja/task.json apps/web/src/locales/ko/task.json apps/web/src/locales/pseudo/task.json apps/web/src/locales/pt-pt/task.json apps/web/src/locales/zh-cn/task.json apps/web/src/locales/zh-hk/task.json apps/web/src/locales/zh-hk/chat.json apps/web/src/locales/zh-tw/task.json apps/web/src/locales/zh-tw/chat.json docs/plans/errands-review-repairs/task-04-await-outcome-recovery.md` | Passed with empty output | `/tmp/errands-task04c-ui-diff-check-final.log` |
| `make -C apps/backend GOFLAGS=-p=1 build-kandev build-mock-agent build-e2e-delivery-fixture e2e-plugin-package` with `GOMAXPROCS=2 GOMEMLIMIT=8GiB` and a `pnpm` wrapper pinned to `corepack pnpm@9.15.9` | Complete current backend and required helpers rebuilt; frozen 25-file Task07 manifest matched before/after | `/tmp/errands-task04c-current-build-wrapper.log`; `apps/backend/bin/kandev` SHA256 `440e660ac8e1f64e004322473d1e31ddebde84e341aa0a3d05a8ddb31b5caa9a` |
| `scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --no-build --shards 1 --project chromium -- e2e/tests/session/errands.spec.ts --grep "desktop errand panel shows only explicit await delivery uncertainty"` | 1 passed in 11.9s; source manifest matched before/after | `/tmp/errands-task04c-current-browser-pair.log`, `/tmp/errands-task04c-run-quiet/kandev-run.e2e.DhJdWyjn.log` |
| `E2E_BROWSER_CONSOLE=1 scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome -- e2e/tests/session/mobile-errands.spec.ts --grep "phone errand drawer shows uncertainty and keeps transcript recovery reachable"` | 1 passed in 13.2s; parent chat/socket readiness, drawer containment, child composer and Back asserted; source manifest matched before/after | `/tmp/errands-task04c-route-diagnostics-phone-wrapper.log`, `/tmp/errands-task04c-run-quiet/kandev-run.e2e.hd7kh0OH.log` |
| `corepack pnpm@9.15.9 exec eslint --max-warnings 0 e2e/helpers/errands.ts e2e/tests/session/mobile-errands.spec.ts` and `corepack pnpm@9.15.9 exec prettier --check` for both files from `apps/web` | Passed with zero warnings and matching format | `/tmp/errands-task04c-route-diagnostics-final-check.log` |

The fresh-source desktop case passed with the real parent await transcript, explicit uncertainty, same-timestamp `received`, sibling isolation, omission clearing and unchanged historical await text. The fresh-source native phone case also passed with parent chat/socket readiness, row-by-ID selection, warning, bounded drawer/row geometry, no horizontal overflow, child composer and Back. One intermediate phone run on the same rebuilt artifacts rendered Task unavailable even though browser task and session GETs returned 200 (`/tmp/errands-task04c-run-quiet/kandev-run.e2e.FweWaGGr.log`); the cause of that route failure is undetermined. Its exact diagnostic retry passed and logged one non-blocking 503 resource error plus a WebSocket reconnect (`/tmp/errands-task04c-run-quiet/kandev-run.e2e.hd7kh0OH.log`). No run had zero MCP errand admission; the new helper saves full backend and process logs before fixture teardown if that occurs. Both final screenshot captures were copied and inspected: desktop `/tmp/errands-task04c-browser-desktop.png` (SHA256 `2b2dcdee1b293b5aa996e5f27e60739e63b046785bacdb4788ca7b1f9cc93db1`) and phone `/tmp/errands-task04c-browser-phone.png` (SHA256 `950f1bf0671a24bdb7de2d82488e587c403867abdf046393900c1239e00a8d1d`). Task04's 04A/04B/04C gates are complete; Task 24's combined desktop/phone matrix remains separate and pending.
