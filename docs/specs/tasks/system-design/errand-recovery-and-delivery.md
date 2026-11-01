---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-009
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
  - REQ-TASKS-ERRANDS-013
  - REQ-TASKS-ERRANDS-CONTEXT-001
  - REQ-TASKS-ERRAND-ORDER-001
created: 2026-10-30
owners:
  - kandev
---

# Errand recovery and delivery boundaries

## Scope and authority

This extends the [Errands design](errands.md) for the existing result, workflow, cancellation, admission and presentation contracts. It preserves the accepted shared-workspace/depth-one architecture. The [repair package](../../../plans/errands-review-repairs/plan.md) owns implementation order and validation; historical implementation receipts are not proof of these repairs.

The user-selected [await uncertainty ADR](../../../decisions/2026-10-30-errand-await-delivery-uncertainty.md) governs AC004.4, 004.8, 004.9 and 004.13. A durable receiver observation can establish reception; an absent observation, handler return, socket write or expired timeout cannot prove non-reception. Known non-exposure is automatically recoverable only after the invocation and any delayed claim are fenced. Unknown exposure remains retained and visible. Explicit inspection and recovery use the retained child transcript and existing conversation controls.

## Requirement mapping

| Contract | Boundary |
| --- | --- |
| 001, 002, 008 | Reserved identity classification and idempotent admission |
| 003, 004 | Exact-turn settlement, transactional claims, outcome recovery |
| 005, 006 | Role-aware runtime authority and fail-closed workflow settlement |
| 007 | Durable exact-target teardown and reconstructed cascades |
| 009 | Existing authenticated parent identity and fixed child catalog remain authoritative |
| 010, 011, CONTEXT-001, ORDER-001 | Authoritative summaries, reconnect/cache ownership and native navigation |
| 012 | Disabled dispatch of retained prepared output; unconditional role isolation |
| 013 | Publish/count only committed transitions; bounded label sets |

## Result identity and settlement

A result claim binds parent, errand, result turn/kind and row version, not only session ID. A delayed cancellation/settlement cannot mutate a successor result. Version-CAS mutation rechecks cancelled lifecycle before recording a late turn outcome. User cancellation keeps its one notice; parent/cascade cancellation keeps its discard/no-result outcome. Release compares the expected token and result identity inside the locked CAS callback, even after a retry.

Capture the immutable completed errand turn before successor queue admission. A user-origin successor preserves the captured result; a verified parent-origin successor may supersede it through the existing admission contract. Failed capture preserves exact durable terminal evidence and leaves the successor unclaimed. FIFO readiness and Send Now must satisfy the same capture precondition; no in-memory flag is sufficient after restart. Do not write task REVIEW merely to make an errand or outstanding/unknown parent promptable.

## Claim transaction

The task repository owns an internal atomic batch claim returning committed immutable snapshots. All candidates have one parent, validated route/token, distinct IDs and expected versions. A stale candidate or SQL error rolls back the complete batch; no partial result is a success. Use the write transaction for reads and writes, deterministic row order and existing PostgreSQL task-lock discipline. Complete fallible projection/assembly before the claim commit and build emitted text from the returned snapshots, never a later reload that can fail or expose an unclaimed result. Counters/events follow successful commit.

Await emits text exclusively for the exact attempt's committed claims. Other routes report their delivery route; newly pending unclaimed text is omitted until a later claim. Cancellation selects an error/status-only outcome before releasing and cannot then return successful result text. Unknown commit/exposure is not a confirmed rollback. The concrete attempt, serialization and prepared-output contracts below govern the implementation.

## Workflow and task-state authority

Parent settlement returns both outstanding state and error. Either known outstanding or unreadable evidence defers workflow and task review-state writes while keeping the session promptable. An error does not synthesize a successful workflow receipt; the original effect remains available to the existing attributed retry path. Ordinary readable no-errand completion remains a positive control. Parent turn cancellation still leaves children running and does not auto-run queued work.

Executor failure flows use the existing session-aware runtime task-state seam. No boolean role helper may conflate missing/unreadable evidence with an ordinary session. Created-session and fresh-fallback launches branch on retained immutable role before profile resolution, task scheduling, deferred-launch claims or workflow prompt composition. Errands preserve their selected profile, restricted MCP mode and attach-only workspace; existing safety/cancellation/ceiling guards remain required even when workflow effects are skipped. Retained errand isolation does not read the feature flag.

## Teardown and lifecycle recovery

