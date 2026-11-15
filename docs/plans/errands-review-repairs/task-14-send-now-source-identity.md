---
id: "14-send-now-source-identity"
title: "Preserve Send Now sources and receipt identity"
status: done
wave: 14
depends_on:
  - "04-await-outcome-recovery"
  - "08-created-errand-isolation"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-006
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.7
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.11
  - AC-TASKS-ERRANDS-004.12
  - AC-TASKS-ERRANDS-006.5
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 14: Preserve Send Now sources and receipt identity

## Scope and authority

Repairs review finding(s) 18 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. No production changes have been made for this packet.

## Low-level implementation and regression plan

The concrete receipt/source contract is defined in the shared recovery design, sections Send Now receipts and Prepared deletion notice. Queue sources/claims own row-independent prepared notices; no separate result outbox or alternate schema is permitted.

**Owned:** `internal/orchestrator/queue_send_now_errand.go`, `queue_send_now.go:promptSendNowClaim`, `queue_send_now_claim.go:{prepareSendNowTranscript,sendNowAfterClaim,markSendNowSourcesRecorded}`, `errand_turn_composition.go` source-aware input/metadata helpers; new `queue_send_now_errand_test.go`. Any persisted claim enrichment belongs with the Task 03/04 claim contract and this packet's durable queue update in `messagequeue/repository_send_now_recovery.go`; serialize it. Do not change generic `BuildSendNowEnvelope` to know Errands policy. Ownership also includes `internal/backendapp/errands_transcript.go:HasDeliveryToken` and `errands_transcript_test.go`: recognize every represented internal source receipt during recovery, preserving scalar legacy decoding and existing submission/error semantics. This integration is required by the approved all-source receipt contract, not a new public API.

**Root correction:** Distinguish **contains an errand notification source** (delivery route/provenance) from **is an empty notification-only dispatch** (whether to start a provider turn). The current boolean is insufficient. Never use the first/oldest source's metadata as authority for every source.

**Algorithm:**

1. Build a private source-aware input from immutable `claim.Sources`, in FIFO order; use Dispatch only for a legacy single-source claim when provenance is independently known. Classify each source as ordinary input, empty notification, or prepared delivery with typed server provenance. A metadata marker in an arbitrary user's content is not classification authority. Preserve source IDs, queue identity and operation generation.
2. Ordinary input existence includes visible content (not trimmed/reconstructed except existing builder behavior), attachments, accepted entity references, step handoff and plan-mode/config-mode input semantics already recognized by the queue. Server-prepared result blocks are a separate component; their nonempty text must not count as user input.
3. Preserve **every** source delivery claim/revision reference. A singular public `DeliveryEnvelope.Token` can remain a compatibility field only if new internal metadata carries all source receipts and recovery reads them all. Do not merge arbitrary `Items` under one token and then release/ack only that token. Use Task 03's atomic claim for live rows; retain existing source receipts separately in the persisted claim list because prepared deleted-row notices remain queue-owned.
4. Compose current pending results once at actual dispatch, using actual notification/turn-context route. A notification-only selection whose output has already been consumed/superseded/discarded starts no turn; a mixed ordinary selection sends all original ordinary input even when its notification is stale. Prepared surviving-deletion output is not empty merely because its errand row is gone.
5. Record the same immutable source-ID set and delivery envelope at transcript creation and updates. `prepareSendNowTranscript` currently passes all source IDs while later `sendNowAfterClaim` can use the random Dispatch ID: fix this so `queuedTranscriptMessageID` is identical through early/late prepared, durable-plan-comment, created and fallback paths. Mark only sources actually represented by that receipt as recorded.
6. Persist enriched combined delivery metadata and the exact effective provider prompt with the durable Send Now claim **before** dispatch. Restore on definite refusal using original source rows and generations. Accepted or uncertain admission retains all receipt ownership and does not replay sources. Use existing initial acceptance callbacks and transient/fresh-fallback snapshot plumbing.
7. Source deletion/generation change or session-incarnation replacement invalidates only the exact old claim; do not stamp/ack a successor. Run claims under existing queue admission fences, never mutate `Sources` in memory and assume durable `claim_json` follows.

**Interface/schema:** No public queue request/response change. Internal source-aware struct replaces the notification boolean where behavior depends on it. Internal persisted metadata expansion (all receipt IDs + structured prepared components) using the shared design and Tasks 03/04 identities. No new SQL table required by this packet; existing claim JSON can store structured provenance, with a guarded writer added if needed. Explicit legacy decoding is necessary.

