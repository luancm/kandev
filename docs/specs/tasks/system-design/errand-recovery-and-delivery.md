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

[Errands design map](errands.md#errands-design-map) lists shared contracts and companion ownership.

## Scope and authority

[Errand status and continuation](errand-status-and-continuation.md) owns recovery-attention projection and refusal presentation. Its attention reason is not a settlement decision and cannot relax this document's exact ownership gates.

This extends the [Errands design](errands.md) for the existing result, workflow, cancellation, admission and presentation contracts. It preserves the accepted shared-workspace/depth-one architecture. The [repair package](../../../plans/errands-review-repairs/plan.md) owns implementation order and validation; historical implementation receipts are not proof of these repairs.

The user-selected [await uncertainty ADR](../../../decisions/2026-10-30-errand-await-delivery-uncertainty.md) governs AC004.4, 004.8, 004.9 and 004.13. A durable receiver observation can establish reception; an absent observation, handler return, socket write or expired timeout cannot prove non-reception. Known non-exposure is automatically recoverable only after the invocation and any delayed claim are fenced. Unknown exposure remains retained and visible. Explicit inspection and recovery use the retained child transcript and existing conversation controls.

## Requirement mapping

| Contract                         | Boundary                                                                            |
| -------------------------------- | ----------------------------------------------------------------------------------- |
| 001, 002, 008                    | Reserved identity classification and idempotent admission                           |
| 003, 004                         | Exact-turn settlement, transactional claims, outcome recovery                       |
| 005, 006                         | Role-aware runtime authority and fail-closed workflow settlement                    |
| 007                              | Durable exact-target teardown and reconstructed cascades                            |
| 009                              | Existing authenticated parent identity and fixed child catalog remain authoritative |
| 010, 011, CONTEXT-001, ORDER-001 | Authoritative summaries, reconnect/cache ownership and native navigation            |
| 012                              | Disabled dispatch of retained prepared output; unconditional role isolation         |
| 013                              | Publish/count only committed transitions; bounded label sets                        |

## Result identity and settlement

A result claim binds parent, errand, result turn/kind and row version, not only session ID. A delayed cancellation/settlement cannot mutate a successor result. Version-CAS mutation rechecks cancelled lifecycle before recording a late turn outcome. User cancellation keeps its one notice; parent/cascade cancellation keeps its discard/no-result outcome. Release compares the expected token and result identity inside the locked CAS callback, even after a retry.

Capture the immutable completed errand turn before successor queue admission. A user-origin successor preserves the captured result; a verified parent-origin successor may supersede it through the existing admission contract. Failed capture preserves exact durable terminal evidence and leaves the successor unclaimed. FIFO readiness and Send Now must satisfy the same capture precondition; no in-memory flag is sufficient after restart. Do not write task REVIEW merely to make an errand or outstanding/unknown parent promptable.

## Claim transaction

The task repository owns an internal atomic batch claim returning committed immutable snapshots. All candidates have one parent, validated route/token, distinct IDs and expected versions. A stale candidate or SQL error rolls back the complete batch; no partial result is a success. Use the write transaction for reads and writes, deterministic row order and existing PostgreSQL task-lock discipline. Complete fallible projection/assembly before the claim commit and build emitted text from the returned snapshots, never a later reload that can fail or expose an unclaimed result. Counters/events follow successful commit.

Await emits text exclusively for the exact attempt's committed claims. Other routes report their delivery route; newly pending unclaimed text is omitted until a later claim. Cancellation selects an error/status-only outcome before releasing and cannot then return successful result text. Unknown commit/exposure is not a confirmed rollback. The concrete attempt, serialization and prepared-output contracts below govern the implementation.

## Workflow and task-state authority

Parent settlement returns both outstanding state and error. Either known outstanding or unreadable evidence defers workflow and task review-state writes while keeping the session promptable. An error does not synthesize a successful workflow receipt; the original effect remains available to the existing attributed retry path. Ordinary readable no-errand completion remains a positive control. Parent turn cancellation still leaves children running and does not auto-run queued work.

Ordinary executor failure flows use the session-aware runtime task-state seam. The Errand launcher resolves its saved profile, restricted MCP surface and existing workspace through its typed execution owner before allocating the runtime. Admission, cancellation and capacity guards apply independently of the release toggle.

## Teardown and lifecycle recovery

The [successful completion design](errand-completion-cleanup.md) extends this ownership boundary with distinct success-close effects. The cancellation rules below remain unchanged; automatic successful close must not reuse cancellation notice or discard semantics.

Persist a versioned private teardown intent alongside logical cancellation in the existing errand row. The intent records its ID, pending/completed/superseded state and captured session/incarnation/execution/turn/submission authority, or an exact reserved initial-launch tuple when no session exists. An absent intent is not proof of teardown. Schema expansion must update every scanner/insert/update and both dialects; no public lifecycle status is added.

Capture and commit under existing cancellation/admission arbitration, release SQL before external I/O, and stop only the captured owned execution. Scheduling asynchronous teardown is not completion: exact stop acknowledgement or confirmed absence completes the intent. Transient failures remain pending for startup and the existing sweep, including flag-off operation. Same-execution successor turns invalidate old stop authority; revalidation and the physical side effect share operation ownership so a successor cannot slip between them. Legacy identity is never fabricated from today's active turn.

Bulk paths inspect pending teardown independently of projected live status; retries neither increment new-cancellation counts nor create another notice. Reconstruct lost cascades from authoritative task archive/missing state and terminal/missing parents before admission. Read errors block only affected task admission, never imply deletion. Mixed valid and terminal parents are processed independently. Recheck ownership immediately before creation/dispatch; the in-memory cascade worker is only a low-latency hint.

Standalone startup correlates an owned Errand with the authenticated helper's task, Errand and execution generation, the exact runtime ID, and the saved latest native conversation binding. The transport key identifies the journal stream. The execution carries its explicit Errand owner and empty TaskSession coordinates. Ambiguous or mismatched candidates stay outside ordinary startup adoption and orphan stops. TaskSession reconciliation excludes typed Errand inventory from missing-session cleanup.

The backend restores the admitted turn and prompt generation from the Errand execution record. It compares the immutable SQL submission with the helper's retained journal and validates the projected cursor before replay. Recovery projects the captured prefix through the live Errand pipeline, then connects for later events. It never submits the prompt again. Unresolved evidence leaves the persisted runtime and transcript available for exact reconciliation.

Deferred Remote Docker recovery uses the saved container and agentctl instance identities with the helper's Errand-owner capability. Registration uses the current inventory and journal. Teardown accepts Docker's `exited` state for the exact recorded container. An inconclusive probe, including an unverified daemon's missing response, leaves the owner guarded.

Shared Kubernetes recovery validates the recorded Pod, PVC and workspace ownership generation. A descendant task's Errand may use an ancestor's workspace. Recovery checks that owner against the environment and the Errand owner against the helper. A missing Pod object leaves physical teardown unresolved. Shared resources remain with their owner.

SSH recovery requires the exact saved controller launch receipt and verifies the live controller before reading the agentctl instance. The SSH task coordinate identifies the controller's physical owner. The helper's explicit Errand identity and the current Errand binding identify the conversation.

Sprites recovery checks the saved Sprite name and creation time before opening its authenticated proxy. An existing secret reference supplies the credential. When an Errand used a literal credential and the process cache is gone, recovery and exact Stop read it from the saved executor profile after checking its executor identity. The runtime inventory carries that profile reference and the typed owner.

Retained plugin recovery reads the exact authenticated helper using the ready inventory's provider installation, environment generation and resource lease. Allocation and cleanup remain with the environment owner.

## Reserved launch recovery

Persist the task, Errand, parent, selected profile and reserved turn and submission before launch. Recover an acknowledgement from the authenticated helper's exact accepted journal entry after checking the current native binding. The launcher reconciles that receipt or the original terminal evidence before changing the queued lifecycle. A settled launch reconciles the exact result. Missing or mismatched evidence keeps admission unresolved without another provider prompt or an invented result. Cancellation wins over launch bookkeeping and uses exact teardown compensation.

Start and queue replay admit executions through the Errand owner and reconcile any durable generation before creating a new one.

## Queue provenance and disabled delivery

Send Now retains the immutable original source IDs through initial transcript creation, subsequent envelope update, durable claim and recovery. A notification source is not the same as an empty notification-only dispatch. Preserve all source delivery receipts and ordinary content/attachments/references without merging them under an unrelated token. Already accepted or uncertain admission is not restored/replayed as a fresh claim.

When Errands are disabled, FIFO and Send Now bypass trusted server notifications before reservation or dispatch. Ordinary input retains its text, attachments, references and queue order. A notification-only Send Now leaves an active turn running. If a mixed claim is definitely refused, restore its exact sources before selecting ordinary input again. Accepted and uncertain claims retain their original ownership. Re-enabling delivery allows queued notifications to compose then-pending results during dispatch.

## Authoritative summaries and frontend state

The task summary read-repair path batch-loads bounded live/waiting aggregates through a flag-aware reader. Disabled means explicit observed zero without reading retained errands; missing reader means unobserved compatibility, not zero. Read errors preserve last-valid state. CAS retry reloads both the summary and authoritative aggregate. Waiting is existential across idle parents with outstanding work; a failed sibling cannot erase a live sibling. Ordinary session lists contain ordinary conversations. The scoped Errand read supplies the associated parent identity.

A store-owned connection generation changes on actual connected boundaries. Each task list tracks freshness for that generation and read scope; request ownership also includes task/reset generation. A stale completion cannot publish or delete a replacement in-flight owner. Shared requests survive one consumer unmount; reconnect of a different mounted task does not certify every cached task as fresh.

Context metadata and runtime cache converge in one accepted ingestion transaction. Omission is unknown/no-op; explicit null is authoritative clear. Request-start context epochs fence live samples and clears against delayed snapshots; timestamps reject older authoritative snapshots. Live epochs do not increment merely for accepting snapshots. Reset updates canonical metadata as well as cache so hook remount cannot revive an old value. Route hydration, list/detail setters, workspace events and reconnect share these rules. Passive rows add no child subscriptions, fetches or polling.

## Navigation, accessibility and uncertainty presentation

PreviewSessionTabs supplies the same scoped navigation context used by Threads, calling its local tab/view-mode selection owner. Open and Back cannot change another workbench selection or primary ownership. Menu visibility derives from the owned Errand view, including retained Errands when the release toggle is off. Threads announcements use actual transcript visibility plus detail activation, independently of read-cursor acknowledgement.

Uncertainty is separate from done/failed/cancelled status. The existing panel shows localized delivery uncertainty and retains Open to the child transcript; the parent-facing response/status surface must not call an uncertain result definitely delivered. Do not add automatic resend or a second task-wide management surface. Phone keeps the existing bottom drawer, one list scroller, fixed safe-area footer, 44px row targets and full-screen child/Back. New copy must cover every locale. Exact attempt classification and additive projection fields belong to the shared delivery packet.

## Validation boundaries

Every repaired invariant has a behavioral regression at its owning boundary. Concurrency uses controlled barriers; SQL transaction/aggregate cases use production SQLite and actual PostgreSQL execution. Runtime integration uses the real orchestrator/executor with mocked provider boundaries and cannot certify real-provider teardown. Desktop/phone E2E exercises real preview/Threads/composer behavior, missed-event reconnect and mixed-parent transitions. Dark media uses isolated synthetic data and frozen candidate code. Package status changes only after final commands pass; no historical receipt or skipped test is promoted to current acceptance.

## Shared delivery persistence and state machines

### Shared identities and serialization

- The Errand row owns its latest result. A change to result content or kind assigns a new `result_revision` UUID. Claims capture that revision and the monotonic `parent_admission_generation`.
- Use one private task-scoped SQL guard for result writes, claims/releases, parent-to-child admission reservation/decision. Before gated reads, PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('errands:' || taskID, 0))`; SQLite reserves its writer using the established `UPDATE tasks SET id = id WHERE 0` pattern from `internal/db/taskhierarchy.go`. No new guard table is needed. All participating writers use the same key and lock order, followed by affected errand rows in sorted ID order. Keep existing task/hierarchy admission locks at their established outer boundary; do not acquire them from inside this guard. Never hold SQL locks across external I/O. Migrate every participating writer to enforce a phantom-free snapshot and test with independent connections.
- Each child parent-prompt admission increments its `parent_admission_generation` only in the transaction recording a positively accepted successor. A durable pre-dispatch reservation fences that child until submission/turn evidence proves acceptance or refusal. Unknown admission keeps the fence and blocks release of older claims for that child. The decision transaction takes the guard, updates the generation and supersedes old pending results on acceptance; on definite refusal it clears the fence without increment. Retry from existing submission evidence; missing evidence is not refusal. Initial, queue, Send Now and fallback admissions all use this seam.
- A claim captures `(errand_id, result_revision, parent_admission_generation, claim_token, route)`. A proved abort releases only the exact matching revision/token/route while the generation remains equal and no unresolved parent admission exists. If generation advanced, mark the old claim superseded/discarded, never pending; if the revision or token changed, do not touch the replacement. A pending abort blocked by an unresolved admission is retried when the admission resolves. User-origin follow-ups do not advance this generation or supersede the result.

### Atomic claims and await attempt outcome

Create task-scoped `errand_await_attempt` (`token` PK, `task_id`, `parent_session_id`, `state`, timestamps) and `errand_await_attempt_item` (PK `(token, errand_id, result_revision)`, captured generation). Retain attempts, including zero-item tombstones, until task deletion; bind operations to task and parent, index by task/state. Support SQLite and PostgreSQL.

States are `open`, `claimed`, `offered`, `uncertain`, `aborted`, and `received`. Commit sender-side `offered` before handing success to MCP; it is neither a receipt nor an immediate recovery warning. Required supported positive observations atomically record `received` with authenticated inbox acceptance. Terminal `received`/`aborted` decisions fence each other. Orphaned attempts lacking either proof become uncertain, never automatically replayable. The [await reception design](errand-await-receipts.md) defines exact native formats, response identity, captured ownership, receipt persistence and presentation timing.

`ClaimResults(task, parent, route, token, candidates)` takes the task guard. For await it creates/locks the attempt, checks task/parent and rejects an existing `aborted` tombstone. It reads the full parent inventory under the guard, validates the requested snapshot and distinct candidate IDs/versions, then claims all pending selected revisions and inserts all immutable attempt items in one transaction. Turn-context/notification uses the same atomic claim and records exact revisions in its receipt. On mismatch roll back and boundedly re-read/reproject. Return transaction snapshots only after commit; include text only for the exact token/revision. Assemble fallible content before claiming where possible; publish only committed changes. A commit error is unknown: retain the token and read back.

`AbortAwait(task, parent, token)` is idempotent and takes the same guard. Upsert an `aborted` tombstone even if no attempt or claim exists yet, then atomically release matching items subject to the identity, generation and admission rules above. Thus an abort that arrives before a delayed claim prevents that claim; a claim that wins first is released by the abort. Retry DB errors and ambiguous commits with the same token. Persist deferred release on the aborted attempt until admission resolves.

On backend error or cancellation the live agentctl handler irrevocably selects error and submits the token-bound abort; it cannot return success. Record a token/parent abort intent durably in agentctl's existing journal before returning the error, and retry delivery to the backend until the tombstone is confirmed; the backend abort endpoint also accepts the live direct attempt. If the journal cannot persist the intent, still return the error, retry while alive, and leave an unconfirmed backend claim uncertain after process loss. The intent is not a receiver receipt. Before returning success to the MCP library, durably mark the attempt `offered`; failure or ambiguity selects the fenced error branch. On recovery consume available authenticated native observations before classifying unresolved `open`, `claimed` or `offered` attempts as uncertain: state, disconnect and process death do not prove non-exposure.

Surface uncertainty alongside the preserved result/transcript and use existing conversation controls for deliberate recovery. Legacy delivered-await rows without attempts remain uncertain, never automatically replayed. Matching text, completion or a missing report proves nothing. Supported positive observations must bind captured parent incarnation/generation, session, token and exact revisions; persist their decision atomically with inbox acceptance before evidence pruning. The attempt retains compact stream/sequence/native-call/hash proof until task deletion. Late receipts settle historical attempts without mutating successor result revisions.

### Send Now receipts and source preservation

Build one private source-aware input from every immutable `claim.Sources` entry in FIFO order, with typed server provenance and the exact source ID, queue generation and session incarnation. Preserve ordinary content, whitespace, attachments, references, handoffs and mode semantics; keep server blocks separate. Never classify by user text or the first source. Compose live pending results once at admission and collect **all** `(token, route, errand ID, revision, generation)` identities. Persist the whole list, exact effective prompt, and the complete source ID set in existing durable claim JSON before provider dispatch; keep any singular envelope token only as compatibility presentation, never as recovery authority.

Use the same source IDs and receipt list for early and late transcript paths and one deterministic message ID. Mark only represented sources recorded. A stale notification-only claim starts no turn; mixed claims retain all ordinary sources. Definite refusal restores the whole claim; acceptance/uncertainty retains all receipts under existing admission evidence. Persist changes, not just in-memory mutations; validate the stored identities before restoring a claim.

### Exact terminal settlement before successor

Use the owned admission, turn, harness binding and projected journal cursor to bind every receipt to its prompt. A provider terminal event closes its exact turn and records its actual outcome. Result capture validates the current owned receipt and parent admission under task and version guards before preparing cleanup and publishing. Historical evidence remains attached to its turn. Exact physical close remains a separate operation before explicit successor admission.

### Interrupted delegation recovery

Record the authenticated local process identity with the admission when available. Recovery requires positive termination of that writer and an authenticated close of the exact Errand instance before opening its retained journal. Read the original submission using the admitted execution, native conversation, stream, prompt generation and payload hash. Project the verified prefix before committing a sequence-zero interruption receipt. The transaction rechecks the admitted identity and cursor and preserves the original payload and provider outcome fields. A real terminal receipt retains authority. An interruption records the delegation as failed and leaves external effects unknown.

Missing or conflicting evidence keeps the owned admission unresolved. The current result and any cancellation decision retain their authority when historical evidence arrives. Cleanup confirms absence separately. Explicit continuation confirms the preceding result and cleanup, then uses the durable interruption receipt to retire the exact journal entry before opening the successor.