Persist a versioned private teardown intent alongside logical cancellation in the existing errand row. The intent records its ID, pending/completed/superseded state and captured session/incarnation/execution/turn/submission authority, or an exact reserved initial-launch tuple when no session exists. An absent intent is not proof of teardown. Schema expansion must update every scanner/insert/update and both dialects; no public lifecycle status is added.

Capture and commit under existing cancellation/admission arbitration, release SQL before external I/O, and stop only the captured owned execution. Scheduling asynchronous teardown is not completion: exact stop acknowledgement or confirmed absence completes the intent. Transient failures remain pending for startup and the existing sweep, including flag-off operation. Same-execution successor turns invalidate old stop authority; revalidation and the physical side effect share operation ownership so a successor cannot slip between them. Legacy identity is never fabricated from today's active turn.

Bulk paths inspect pending teardown independently of projected live status; retries neither increment new-cancellation counts nor create another notice. Reconstruct lost cascades from authoritative task archive/missing state and terminal/missing parents before admission. Read errors block only affected task admission, never imply deletion. Mixed valid and terminal parents are processed independently. Recheck ownership immediately before creation/dispatch; the in-memory cascade worker is only a low-latency hint.

## Reserved launch recovery

Persist immutable initial launch authority with the session insertion: task/session/parent, reserved turn/submission and selected profile. Reconcile that tuple against durable turn/submission state and authenticated restored runtime inventory. Classify absent, prepared-unattempted, admitted, settled or blocked. Only absent creates a new session. Prepared continues the same session and reserved identity; admitted/uncertain effects never trigger another provider prompt. Settled adopts bookkeeping then reconciles the exact result. Missing/mismatched evidence blocks instead of producing a false launch-failure result. Cancellation wins over bookkeeping adoption and uses exact teardown compensation.

## Queue provenance and disabled delivery

Send Now retains the immutable original source IDs through initial transcript creation, subsequent envelope update, durable claim and recovery. A notification source is not the same as an empty notification-only dispatch. Preserve all source delivery receipts and ordinary content/attachments/references without merging them under an unrelated token. Already accepted or uncertain admission is not restored/replayed as a fresh claim.

The disabled gate applies before any dispatch of prepared output, including deletion notices and restored claims. Preserve independently owned full result content and exact ordinary source input. Never strip arbitrary user text by matching system tags, labels or previews. Server-only output can remain pending while ordinary input proceeds; legacy ambiguous source provenance is retained without dispatch. Re-enable uses the same pending authority, not a reconstructed preview. Shared delivery storage and queue ownership are one integration boundary, with a single writer.

## Authoritative summaries and frontend state

The task summary read-repair path batch-loads bounded live/waiting aggregates through a flag-aware reader. Disabled means explicit observed zero without reading retained errands; missing reader means unobserved compatibility, not zero. Read errors preserve last-valid state. CAS retry reloads both the summary and authoritative aggregate. Waiting is existential across idle parents with outstanding work; a failed sibling cannot erase a live sibling. Fallback session observations copy role and parent independently of the flag.

A store-owned connection generation changes on actual connected boundaries. Each task list tracks freshness for that generation and read scope; request ownership also includes task/reset generation. A stale completion cannot publish or delete a replacement in-flight owner. Shared requests survive one consumer unmount; reconnect of a different mounted task does not certify every cached task as fresh.

Context metadata and runtime cache converge in one accepted ingestion transaction. Omission is unknown/no-op; explicit null is authoritative clear. Request-start context epochs fence live samples and clears against delayed snapshots; timestamps reject older authoritative snapshots. Live epochs do not increment merely for accepting snapshots. Reset updates canonical metadata as well as cache so hook remount cannot revive an old value. Route hydration, list/detail setters, workspace events and reconnect share these rules. Passive rows add no child subscriptions, fetches or polling.

## Navigation, accessibility and uncertainty presentation

PreviewSessionTabs supplies the same scoped navigation context used by Threads, calling its local tab/view-mode selection owner. Open and Back cannot change another workbench selection or primary ownership. Menu visibility derives from immutable role, including flag-off retained sessions. Threads announcements use actual transcript visibility plus detail activation, independently of read-cursor acknowledgement.

Uncertainty is separate from done/failed/cancelled status. The existing panel shows localized delivery uncertainty and retains Open to the child transcript; the parent-facing response/status surface must not call an uncertain result definitely delivered. Do not add automatic resend or a second task-wide management surface. Phone keeps the existing bottom drawer, one list scroller, fixed safe-area footer, 44px row targets and full-screen child/Back. New copy must cover every locale. Exact attempt classification and additive projection fields belong to the shared delivery packet.

## Validation boundaries