**Regressions:** `TestSendNowMixedErrandSourcesPreserveOrdinaryInputAndIdentity`: user first/prepared last and reverse; distinct user text, multiple result tokens/revisions, attachments/ref/handoff. Exercise real `SendQueuedNow` and capture provider payload + persisted transcript, not helper flags. Assert all ordinary input exactly retained, each result once, exact source IDs and recovery receipts. `TestSendNowStaleNotificationOnlyStartsNoTurn` and mixed stale-notification control. `TestSendNowErrandTranscriptUsesSameSourceIDsForEarlyAndLateSnapshot` mixes durable plan comment so the early transcript path is real; assert one deterministic row after update/reload. `TestSendNowErrandClaimsSurviveRestartAndRefusal` reopens queue + tasks DB and tests definite refusal vs uncertain acceptance, all tokens/revisions and sources, replacement-incarnation fence. Include CREATED and missing-execution fallback cases because the existing `startOptions.prepareInitialPrompt` has separate logic.

```bash
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(SendNow.*Errand|SendNow.*Notification|SendNowMixed|ComposeErrandTurn|Queued.*Errand)' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator/messagequeue -run 'SendNow|Admission|Delivery' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/queue_send_now_errand.go apps/backend/internal/orchestrator/queue_send_now.go apps/backend/internal/orchestrator/queue_send_now_claim.go apps/backend/internal/orchestrator/errand_turn_composition.go
git diff --check
```

**ACs:** `004.7`, `004.8`, `004.9`, `004.10`, `004.11`, `004.12`; source retention supports `006.5`. Dependencies: Tasks 03, 04 and 08; shared receipt contract in the recovery design. Do not publish a new multi-token wire API; the list is internal authority, historical cards retain their immutable snapshots.

## Dependencies and ownership