Every repaired invariant has a behavioral regression at its owning boundary. Concurrency uses controlled barriers; SQL transaction/aggregate cases use production SQLite and actual PostgreSQL execution. Runtime integration uses the real orchestrator/executor with mocked provider boundaries and cannot certify real-provider teardown. Desktop/phone E2E exercises real preview/Threads/composer behavior, missed-event reconnect and mixed-parent transitions. Dark media uses isolated synthetic data and frozen candidate code. Package status changes only after final commands pass; no historical receipt or skipped test is promoted to current acceptance.

## Shared delivery persistence and state machines

### Shared identities and serialization

- Keep the current errand row as authority for its latest result. Add `result_revision` (opaque UUID, assigned whenever result content/kind is replaced, including cancellation), and `parent_admission_generation` (monotonic integer). Backfill a stable UUID per existing nonempty result. Claims use revision, not mutable CAS version; replacement always gets a new UUID.
- Use one private task-scoped SQL guard for result writes, claims/releases, parent-to-child admission reservation/decision and prepared deletion admission. Before gated reads, PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('errands:' || taskID, 0))`; SQLite reserves its writer using the established `UPDATE tasks SET id = id WHERE 0` pattern from `internal/db/taskhierarchy.go`. No new guard table is needed. All participating writers use the same key and lock order, followed by affected errand rows in sorted ID order. Keep existing task/hierarchy admission locks at their established outer boundary; do not acquire them from inside this guard. Never hold SQL locks across external I/O. Migrate every participating writer to enforce a phantom-free snapshot and test with independent connections.
- Each child parent-prompt admission increments its `parent_admission_generation` only in the transaction recording a positively accepted successor. A durable pre-dispatch reservation fences that child until submission/turn evidence proves acceptance or refusal. Unknown admission keeps the fence and blocks release of older claims for that child. The decision transaction takes the guard, updates the generation and supersedes old pending results on acceptance; on definite refusal it clears the fence without increment. Retry from existing submission evidence; missing evidence is not refusal. Initial, queue, Send Now and fallback admissions all use this seam.
- A claim captures `(errand_id, result_revision, parent_admission_generation, claim_token, route)`. A proved abort releases only the exact matching revision/token/route while the generation remains equal and no unresolved parent admission exists. If generation advanced, mark the old claim superseded/discarded, never pending; if the revision or token changed, do not touch the replacement. A pending abort blocked by an unresolved admission is retried when the admission resolves. User-origin follow-ups do not advance this generation or supersede the result.

### Atomic claims and await attempt outcome

Create task-scoped `errand_await_attempt` (`token` PK, `task_id`, `parent_session_id`, `state`, timestamps) and `errand_await_attempt_item` (PK `(token, errand_id, result_revision)`, captured generation). Retain attempts, including zero-item tombstones, until task deletion; bind operations to task and parent, index by task/state. Support SQLite and PostgreSQL.

States are `open`, `claimed`, `offered`, `uncertain`, `aborted`, and `received`. Commit sender-side `offered` before handing success to MCP; it is neither a receipt nor an immediate recovery warning. Required supported positive observations atomically record `received` with authenticated inbox acceptance. Terminal `received`/`aborted` decisions fence each other. Orphaned attempts lacking either proof become uncertain, never automatically replayable. The [await reception design](errand-await-receipts.md) defines exact native formats, response identity, captured ownership, receipt persistence and presentation timing.

`ClaimResults(task, parent, route, token, candidates)` takes the task guard. For await it creates/locks the attempt, checks task/parent and rejects an existing `aborted` tombstone. It reads the full parent inventory under the guard, validates the requested snapshot and distinct candidate IDs/versions, then claims all pending selected revisions and inserts all immutable attempt items in one transaction. Turn-context/notification uses the same atomic claim and records exact revisions in its receipt. On mismatch roll back and boundedly re-read/reproject. Return transaction snapshots only after commit; include text only for the exact token/revision. Assemble fallible content before claiming where possible; publish only committed changes. A commit error is unknown: retain the token and read back.

`AbortAwait(task, parent, token)` is idempotent and takes the same guard. Upsert an `aborted` tombstone even if no attempt or claim exists yet, then atomically release matching items subject to the identity, generation and admission rules above. Thus an abort that arrives before a delayed claim prevents that claim; a claim that wins first is released by the abort. Retry DB errors and ambiguous commits with the same token. Persist deferred release on the aborted attempt until admission resolves.

On backend error or cancellation the live agentctl handler irrevocably selects error and submits the token-bound abort; it cannot return success. Record a token/parent abort intent durably in agentctl's existing journal before returning the error, and retry delivery to the backend until the tombstone is confirmed; the backend abort endpoint also accepts the live direct attempt. If the journal cannot persist the intent, still return the error, retry while alive, and leave an unconfirmed backend claim uncertain after process loss. The intent is not a receiver receipt. Before returning success to the MCP library, durably mark the attempt `offered`; failure or ambiguity selects the fenced error branch. On recovery consume available authenticated native observations before classifying unresolved `open`, `claimed` or `offered` attempts as uncertain: state, disconnect and process death do not prove non-exposure.

Surface uncertainty alongside the preserved result/transcript and use existing conversation controls for deliberate recovery. Legacy delivered-await rows without attempts remain uncertain, never automatically replayed. Matching text, completion or a missing report proves nothing. Supported positive observations must bind captured parent incarnation/generation, session, token and exact revisions; persist their decision atomically with inbox acceptance before evidence pruning. The attempt retains compact stream/sequence/native-call/hash proof until task deletion. Late receipts settle historical attempts without mutating successor result revisions.

### Prepared deletion notice: queue is the row-independent authority

Use the existing durable server notification queue row, not a second result outbox. At enabled deletion, `PreserveCancellationNotice` idempotently creates one server-owned source keyed by `(task, parent incarnation, errand ID, cancellation revision)`, containing the complete bounded prepared block and immutable display snapshot, typed provenance, and the original queue source ID/generation. Confirm durable admission before deleting; queue-full/error prevents deletion. Retry after a crash reuses the same source. The source owns the full notice independently of the errand row; its preview is insufficient.

The source stays in the queue while pending. Dispatch may remove it only in a durable queue claim containing the full original source, ID/generation and exact dispatch/receipts. The claim owns it: definite refusal restores the source; acceptance settles with admission/transcript evidence; uncertainty retains it without replay. Every crash leaves source or claim recoverable. Terminal parent/task cleanup may discard it under existing rules.

With Errands disabled, drain and Send Now bypass untouched typed server-only sources before reservation or dispatch, preserving ordinary FIFO. A notification-only Send Now does not interrupt a live turn. For existing mixed claims, definite refusal restores the whole exact claim before ordinary-only reselection; accepted/uncertain claims remain owned, without replay. On enabled restart normal drain delivers the retained source once. Preserve capacity, auto-run, CREATED/fallback and incarnation fences.

Retain legacy prepared sources with proved provenance and full content; normalize metadata without changing their IDs. For legacy mixed claims use the persisted complete original `Sources` to rebuild only after definite refusal and full restoration. Missing or contradictory provenance, full content, source IDs or admission blocks the source/claim for inspection. Never strip text, fabricate a notice, drop user input or replay uncertainty; lookalike user text stays ordinary.

### Send Now receipts and source preservation

Build one private source-aware input from every immutable `claim.Sources` entry in FIFO order, with typed server provenance and the exact source ID, queue generation and session incarnation. Preserve ordinary content, whitespace, attachments, references, handoffs and mode semantics; keep server blocks separate. Never classify by user text or the first source. Compose live pending results once at admission and collect **all** `(token, route, errand ID, revision, generation)` identities, plus each prepared source ID/revision. Persist the whole list, exact effective prompt, and the complete source ID set in existing durable claim JSON before provider dispatch; keep any singular envelope token only as compatibility presentation, never as recovery authority.

Use the same source IDs and receipt list for early and late transcript paths and one deterministic message ID. Mark only represented sources recorded. A stale notification-only claim starts no turn; mixed claims retain all ordinary sources. Prepared notices dispatch without an errand row. Definite refusal restores the whole claim; acceptance/uncertainty retains all receipts under existing admission evidence. Persist changes, not just in-memory mutations; block legacy claims missing identities.

### Exact terminal settlement before successor

Use existing durable turn, submission, journal/inbox and terminal-effect evidence for retries. Persist an idempotent settlement decision keyed by `(task, child session incarnation, exact turn ID, submission ID)` in the existing durable effect/receipt mechanism, atomically with result creation or an explicit discard/no-result decision. Capture parent-origin and exact turn identity at admission, not by looking up the latest turn at recovery. Use that turn's committed canonical output; missing evidence leaves the decision unresolved. Fresh-state cancellation/stale-event guards must preserve cancelled children and successor results.

Before every successor reservation (FIFO, auto-run, Send Now, created and fallback), under the task guard ensure each preceding terminal child turn has its exact required decision committed. Replay pending durable effects by exact entry/turn/submission before retrying admission. Unknown or failed settlement blocks that child, preserving its queue source for startup/drain retry; never synthesize a result or use the latest turn. Serialize decision check and reservation; no lock spans runtime dispatch. Preserve execution/generation fences and task isolation.