[Task 04](task-04-await-outcome-recovery.md), [Task 08](task-08-created-errand-isolation.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Ownership blocker identified on 2026-11-01 before production or test edits. Task14 remains `in_progress`; no acceptance is claimed.

The existing recovery consumer `apps/backend/internal/backendapp/errands_transcript.go:57` implements `HasDeliveryToken` by comparing only the singular `ErrandDeliveryTokenMetadataKey` at line 66. `internal/errands/reconcile.go:188` calls that consumer for each delivered token and releases absent tokens at lines 195–202. Adding an internal receipt list in the owned Send Now files and durable claim JSON cannot make that consumer recognize the additional tokens. After an accepted multi-source dispatch, reconciliation would therefore return represented secondary claims to pending and allow duplicate delivery. Keeping the singular token as presentation compatibility does not solve recovery ownership.

Safe completion requires integration authority for that existing recovery reader and its focused tests to decode every persisted internal receipt, preserve explicit legacy scalar behavior and retain the existing submission-outcome check. These files are outside this packet's explicitly granted ownership. No alternate transcript rows, public wire expansion or changes to accepted Task03/04 release semantics were introduced to work around the boundary.

Read-only inspection covered the full work order, shared recovery design (including Send Now receipts and Prepared deletion notice), accepted Task03/04/08/13 contracts and current source/consumer implementations. Before inventory and original owned snapshots are retained at `/tmp/errands-task14-before-inventory.json`, `/tmp/errands-task14-before-status.z` and `/tmp/errands-task14-before-files/`. Progress is recorded at `/tmp/errands-task14-progress.md`. Only this Results section and the temporary evidence files changed. No tests, builds, lint, database servers or other heavy commands were started; no command handles or validation locks remain held. No commit, push, shared-manifest edit, schema change or live operation occurred.

Task15 handoff remains pending: its disabled-output gate must consume the internal structured components and typed server provenance once Task14's complete receipt/recovery contract can be implemented. No Task15 implementation was attempted.


### Ownership integration resolved

The primary grants the transcript reader and its focused tests to Task14. Its scalar-only lookup would otherwise cause reconciliation to release additional represented tokens. Extend the same internal provenance decoder to recovery; test real persisted metadata after reopen, every represented token, unrelated tokens, scalar legacy data, failed/cancelled versus accepted/uncertain submission evidence and read errors. The earlier ownership blocker is resolved; implementation remains in progress. Preserve the existing ambiguity policy and do not infer receipt from arbitrary user content.

### Interrupted implementation and bounded continuation

Errand `af0ad700-c38e-4c48-a062-1ab9e6e75d30` exited with code `-1` without an acceptance result. The reported npm notices do not establish a root cause. Four Send Now regression functions and an extended persisted-transcript recovery test are saved; `/tmp/errands-task14-progress.md` records no production edits for this packet. `/tmp/errands-task14-red.log` is empty, so neither RED nor GREEN is established. Read-only inspection found no active Go/compiler/test process or holder of `/tmp/kandev-errands-repair-build.lock`; the historical handle `32362` must not be treated as a resumable command or completed check.

Continue sequentially in two bounded subpasses without changing this work order's acceptance bar:

- **14A, internal receipts and recovery:** Own new `internal/errands/send_now_delivery.go` and `send_now_delivery_test.go` for the shared internal structured source/receipt representation and decoder, plus `internal/backendapp/errands_transcript.go` and `errands_transcript_test.go`. Establish behavioral RED using the saved real persisted-transcript regression, then recognize every represented token while preserving legacy scalar behavior, exact source/receipt identities, and existing durable submission/error semantics. Use complete production-shaped fixtures, validate malformed or contradictory structured metadata, and do not treat arbitrary user content as receipt evidence. The saved `errand_send_now` fixture is a starting point, not authority to omit identities required by the shared design. Record the final internal shape for the dispatch consumer. This pass does not change Send Now dispatch, source admission, claim persistence, or Task15.
- **14B, queue and transcript integration:** After 14A's receipt, complete the original algorithm and all named Send Now regressions. In addition to the original owned files, the primary grants narrowly coupled changes to `internal/orchestrator/errand_parent_deliverer.go` to stamp trusted server source provenance and `internal/orchestrator/event_handlers_agent.go` to preserve the complete structured metadata when updating an existing transcript row, with focused tests. These implement the existing contract; they do not authorize unrelated admission or event-handler changes. Retain every original source/generation and all delivery receipts in the guarded durable claim before dispatch, and verify CREATED/fallback, early/late transcript, refusal/restart/uncertainty, and successor fences.

For 14A, run the focused `TestErrandTranscript` recovery cases and new `TestSendNowDelivery` codec cases with `go test -race -trimpath -p 1`, plus changed-file lint and formatting. Set `GOMAXPROCS=2` and hold the shared build lock for each heavy command. Save commands, exit status, logs, and any live handle in `/tmp/errands-task14-progress.md`; finish with a durable checkpoint rather than assuming an interrupted command passed. Keep Task14 `in_progress` after 14A. Only 14B plus the complete original acceptance commands can close it.

### Task14B integration checkpoint and narrow ownership extension

The first Task14B pass remains incomplete: 28 of 30 targeted orchestrator tests pass; final runtime-canonical prompt persistence and attachment-only empty text fail. Queue, reader and codec selectors, build, SQL guard and scoped lint pass, but do not establish Task14 acceptance. Evidence: `/tmp/errands-task14b-handoff.md`. CREATED/fallback with fresh callback context, nonzero producer generations and contradictory legacy metadata still need complete coverage. All implementation remains uncommitted.

The primary extends ownership to `internal/orchestrator/agent_delivery_submission.go:prepareAgentDeliverySubmission` and its directly relevant tests for a narrow Send Now hook at the existing pre-dispatch admission boundary. Persist the actual runtime-canonical text through the captured exact claim, including when runtime callbacks use a fresh context; preserve legacy and durable protocols and all non-Send-Now admission behavior. Do not broaden this into an admission refactor.

Ownership also includes `internal/errands/send_now_delivery.go` and its tests to accept an explicitly present empty `effective_prompt` for valid attachment-only ordinary input while still rejecting missing/null/wrong-type fields and retaining strict identities, receipts and provenance. This corrects an overly restrictive implementation of the existing ordinary-input parity requirement; never synthesize text. Preserve the accepted reader ambiguity policy. Complete the remaining original Task14 acceptance, including CREATED/fallback, nonzero producer identities and fail-closed contradictory legacy metadata, before marking done or committing accepted integration. Task15 remains separate.

### Final acceptance

Task14 is complete within the recorded SQLite and controlled-runtime backend scope. Final runtime-canonical prompt and attachments cross the existing common admission boundary; the exact durable claim and deterministic transcript are written before provider I/O. Explicit empty ordinary input is valid while missing/null prompt fields, empty server-only metadata and ambiguous legacy sources fail closed. CREATED/fallback fresh callback contexts, both protocols, nonzero producer generations, write-refusal restoration, accepted/uncertain retention and successor fences are exercised. Focused integration review found no issues.

Final race selectors passed: 38 orchestrator tests, 86 queue tests, four recovery-reader tests, six codec tests and 28 adjacent admission tests (counts are per selector, not deduplicated). Eight queue PostgreSQL cases/subcases skipped without a DSN. Changed-package build, SQL guard, SQLite store conformance, scoped lint, gofmt and whitespace checks passed. No PostgreSQL server, browser or real provider was exercised. Exact commands, test-node counts, hashes and limits: `/tmp/errands-task14b-completion-handoff.md`. Task15 disabled-output behavior remains separate.
